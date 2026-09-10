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
	cells, err := coverage.Decode("cells.ndjson", []byte(line+"\n"), []string{"u1", "u2", "u3"}, intentRoles())
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

