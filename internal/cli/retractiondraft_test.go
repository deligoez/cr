package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// noteRestingRound is detectedHome's round carrying one claim drawn from a note,
// and `cr record` run over f1 resting on that claim. f1 cites line 1 of lib.go,
// outside its unit, so §6.2 grades it `cited` and it may assert while the note
// stands. It returns the note's id.
func noteRestingRound(t *testing.T, layout state.Layout) string {
	t.Helper()
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.IssueKey = fixtureIssue
	recorded, err := note.Append(layout, fixtureIssue, "Load may panic during start-up only.",
		note.SourceMeeting, fixturePRNumber, time.Now())
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Write(state.FileClaims, []byte(`{"id":"`+fixtureIssue+`#c1",`+
		`"text":"Load may panic during start-up.","source":"note",`+
		`"span":"Load may panic during start-up only.","note_id":"`+recorded.ID+`",`+
		`"head":"`+meta.Head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())

	rests := confirming("f1", 4)
	delete(rests, "rule")
	rests["citations"] = []map[string]any{{"path": "lib.go", "line": 1}}
	rests["claim"] = fixtureIssue + "#c1"
	// A body §8.1.5 lets reach the payload in either register, so what
	// `cr post` reports below is the register and not a refused body.
	rests["summary"] = "Does Load panic where it should return an error?"
	_, err = runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", rests), "--repo", fixtureSlug)
	require.NoError(t, err)
	return recorded.ID
}

// storedRecord is the stored record bearing id.
func storedRecord(t *testing.T, layout state.Layout, id string) finding.Finding {
	t.Helper()
	stored := storedFindings(t, layout)
	for i := range stored {
		if stored[i].ID == id {
			return stored[i]
		}
	}
	require.Failf(t, "no stored record", "findings.ndjson holds no %s", id)
	return finding.Finding{}
}

// §3.6.6, §8.1.6 and round 9's retracted-provenance-still-posts through the
// commands: `cr record` stores f1 on a claim drawn from a note, the note is then
// withdrawn in the same round — retracted through `cr note --remove`, or gone
// from the store — and `cr post`, run before any redraft, builds f1 as a
// question, so the assertion cannot be sent. `cr draft` then holds f1 as a
// question and reports the forcing in its payload and in summary.json.
//
// Audit round 2's list-41-6: §8.1.6 makes no exception for a withdrawn note, so
// both the posted body and the draft still carry the provenance region naming
// the note, and the region says how it was withdrawn — retracted with its
// source, or absent from the store with no source to name.
//
// Before the withdrawal the same draft carries f1 as a cited finding with the
// region naming the standing note, which is what proves the withdrawal is what
// moved it.
func TestAWithdrawnNoteHoldsTheRecordRestingOnItAsAQuestion(t *testing.T) {
	for _, withdrawal := range []struct {
		name     string
		withdraw func(t *testing.T, layout state.Layout, id string)
		// noteLine is the region's note line once the note is withdrawn,
		// given the note's id.
		noteLine func(id string) string
	}{
		{name: "retracted by cr note remove", withdraw: func(t *testing.T, _ state.Layout, id string) {
			t.Helper()
			_, err := runIn(t, "note", "--remove", id)
			require.NoError(t, err)
		}, noteLine: func(id string) string {
			return "note: " + id + " (source: meeting; retracted)"
		}},
		{name: "no longer held by the store", withdraw: func(t *testing.T, layout state.Layout, _ string) {
			t.Helper()
			require.NoError(t, os.WriteFile(layout.ContextFile(fixtureIssue), nil, 0o600))
		}, noteLine: func(id string) string {
			return "note: " + id + " (not in the context store, so its source is unknown)"
		}},
	} {
		t.Run(withdrawal.name, func(t *testing.T) {
			layout := detectedHome(t)
			noteID := noteRestingRound(t, layout)
			regionNaming := func(noteLine string) string {
				return "<!-- cr:provenance -->\n" +
					"claim: " + fixtureIssue + "#c1 (source: note)\n" +
					noteLine + "\n" +
					"<!-- cr:/provenance -->"
			}

			before := blockOf(t, draftedFixture(t, layout), "f1")
			require.True(t, strings.HasPrefix(before, `kind="finding" `),
				"the control: a cited record on a standing note asserts")
			require.Equal(t, finding.GradeCited, storedRecord(t, layout, "f1").Grade)
			require.Equal(t, regionNaming("note: "+noteID+" (source: meeting)"), provenanceRegionOf(t, before),
				"the control: the region names the standing note")

			withdrawal.withdraw(t, layout, noteID)
			reported := finding.Withdrawn{{Class: "panic-in-library", Count: 1}}
			withdrawn := regionNaming(withdrawal.noteLine(noteID))

			// `cr post` first, before any redraft: findings.ndjson and the
			// draft's marker still say finding, so only post's own
			// application of §3.6.6 can hold the comment as a question.
			require.Equal(t, finding.KindFinding, storedRecord(t, layout, "f1").Kind)
			posted, err := runPost(t, fixturePR, "--repo", fixtureSlug)
			require.NoError(t, err)
			var built struct {
				Comments  []postedComment   `json:"comments"`
				Withdrawn finding.Withdrawn `json:"forced_by_retraction"`
				Payload   struct {
					Comments []struct {
						Body string `json:"body"`
					} `json:"comments"`
				} `json:"payload"`
			}
			require.NoError(t, json.Unmarshal([]byte(posted), &built))
			assert.Equal(t, []postedComment{{ID: "f1", Kind: finding.KindQuestion}}, built.Comments,
				"cr post cannot send the assertion")
			assert.Equal(t, reported, built.Withdrawn, "and reports the forcing")
			require.Len(t, built.Payload.Comments, 1)
			assert.Equal(t, withdrawn, provenanceRegionOf(t, built.Payload.Comments[0].Body),
				"§8.1.6: the posted body names the note and how it was withdrawn")

			printed, err := runDraft(t, fixturePR, "--repo", fixtureSlug)
			require.NoError(t, err)
			var drafted struct {
				Forced    finding.Forcings  `json:"forced_to_question"`
				Withdrawn finding.Withdrawn `json:"forced_by_retraction"`
			}
			require.NoError(t, json.Unmarshal([]byte(printed), &drafted))
			assert.Equal(t, reported, drafted.Withdrawn, "§3.6.6: the forcing is reported per class")
			assert.Equal(t, finding.Forcings{}, drafted.Forced, "and not as §6.3's, whose grade it did not change")

			var summarised finding.Withdrawn
			require.NoError(t, json.Unmarshal(readSummaryOf(t, layout)["forced_by_retraction"], &summarised))
			assert.Equal(t, reported, summarised, "the round summary carries the same count")

			stored := storedRecord(t, layout, "f1")
			assert.Equal(t, finding.KindQuestion, stored.Kind, "the record is held as a question")
			assert.Equal(t, finding.GradeCited, stored.Grade, "and keeps the grade §6.2 computed")

			after := blockOf(t, readDraftOf(t, layout), "f1")
			assert.True(t, strings.HasPrefix(after, `kind="question" `), "the draft's marker is a question")
			assert.Equal(t, withdrawn, provenanceRegionOf(t, after),
				"§8.1.6: the draft names the note and how it was withdrawn")

			disclosed := "§3.6.6: 1 records resting on a withdrawn note held as question — panic-in-library 1"
			for _, command := range []string{"draft", "post"} {
				lines := strings.Split(throughATerminal(t, command, fixturePR, "--repo", fixtureSlug), "\n")
				assert.Containsf(t, lines, disclosed, "§11.1: cr %s prints the report on its own line", command)
			}
		})
	}
}

// provenanceRegionOf is the one §8.1.6 provenance region text carries, from its
// opening delimiter through its closing one, or the empty string when it
// carries none.
func provenanceRegionOf(t *testing.T, text string) string {
	t.Helper()
	const open, closing = "<!-- cr:provenance -->", "<!-- cr:/provenance -->"
	require.LessOrEqual(t, strings.Count(text, open), 1, "a comment carries at most one provenance region")
	start := strings.Index(text, open)
	if start < 0 {
		return ""
	}
	length := strings.Index(text[start:], closing)
	require.GreaterOrEqual(t, length, 0, "a region that opens closes by its own pair")
	return text[start : start+length+len(closing)]
}

// readSummaryOf reads the fixture round's summary.json as its raw fields.
func readSummaryOf(t *testing.T, layout state.Layout) map[string]json.RawMessage {
	t.Helper()
	body, err := os.ReadFile(
		layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileSummary))
	require.NoError(t, err)
	var summary map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &summary))
	return summary
}

// readDraftOf reads the fixture round's draft.md without running `cr draft`.
func readDraftOf(t *testing.T, layout state.Layout) string {
	t.Helper()
	written, err := os.ReadFile(
		layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft))
	require.NoError(t, err)
	return string(written)
}
