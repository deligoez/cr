package review

import (
	"slices"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
)

// IntentRound is what one round's unmapped-unit items are raised from: the
// units, the mapping, the issue key's notes, and the cells the agent has
// recorded so far.
//
// Every file is handed in whole rather than narrowed by the caller, and each
// is narrowed here by the rule that governs it. The pairs and the cells are
// round-scoped (§9.3.5), so they are filtered on Round. The notes are not:
// §9.3.5 exempts the context store from round scoping, and note.StandingOf
// reads an id missing from a narrowed slice as dangling, so a caller that
// trimmed the store would un-explain a unit nobody retracted a note for.
type IntentRound struct {
	// Round is the round the items are raised for.
	Round int
	// Units are the round's unit ids, in §3.4.6's order.
	Units []string
	// Pairs is mapping.ndjson whole.
	Pairs []mapping.Pair
	// Notes is every note the issue key's store holds, retracted ones
	// included.
	Notes []note.Note
	// Cells is coverage.ndjson whole.
	Cells []coverage.Cell
	// IntentRoles are the ids of the round's active roles on the intent
	// axis, which are the roles §4.1.5's decision belongs to.
	IntentRoles []string
}

// Raise is §4.1.2 and §4.1.5 over one round: an item for every unit the round's
// mapping maps to no claim, carrying the issue key's notes, except the units a
// note has already been recorded as explaining.
//
// The decision is read, never made. §4.1.5 gives it to the agent — "the agent
// decides whether a note explains the unit" — and forbids cr to decide it by
// matching text, and §2.1.3 lists it among the six judgements cr never makes.
// What reaches cr is the decision itself, as a coverage cell recorded through
// `cr cells record` with `note_id` naming the note, and that is the only thing
// that stops an item from being raised. A note whose words happen to describe
// the unit exactly suppresses nothing until a role says so.
//
// The notes attached are the ones that still stand. A retracted note is revoked
// hearsay per §3.6.6, and offering it as a reason to withhold a question would
// hand the agent a fact the human has already withdrawn.
func Raise(r *IntentRound) []UnmappedUnit {
	standing := standingNotes(r.Notes)
	items := Unmapped(r.Units, r.Pairs, r.Round)
	raised := make([]UnmappedUnit, 0, len(items))
	for _, item := range items {
		if r.explained(item.Unit) {
			continue
		}
		item.Notes = standing
		raised = append(raised, item)
	}
	return raised
}

// explained reports whether an intent role has recorded, for this round, a
// cell at unit that cites a note which still stands.
//
// All three conditions are the decision's own. The round, because a cell of an
// earlier round sat at a different piece of code under the same id (§3.4.6).
// The intent axis, because §4.1.5's item is raised by the intent role and it is
// that role's cell §4.1.5 has cite the note; a correctness cell carrying a
// `note_id` is some other lens's remark, not the decision this reads. And the
// note's standing, because §3.6.6 keeps a note revocable: a cell citing one
// that was retracted, or one the store never held, rests on nothing, and the
// question it withheld is raised again rather than silently kept away. A cell
// with no `note_id` at all is the last of those: an empty id names no note in
// the store, so note.StandingOf reports it dangling like any other.
func (r *IntentRound) explained(unit string) bool {
	for i := range r.Cells {
		cell := &r.Cells[i]
		if cell.Round != r.Round || cell.Unit != unit {
			continue
		}
		if !slices.Contains(r.IntentRoles, cell.Role) {
			continue
		}
		if note.StandingOf(r.Notes, cell.NoteID).Stands() {
			return true
		}
	}
	return false
}

// standingNotes is the store narrowed to the notes a role may still rest a
// decision on, in the order they were recorded. It is empty rather than nil
// when none stands, per §12.
func standingNotes(notes []note.Note) []note.Note {
	standing := make([]note.Note, 0, len(notes))
	for i := range notes {
		if notes[i].Standing().Stands() {
			standing = append(standing, notes[i])
		}
	}
	return standing
}
