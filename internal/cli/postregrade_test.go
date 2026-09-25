package cli

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// movedOffTarget is audit round 5's list-38-2 up to the edit: `cr record` stores
// f1 resting on the fixture's failed gap probe, whose target is line 43 of the
// record's 42-44 anchor, so §6.2's `probed` row holds; `cr draft` renders it;
// and the reviewer's marker then moves the anchor to 46-46, which no longer
// holds the target. edits are further marker edits made in the same save.
//
// The record asks a question in its summary, so §8.1.5 lets it reach the author
// as one once §6.3 forces it there.
func movedOffTarget(t *testing.T, kind finding.Kind, edits ...[2]string) state.Layout {
	t.Helper()
	layout := probedHome(t, gapFixture{result: "failed", passed: true, mapped: true, issue: gapIssue})
	standInDiff(t, []git.Hunk{{
		Path: recordPath, Side: git.Right, BaseStart: 1, BaseLines: 100, HeadStart: 1, HeadLines: 100,
	}})
	record := aProbedRecord(gapClaim)
	record["kind"] = string(kind)
	record["summary"] = "Does any test fail when the cancel branch is removed?"
	_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", record), "--repo", recordSlug)
	require.NoError(t, err)
	_, err = runDraft(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)
	require.Equal(t, finding.GradeProbed, probedRecord(t, layout).Grade,
		"the probe's target lies inside the recorded anchor, so the record starts out probed")

	file := layout.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FileDraft)
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	// The whole pair is replaced, for the reason markeredits_test.go gives.
	edited := markerEdit(t, string(body), "f1", `start_line="42" line="44"`, `start_line="46" line="46"`)
	for _, edit := range edits {
		edited = markerEdit(t, edited, "f1", edit[0], edit[1])
	}
	require.NoError(t, os.WriteFile(file, []byte(edited), 0o600))
	return layout
}

// probedRecord is f1 as findings.ndjson holds it for the fixture's round.
func probedRecord(t *testing.T, layout state.Layout) finding.Finding {
	t.Helper()
	stored, err := state.ReadStamped[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings, recordRound)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	return stored[0]
}

// labelOf is the first paragraph of a comment body, which is where §8.1.4 puts
// a question's label region.
func labelOf(body string) string {
	label, _, _ := strings.Cut(body, "\n\n")
	return label
}

// arguedLabel is §8.1.4's region for an argued question in the default
// render.lang.
func arguedLabel(t *testing.T) string {
	t.Helper()
	label, err := render.QuestionLabelRegion(render.LangEN, finding.GradeArgued)
	require.NoError(t, err)
	return label
}

// §7.2.2 and invariant 4, audit round 5's list-38-2: `cr post` recomputes the
// grade after it has applied the draft's anchor edit, not before, so a record
// moved off its probe's target loses the `probed` grade and §6.3 forces it to a
// question before the payload is built — in the dry run, in the request the
// confirmed send hands gh, and in what the send stores.
//
// Recomputed before the edit, the grade would still read the target inside the
// old range, and f1 would reach the author as a finding at line 46 carrying the
// line-43 evidence block.
func TestAnAnchorMovedOffItsProbeTargetIsPostedAsAnArguedQuestion(t *testing.T) {
	layout := movedOffTarget(t, finding.KindFinding)

	printed, err := runPost(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)
	var report struct {
		Comments []postedComment  `json:"comments"`
		Payload  *post.Review     `json:"payload"`
		Forced   finding.Forcings `json:"forced_to_question"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	assert.Equal(t, []postedComment{{ID: "f1", Kind: finding.KindQuestion}}, report.Comments,
		"§6.3.1: the recomputed grade is argued, so the record leaves as a question")
	assert.Equal(t, finding.Forcings{{Class: "unchecked-error", Count: 1}}, report.Forced)
	require.NotNil(t, report.Payload)
	require.Len(t, report.Payload.Comments, 1)
	assert.Equal(t, 46, report.Payload.Comments[0].Line, "§7.2: the moved anchor is the comment's position")
	assert.Equal(t, arguedLabel(t), labelOf(report.Payload.Comments[0].Body),
		"§8.1.4: the question carries the argued label, not an evidence block")

	shim := ghShimming(t, report.Payload)
	_, err = runPost(t, recordPR, "--repo", recordSlug, "--confirm")
	require.NoError(t, err)
	require.Len(t, shim.writes(t), 1)
	var sent post.Review
	require.NoError(t, json.Unmarshal(shim.sentBody(t), &sent))
	require.Len(t, sent.Comments, 1)
	assert.Equal(t, arguedLabel(t), labelOf(sent.Comments[0].Body),
		"the request gh was handed carries the same argued question")

	stored := probedRecord(t, layout)
	assert.Equal(t, finding.GradeArgued, stored.Grade, "§7.2.2: the recomputed grade is stored")
	assert.Equal(t, finding.KindQuestion, stored.Kind, "§6.3.1: the forcing is stored")
	assert.Equal(t, 46, stored.Anchor.Line)
}

// markerAttribute matches one `name="value"` pair of a §7.1.1 marker.
var markerAttribute = regexp.MustCompile(`(\w+)="([^"]*)"`)

// markerFields parses the marker line of record id in a draft into its fields.
func markerFields(t *testing.T, draft, id string) map[string]string {
	t.Helper()
	for line := range strings.SplitSeq(draft, "\n") {
		if strings.HasPrefix(line, `<!-- cr:record id="`+id+`"`) {
			fields := make(map[string]string)
			for _, pair := range markerAttribute.FindAllStringSubmatch(line, -1) {
				fields[pair[1]] = pair[2]
			}
			return fields
		}
	}
	require.Failf(t, "no marker", "the draft holds no block for %s", id)
	return nil
}

// The same order at `cr draft`: a regeneration that applies the anchor edit
// recomputes that record's grade before §6.3's forcing, so the draft the
// reviewer reads next already shows the argued question the post would send.
func TestARegeneratedDraftRegradesARecordItsTriageMoved(t *testing.T) {
	layout := movedOffTarget(t, finding.KindFinding)

	_, err := runDraft(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)

	assert.Equal(t, finding.GradeArgued, probedRecord(t, layout).Grade,
		"the grade is recomputed over the anchor the triage applied")
	body, err := os.ReadFile(layout.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FileDraft))
	require.NoError(t, err)
	marker := markerFields(t, string(body), "f1")
	assert.Equal(t, "question", marker["kind"], "§6.3.1 at draft time, over the recomputed grade")
	assert.Equal(t, "argued", marker["grade"])
	assert.Equal(t, "46", marker["line"])
}

// §7.2's `kind` row against the anchor edit made in the same save: a question
// hardened to a finding while its anchor moves off the probe's target was
// admitted on the grade the old anchor earned, and aborts once the grade is
// recomputed over the new one — at both commands that interpret the draft,
// rather than being forced back to a question behind the reviewer's back.
func TestAHardeningMadeWithTheAnchorMoveIsHeldToTheRecomputedGrade(t *testing.T) {
	for _, command := range []string{"draft", "post"} {
		t.Run(command, func(t *testing.T) {
			movedOffTarget(t, finding.KindQuestion, [2]string{`kind="question"`, `kind="finding"`})

			_, err := runCLIPrinting(t, command, recordPR, "--repo", recordSlug)

			require.Error(t, err)
			var refused *finding.ArguedAssertionError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, "f1", refused.Record, "§6.3.3 names the record")
			assert.Equal(t, ExitValidation, exitCodeFor(err))
		})
	}
}
