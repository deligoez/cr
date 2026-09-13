package review

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/role"
)

// The issue key's notes, as the store returns them: one that stands, and one a
// human retracted per §3.6.6.
func storedNotes() []note.Note {
	retracted := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return []note.Note{
		{ID: "CR-1#n1", Text: "the retry in u2 is deliberate", Source: note.SourceChat, PR: 7},
		{ID: "CR-1#n2", Text: "the cache in u3 was agreed in standup", Source: note.SourceMeeting, PR: 7,
			RetractedAt: &retracted},
	}
}

// intentRoles are the round's active roles: one on the intent axis, whose cell
// §4.1.5 reads, and one beside it, whose cell it does not.
func intentRoles() []role.Role {
	return []role.Role{
		{ID: "intent-coverage", Title: "Intent coverage", Axis: axis.Intent},
		{ID: "correctness", Title: "Correctness", Axis: axis.Correctness},
	}
}

// recorded is what `cr cells record` stores for one line an agent wrote: the
// cell coverage.Decode accepted, with the round the writer stamps onto it.
func recorded(t *testing.T, round int, line string) coverage.Cell {
	t.Helper()
	cells, err := coverage.Decode("cells.ndjson", []byte(line+"\n"), []string{"u1", "u2", "u3"}, intentRoles(), nil)
	require.NoError(t, err)
	require.Len(t, cells, 1)
	cells[0].Round = round
	return *cells[0]
}

// intentRound is round 2 of a pull request whose mapping maps u1 and leaves u2
// and u3 unmapped. It is round 2 rather than 1 so a round-1 cell has a round of
// its own to have been recorded in.
func intentRound(cells ...coverage.Cell) *IntentRound {
	return &IntentRound{
		Round:       2,
		Units:       []string{"u1", "u2", "u3"},
		Pairs:       []mapping.Pair{stored("CR-1#c1", "u1", 2)},
		Notes:       storedNotes(),
		Cells:       cells,
		IntentRoles: []string{"intent-coverage"},
	}
}

// raisedUnits names the units a round raised items for.
func raisedUnits(items []UnmappedUnit) []string {
	units := make([]string, 0, len(items))
	for _, item := range items {
		units = append(units, item.Unit)
	}
	return units
}

// cr attaches the issue key's notes to every unmapped unit (§4.1.5), and only
// the notes that still stand.
func TestTheIssueKeysNotesAreAttachedToEveryUnmappedUnit(t *testing.T) {
	items := Raise(intentRound())

	require.Equal(t, []string{"u2", "u3"}, raisedUnits(items))
	for _, item := range items {
		require.Len(t, item.Notes, 1, "every unmapped unit carries the store's standing notes")
		assert.Equal(t, "CR-1#n1", item.Notes[0].ID,
			"§3.6.6: the retracted note is withdrawn hearsay and is not offered as a reason")
	}
}

// A recorded note suppresses the matching unmapped-unit question only when the
// agent says so, and the cell that says so carries the note's id.
//
// n1's text names u2 in so many words, which is the case a text match would
// take as settled. It is not: with the note recorded and no decision recorded,
// u2 is raised all the same, because §4.1.5 forbids cr to decide by matching
// text and §2.1.3 gives the decision to the agent. Only once the intent role's
// cell arrives through the decoder `cr cells record` uses, citing n1, does u2
// stop being raised — and u3, which nobody explained, is raised throughout.
func TestARecordedNoteSuppressesTheQuestionOnlyWhenTheAgentSaysSo(t *testing.T) {
	assert.Equal(t, []string{"u2", "u3"}, raisedUnits(Raise(intentRound())),
		"a note whose words describe the unit suppresses nothing on its own")

	decided := recorded(t, 2, `{"unit":"u2","role":"intent-coverage","result":"pass","note_id":"CR-1#n1"}`)
	assert.Equal(t, "CR-1#n1", decided.NoteID, "§4.1.5: the coverage cell cites the note id")

	assert.Equal(t, []string{"u3"}, raisedUnits(Raise(intentRound(decided))),
		"once the agent records that n1 explains u2, u2's question is not raised")
}

// A cell suppresses an item only when it is the intent role's decision, for this
// round, resting on a note that still stands.
//
// Each case below is a cell sitting at u2 that is one condition short of that,
// and each leaves u2's question raised. Suppressing on any of them would
// withhold a question on the strength of something other than the decision
// §4.1.5 describes: another lens's remark, a decision about a unit of the same
// id in an earlier round, a note §3.6.6 has revoked, or a note that never was.
func TestACellShortOfTheIntentRolesStandingDecisionSuppressesNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		round int
		line  string
	}{
		{"another role's cell", 2,
			`{"unit":"u2","role":"correctness","result":"question","note_id":"CR-1#n1"}`},
		{"an earlier round's cell", 1,
			`{"unit":"u2","role":"intent-coverage","result":"pass","note_id":"CR-1#n1"}`},
		{"a retracted note", 2,
			`{"unit":"u2","role":"intent-coverage","result":"pass","note_id":"CR-1#n2"}`},
		{"a note the store never held", 2,
			`{"unit":"u2","role":"intent-coverage","result":"pass","note_id":"CR-1#n9"}`},
		{"no note at all", 2,
			`{"unit":"u2","role":"intent-coverage","result":"question"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cell := recorded(t, tc.round, tc.line)

			assert.Equal(t, []string{"u2", "u3"}, raisedUnits(Raise(intentRound(cell))))
		})
	}
}
