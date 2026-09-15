package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// headerWaivers is the header's waiver line, written out rather than read off
// the constant, so a rewording of what the reviewer is told fails here.
const headerWaivers = "waivers: a block marked disposition=\"wrong\" writes a repository-wide waiver, and a " +
	"deleted block a pull-request-scoped one, when the next `cr draft` regenerates this file, " +
	"before anything is posted (§7.1.6); with no `cr draft` in between, `cr post --confirm` writes them"

// warned is the warnings a `cr draft` run reported.
type warned struct {
	Warnings []string `json:"warnings"`
}

// draftWarnings runs `cr draft` and decodes the warnings it reported.
func draftWarnings(t *testing.T) []string {
	t.Helper()
	printed, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	var got warned
	require.NoError(t, json.Unmarshal([]byte(printed), &got))
	return got.Warnings
}

// headerTail is every line of the draft's header after its comment count, up
// to and excluding the close: the lines field-feedback 2.6, 2.8 and 2.11 added,
// read whole so an extra or missing line fails as surely as a changed one.
func headerTail(t *testing.T, rendered string) []string {
	t.Helper()
	header, _, closed := strings.Cut(rendered, "\n-->\n")
	require.True(t, closed, "the header closes")
	lines := strings.Split(header, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "comments: ") {
			return lines[i+1:]
		}
	}
	require.Fail(t, "the header carries its comment count")
	return nil
}

// Field-feedback 2.6 through `cr draft` and `cr post`: a question whose body
// holds no "?" is warned about when the draft is written, naming the record,
// and counted in the header; `cr post` still refuses it; and once the body in
// draft.md asks, the warning, the header line and the refusal are all gone.
//
// f1 is §6.3.1's case — an `argued` record `cr record` stored as a question
// over its English statement prose. f2 is stored the same way but its summary
// already asks, and f3 is a `cited` finding whose statement §8.1.5 does not
// concern, so neither is named.
func TestDraftWarnsOverAQuestionBodyThatDoesNotAsk(t *testing.T) {
	forced := func(id string) *finding.Finding {
		record := aStoredRecord(id, finding.StateDraft)
		record.Kind, record.Grade = finding.KindQuestion, finding.GradeArgued
		return record
	}
	asking := forced("f2")
	asking.Summary = "Is the error Decode returns dropped?"
	layout := draftedHome(t, forced("f1"), asking, aCitedRecord("f3"))

	assert.Equal(t, []string{
		"record f1: its kind=question body holds no \"?\" character, and §8.1.5 has `cr post` refuse it; " +
			"rewrite the body in draft.md into the question it asks",
	}, draftWarnings(t))
	assert.Equal(t, []string{
		headerWaivers,
		"questions without \"?\": 1, which `cr post` refuses (§8.1.5): f1",
	}, headerTail(t, readDraft(t, layout)))

	_, err := runPost(t, draftPR, "--repo", draftSlug)
	var refused *render.BodyError
	require.ErrorAs(t, err, &refused, "cr post keeps §8.1.5's refusal")
	assert.Equal(t, "f1", refused.Record)
	assert.Equal(t, ExitValidation, exitCodeFor(err))

	file := readDraft(t, layout)
	statement := aStoredRecord("f1", finding.StateDraft).Summary
	require.Equal(t, 1, strings.Count(file, statement))
	writeDraft(t, layout, strings.Replace(file, statement, "Is the error Decode returns dropped, f1?", 1))

	assert.Equal(t, []string{}, draftWarnings(t))
	assert.Equal(t, []string{headerWaivers}, headerTail(t, readDraft(t, layout)))
	_, err = runPost(t, draftPR, "--repo", draftSlug)
	assert.NoError(t, err, "the rewritten question posts")
}

// Field-feedback 2.8 through `cr draft`: the header names the bodies that run
// past 1,200 characters, and only those.
//
// The limit is counted in characters. f2's body is exactly 1,200 of them and
// more than 1,200 bytes, since its evidence is a two-byte letter repeated, so
// a byte count would name it; f1's is one character longer and is named.
func TestTheDraftHeaderNamesBodiesOverTheLengthLimit(t *testing.T) {
	sized := func(id string, characters int) *finding.Finding {
		record := aCitedRecord(id)
		record.Evidence = strings.Repeat("ı", characters-len([]rune(record.Summary))-len("\n\n"))
		return record
	}
	layout := draftedHome(t, sized("f1", 1201), sized("f2", 1200))

	assert.Equal(t, []string{}, draftWarnings(t), "a long body is the header's to report, not a warning")
	assert.Equal(t, []string{
		headerWaivers,
		"long bodies: 1 over 1200 characters: f1",
	}, headerTail(t, readDraft(t, layout)))
}
