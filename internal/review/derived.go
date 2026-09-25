package review

import (
	"slices"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// kindsOf is §4.6.7 over the round's units: the kind the resolved profile gives
// each unit that has one, by unit id.
func kindsOf(p *profile.Profile, units []Unit) map[string]profile.UnitKind {
	kinds := make(map[string]profile.UnitKind)
	for i := range units {
		if kind, found := p.KindOf(units[i].Path); found {
			kinds[units[i].ID] = kind
		}
	}
	return kinds
}

// withheld reports whether §4.6.7 or §4.6.8 withholds the role's prompt over
// the unit: the unit is a twin, whose prompts are its earlier unit's, or it is
// of a kind whose entry does not list the role. Neither `--all` nor §4.6.5's
// second pass lifts it — both are about which held cells to emit again, and a
// withheld cell has no prompt to emit.
func (r *Round) withheld(roleID, unitID string) bool {
	if at := slices.IndexFunc(r.Units, func(u Unit) bool { return u.ID == unitID }); at >= 0 && r.Units[at].TwinOf != "" {
		return true
	}
	kind, found := r.Kinds[unitID]
	return found && !slices.Contains(kind.Roles, roleID)
}

// twins are the round's §4.6.8 pairs, as coverage.TwinCells reads them.
func (r *Round) twins() []coverage.Twin {
	twins := make([]coverage.Twin, 0)
	for i := range r.Units {
		if r.Units[i].TwinOf != "" {
			twins = append(twins, coverage.Twin{Unit: r.Units[i].ID, Hash: r.Units[i].Hash, Of: r.Units[i].TwinOf})
		}
	}
	return twins
}

// twinsOf are the ids of the units that repeat the unit's hunks.
func (r *Round) twinsOf(unitID string) []string {
	ids := make([]string, 0)
	for i := range r.Units {
		if r.Units[i].TwinOf == unitID {
			ids = append(ids, r.Units[i].ID)
		}
	}
	return ids
}

// recordDerived records the cells §4.6.7 and §4.6.8 have cr fill itself and
// the round does not hold yet: an `na` at every active role a unit's kind does
// not list, and at every twin the result held at its earlier unit, including a
// kind's `na` recorded in the same call.
//
// It is here, where `cr review` has already read the round's units, its active
// roles and its cells, and it writes through the same stamped, keyed writer
// `cr cells record` does, so a derived cell is replaced by a role's own exactly
// as any other cell is. The cells are the round's, not the invocation's: an
// `--axis` or `--units` narrowing changes which prompts are emitted and not
// which cells a kind or a twin settles.
func (r *Round) recordDerived(src *Sources) error {
	derived := make([]*coverage.Cell, 0)
	for i := range r.Units {
		u := &r.Units[i]
		kind, found := r.Kinds[u.ID]
		if !found {
			continue
		}
		for _, id := range r.Active {
			if !slices.Contains(kind.Roles, id) && !r.holds(id, u.ID) {
				derived = append(derived, coverage.KindCell(u.ID, u.Hash, id, kind.Kind))
			}
		}
	}
	held := make([]*coverage.Cell, 0, len(r.Cells)+len(derived))
	for i := range r.Cells {
		if r.Cells[i].Head == r.Head {
			held = append(held, &r.Cells[i])
		}
	}
	for _, copied := range coverage.TwinCells(append(held, derived...), r.twins()) {
		if !r.holds(copied.Role, copied.Unit) && !slices.ContainsFunc(derived, func(cell *coverage.Cell) bool {
			return cell.Unit == copied.Unit && cell.Role == copied.Role
		}) {
			derived = append(derived, copied)
		}
	}
	if len(derived) == 0 {
		return nil
	}
	lock, err := src.Layout.LockPR(src.Owner, src.Repo, src.PR)
	if err != nil {
		return err
	}
	stamp := state.Stamp{Head: r.Head, Round: r.Round}
	if err := state.ReplaceStampedKeys(lock, state.FileCoverage, stamp, derived, coverage.KeyFields()); err != nil {
		// The lock is released on the way out of every branch, and the
		// write's own failure is what the caller is told about.
		_ = lock.Unlock()
		return err
	}
	if err := lock.Unlock(); err != nil {
		return err
	}
	for _, cell := range derived {
		r.Cells = append(r.Cells, *cell)
	}
	return nil
}
