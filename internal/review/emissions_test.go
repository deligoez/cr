package review

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// at is a fixed moment plus minutes, so every timestamp below is ordered by
// construction.
func at(minutes int) time.Time {
	return time.Date(2026, 9, 16, 10, minutes, 0, 0, time.UTC)
}

// linesOf is what a run of Run should have appended: one line per prompt,
// stamped with the round and head, the pass, the moment and the notes.
func linesOf(fan *Fanout, pass string, emitted time.Time, notes []string) []Emission {
	lines := make([]Emission, 0, len(fan.Prompts))
	for i := range fan.Prompts {
		lines = append(lines, Emission{
			Stamp: state.Stamp{Head: fan.Head, Round: fan.Round},
			Pass:  pass, Axis: fan.Prompts[i].Axis, Role: fan.Prompts[i].Role, Unit: fan.Prompts[i].Unit,
			EmittedAt: emitted, Notes: notes,
		})
	}
	return lines
}

// `cr review` appends one line per prompt it emits to emissions.ndjson, each
// carrying the round, the head, the pass, the moment and the ids of the notes
// the prompt carried; a second run appends its own lines after the first's and
// names its own pass; and a round nothing emitted for reads as none
// (field-feedback 1.5).
func TestAReviewAppendsOneEmissionLinePerPrompt(t *testing.T) {
	src := briefed(t)
	src.Clock = func() time.Time { return at(1) }
	full, err := Run(src)
	require.NoError(t, err)
	require.NotEmpty(t, full.Prompts)

	src.Axis, src.Clock = axis.Intent, func() time.Time { return at(2) }
	second, err := Run(src)
	require.NoError(t, err)
	require.NotEmpty(t, second.Prompts)

	carried := []string{runIssue + "#n1"}
	read, err := ReadEmissions(src.Layout, runOwner, runRepo, runPR, 1)
	require.NoError(t, err)
	assert.Equal(t,
		append(linesOf(full, PassAll, at(1), carried), linesOf(second, axis.Intent, at(2), carried)...), read)

	other, err := ReadEmissions(src.Layout, runOwner, runRepo, runPR, 2)
	require.NoError(t, err)
	assert.Equal(t, []Emission{}, other, "a line of round 1 is not round 2's")
}

// A pull request no `cr review` has emitted for reads as no emission, and a
// line cr cannot decode is refused naming the file.
func TestEmissionsReadAsNoneWhenAbsentAndRefuseAnUnusableLine(t *testing.T) {
	src := briefed(t)
	none, err := ReadEmissions(src.Layout, runOwner, runRepo, runPR, 1)
	require.NoError(t, err)
	assert.Equal(t, []Emission{}, none)

	held, err := src.Layout.LockPR(runOwner, runRepo, runPR)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileEmissions, []byte("{not json\n")))
	require.NoError(t, held.Unlock())
	_, err = ReadEmissions(src.Layout, runOwner, runRepo, runPR, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), src.Layout.PRFile(runOwner, runRepo, runPR, state.FileEmissions))
}

// A run that emits no prompt writes no line and creates no file.
func TestAReviewEmittingNoPromptWritesNoEmission(t *testing.T) {
	src := briefed(t)
	r := &Round{Round: 1, Head: "h"}
	require.NoError(t, r.recordEmissions(src, []Prompt{}))
	_, err := os.Stat(src.Layout.PRFile(runOwner, runRepo, runPR, state.FileEmissions))
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// An emission cr cannot append fails the review rather than being dropped:
// the lines are how `cr draft` and `cr status` tell which prompts a note
// postdates, so a run that lost them would report a prompt as carrying notes
// it never saw. A directory where the file belongs makes the append fail.
func TestAnEmissionThatCannotBeAppendedFailsTheReview(t *testing.T) {
	src := briefed(t)
	require.NoError(t, os.MkdirAll(src.Layout.PRFile(runOwner, runRepo, runPR, state.FileEmissions), 0o700))
	r := &Round{Round: 1, Head: "h"}

	require.Error(t, r.recordEmissions(src, []Prompt{{Role: "correctness", Unit: "u1", Axis: axis.Correctness}}))
}

// emission is one line of role over unit, in round 1 at head h, emitted at
// the minute given and carrying the notes given.
func emission(pass, role, unitID string, minute int, notes ...string) Emission {
	return Emission{
		Stamp: state.Stamp{Head: "h", Round: 1}, Pass: pass, Axis: role, Role: role, Unit: unitID,
		EmittedAt: at(minute), Notes: append([]string{}, notes...),
	}
}

// A note postdates every run that emitted before it and did not carry it, one
// entry per run, and no run emitted after it or carrying it.
func TestANotePostdatesEveryEarlierRunThatDidNotCarryIt(t *testing.T) {
	emitted := []Emission{
		emission(PassAll, "correctness", "u1", 1), emission(PassAll, "convention", "u1", 1),
		emission(PassAll, "correctness", "u2", 1),
		emission(axis.Intent, "intent-coverage", "u1", 2),
		emission(PassAll, "correctness", "u1", 3, "CR-7#n2"),
		emission(PassAll, "correctness", "u1", 6),
	}
	recorded := note.Note{ID: "CR-7#n2", RecordedAt: at(5)}

	assert.Equal(t, []Postdated{
		{Round: 1, Head: "h", Pass: PassAll, EmittedAt: at(1), Roles: []string{"correctness", "convention"}, Prompts: 3},
		{Round: 1, Head: "h", Pass: axis.Intent, EmittedAt: at(2), Roles: []string{"intent-coverage"}, Prompts: 1},
	}, PassesBefore(emitted, &recorded))
	assert.Equal(t, []Postdated{}, PassesBefore(emitted, &note.Note{ID: "CR-7#n3", RecordedAt: at(0)}),
		"a note recorded before every run postdates none")
}

// record is a stored record of role over unit in round 1 at head h.
func record(id, role, unitID, claim string) *finding.Finding {
	return &finding.Finding{ID: id, Role: role, Unit: unitID, Claim: claim, Stamp: state.Stamp{Head: "h", Round: 1}}
}

// NotesAfter names a record when a standing note on its claim or unit was
// recorded after the latest emission of its role and unit before the record was
// stored, and that emission did not carry the note. Every other combination
// below leaves the record out.
func TestNotesAfterNamesOnlyRecordsWhosePromptANoteOnTheirClaimOrUnitPostdates(t *testing.T) {
	retracted := at(9)
	notes := []note.Note{
		{ID: "CR-7#n1", RecordedAt: at(0)},                          // before every prompt
		{ID: "CR-7#n2", RecordedAt: at(5)},                          // no link
		{ID: "CR-7#n3", RecordedAt: at(5)},                          // drawn into claim c9
		{ID: "CR-7#n4", RecordedAt: at(5), PR: 7, Record: "f10"},    // answers f10 on u2
		{ID: "CR-7#n5", RecordedAt: at(5), RetractedAt: &retracted}, // no longer stands
		{ID: "CR-7#n6", RecordedAt: at(5), PR: 8, Record: "f10"},    // answers another pull request's f10
	}
	claims := []intent.Claim{{ID: "CR-7#c9", Source: intent.ClaimFromNote, NoteID: "CR-7#n3"}}
	emitted := []Emission{
		emission(PassAll, "correctness", "u1", 1, "CR-7#n1"),
		emission(PassAll, "correctness", "u2", 1, "CR-7#n1"),
		emission(PassAll, "convention", "u1", 1, "CR-7#n1"),
		emission(PassAll, "convention", "u1", 7, "CR-7#n1", "CR-7#n2", "CR-7#n3", "CR-7#n4", "CR-7#n6"),
		{Stamp: state.Stamp{Head: "other", Round: 1}, Pass: PassAll, Role: "intent-coverage", Unit: "u1", EmittedAt: at(1)},
		// A prompt that did not carry n1, although n1 was recorded before
		// it, and did carry n4, stamped after it by a clock that ran ahead:
		// only a note both recorded after the prompt and not carried counts.
		emission(PassAll, "convention", "u2", 4, "CR-7#n4"),
	}
	records := []*finding.Finding{
		record("f1", "correctness", "u1", ""),        // n2 and n6
		record("f2", "correctness", "u1", "CR-7#c9"), // n2, n3 and n6
		record("f10", "correctness", "u2", ""),       // n2, n4 and n6
		record("f11", "convention", "u1", ""),        // stored after the re-emission carrying the notes
		record("f12", "convention", "u1", ""),        // stored before it
		record("f13", "intent-coverage", "u1", ""),   // only another head's emission
		record("f14", "convention", "u2", ""),        // n2 and n6, not n1 or n4
	}
	stored := map[string]time.Time{"f11": at(8), "f12": at(6)}

	report := NotesAfter(&PromptNotes{
		Emissions: emitted, Notes: notes, Claims: claims, Records: records, RecordedAt: stored, PR: 7,
	})

	assert.Equal(t, NotesAfterPrompts{
		Count: 5,
		Records: []RecordBeforeNotes{
			{Record: "f1", Notes: []string{"CR-7#n2", "CR-7#n6"}},
			{Record: "f2", Notes: []string{"CR-7#n2", "CR-7#n3", "CR-7#n6"}},
			{Record: "f10", Notes: []string{"CR-7#n2", "CR-7#n4", "CR-7#n6"}},
			{Record: "f12", Notes: []string{"CR-7#n2", "CR-7#n6"}},
			{Record: "f14", Notes: []string{"CR-7#n2", "CR-7#n6"}},
		},
		Unattributed: []string{"f13"},
	}, report)

	assert.Equal(t, NotesAfterPrompts{
		Count:        1,
		Records:      []RecordBeforeNotes{{Record: "f10", Notes: []string{"CR-7#n2", "CR-7#n4", "CR-7#n6"}}},
		Unattributed: []string{"f13"},
	}, report.Only([]string{"f10", "f11", "f13"}))

	assert.Equal(t, NotesAfterPrompts{Records: []RecordBeforeNotes{}, Unattributed: []string{}},
		NotesAfter(&PromptNotes{Emissions: emitted, Notes: notes[4:5], Records: records, PR: 7}),
		"with no standing note nothing is reported, not even a record with no emission")
}
