package review

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// Shard is `--shard <k>/<n>` of §4.6.1: the k-th of n contiguous parts of the
// round's units in ascending numeric order of id. The command line settles
// that 1 <= K <= N before one reaches Run.
type Shard struct {
	K int
	N int
}

// UnknownUnitError reports a `--units` id that is not a unit of the current
// round, which §4.6.1 rejects with exit code 1.
type UnknownUnitError struct {
	// Unit is the id as it was given.
	Unit string
	// Round is the round the id was looked for in, and Units its unit ids
	// in ascending numeric order.
	Round int
	Units []string
}

func (e *UnknownUnitError) Error() string {
	return fmt.Sprintf("--units names %q, which is not a unit of round %d; §4.6.1 narrows the prompts to units "+
		"of the current round, which are %s", e.Unit, e.Round, listed(e.Units))
}

// byNumber orders unit ids by the number §3.4.6's u<n> carries, so u2 comes
// before u10. An id of another form sorts after every numbered one, by text.
func byNumber(a, b string) int {
	na, errA := strconv.Atoi(strings.TrimPrefix(a, "u"))
	nb, errB := strconv.Atoi(strings.TrimPrefix(b, "u"))
	switch {
	case errA == nil && errB == nil:
		return cmp.Compare(na, nb)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return cmp.Compare(a, b)
}

// pickUnits is §4.6.1's `--units` and `--shard` over the round's unit ids: the
// ids the invocation narrows its prompts to, and nil when it names neither.
//
// It narrows prompts only. §4.6.3's expected cells stay the round's whole set,
// and the fan-out directories are made for every unit.
func (s *Sources) pickUnits(round int, ids []string) ([]string, error) {
	ordered := slices.SortedFunc(slices.Values(ids), byNumber)
	if s.Units != nil {
		for _, id := range s.Units {
			if !slices.Contains(ids, id) {
				return nil, &UnknownUnitError{Unit: id, Round: round, Units: ordered}
			}
		}
		return append(make([]string, 0, len(s.Units)), s.Units...), nil
	}
	if s.Shard == nil {
		return nil, nil
	}
	picked := make([]string, 0)
	for i, id := range ordered {
		if i*s.Shard.N/len(ordered)+1 == s.Shard.K {
			picked = append(picked, id)
		}
	}
	return picked, nil
}

// holds reports whether the round holds the cell at its head (§4.6.3's
// `recorded`).
func (r *Round) holds(roleID, unitID string) bool {
	return slices.ContainsFunc(r.Cells, func(held coverage.Cell) bool {
		return held.Unit == unitID && held.Role == roleID && held.Head == r.Head
	})
}

// staleByNote is §4.6.1's note condition for one cell: its latest line in
// emissions.ndjson precedes a standing note of the issue key that the line does
// not carry, that the held cell does not cite in `note_id`, and that is on the
// cell's unit.
//
// The latest line and the notes it missed are read the way NotesAfter reads a
// record's prompt, and the note's place is the same link read at the unit
// rather than at a record.
func (r *Round) staleByNote(roleID, unitID string) bool {
	line := latestEmission(r.Emitted, state.Stamp{Head: r.Head, Round: r.Round}, roleID, unitID,
		func(*Emission) bool { return true })
	if line == nil {
		return false
	}
	cited := r.citedNote(roleID, unitID)
	records := r.roundRecords()
	return len(missedNotes(line, r.Notes, func(recorded *note.Note) bool {
		return recorded.ID != cited &&
			linkOf(recorded, r.Claims, records, r.PR).onUnit(unitID, r.Pairs, r.Round)
	})) > 0
}

// citedNote is the `note_id` of the latest line the round holds for the cell
// at its head, and empty when that line cites no note or there is none.
func (r *Round) citedNote(roleID, unitID string) string {
	cited := ""
	for i := range r.Cells {
		held := &r.Cells[i]
		if held.Unit == unitID && held.Role == roleID && held.Head == r.Head {
			cited = held.NoteID
		}
	}
	return cited
}

// roundRecords is the stored records of the round, which a note answering a
// record is looked up among.
func (r *Round) roundRecords() []*finding.Finding {
	records := make([]*finding.Finding, 0, len(r.Held))
	for i := range r.Held {
		if r.Held[i].Round == r.Round {
			records = append(records, &r.Held[i])
		}
	}
	return records
}

// emits reports whether the role's prompt over the unit is emitted on this
// invocation.
//
// `--units` and `--shard` narrow every pass to the units they pick. §4.6.5's
// second intent pass then re-emits one prompt per unit the round's mapping maps
// to zero claims, and §4.6.1's default narrowing does not apply to it: which
// units those are is mapping.ClaimsOf's answer rather than Round.Unmapped's,
// because §4.1.5 leaves an item unraised once a note explains the unit, and
// that unit is still one the mapping maps to nothing — dropping it here would
// take away the prompt carrying the note the agent is meant to weigh.
//
// Every other pass emits, without `--all`, only a cell the round does not hold
// at its head or one the note condition makes stale.
func (r *Round) emits(roleID, unitID string) bool {
	if r.Picked != nil && !slices.Contains(r.Picked, unitID) {
		return false
	}
	if r.SecondPass {
		return len(mapping.ClaimsOf(r.Pairs, r.Round, unitID)) == 0
	}
	return r.All || !r.holds(roleID, unitID) || r.staleByNote(roleID, unitID)
}
