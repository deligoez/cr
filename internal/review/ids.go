package review

import (
	"maps"
	"slices"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/role"
)

// idBlock is how many record ids one prompt's block holds.
//
// A block is one role's ids for one unit. A hundred is five times the default
// post.max_comments of twenty, which caps a whole round's review, so a role
// writing more records on one unit than the round could post is not a role
// given too few. A role that runs past its block writes into the next prompt's,
// and where that prompt's role wrote the same id `cr merge` refuses the repeat
// with exit code 1 (§6.1): an overrun is refused, never recorded under another
// record's id.
const idBlock = 100

// IDs is the run of §6.1 record ids one prompt's role may write for its unit.
type IDs struct {
	// First is the block's first id past every id a stored record holds
	// in it. It is past Last when the block's last id is held.
	First int
	// Last is the block's last id.
	Last int
}

// left reports whether the block still holds an id past the stored ones.
func (b IDs) left() bool {
	return b.First <= b.Last
}

// spelled names the run as the prompt's JSON carries it, and as two empty
// strings when no id past the stored ones is left.
func (b IDs) spelled() (first, last string) {
	if !b.left() {
		return "", ""
	}
	return finding.IDOf(b.First), finding.IDOf(b.Last)
}

// idBase is the highest id held by a record an earlier round stored for the
// pull request, which every block of this round starts past.
//
// §6.1 makes an id stable for the life of the pull request, so an earlier
// round's ids are spent. This round's own records are left out on purpose: they
// sit inside the blocks this round hands out, and counting them would move every
// block the moment `cr record` stored the first file of a round whose other roles
// are still writing, giving a later `cr review` of the same round blocks that
// overlap the ids an earlier emission gave.
func (r *Round) idBase() int {
	base := 0
	for i := range r.Held {
		if r.Held[i].Round == r.Round {
			continue
		}
		if n, ok := finding.IDSuffix(r.Held[i].ID); ok && n > base {
			base = n
		}
	}
	return base
}

// ids is the block of the prompt for the role over the unit at index at.
//
// Its place is the prompt's place in a grid of Round.Places times every unit in
// id order, and never its place in this invocation's emission or in the active
// set: `--axis` and §4.6.5's second pass give a prompt the block the full
// fan-out gives it, and a re-brief that changes the active roles moves no block
// an earlier emission gave out, so no two prompts of the round share an id
// however the passes are run. Within the block the first id is past every id a
// stored record already holds there, so a prompt emitted again after `cr record`
// stored its role's records does not hand those ids out a second time.
//
// Round.Roles is always drawn from the corpus Round.Places is built over, which
// is what makes the role's index there its place.
func (r *Round) ids(base int, roleID string, at int) IDs {
	place := slices.Index(r.Places, roleID)*len(r.Units) + at
	block := IDs{First: base + place*idBlock + 1}
	block.Last = block.First + idBlock - 1
	for i := range r.Held {
		if n, ok := finding.IDSuffix(r.Held[i].ID); ok && n >= block.First && n <= block.Last {
			block.First = n + 1
		}
	}
	return block
}

// blockPlaces is the order a round's rows of id blocks are laid out in: every
// role id cr ships, ascending, and then every other role id the corpus
// resolved, ascending.
//
// It is neither the active set nor §2.5.5's corpus order, because a re-brief of
// the same round can change both, and a block that moves after an emission gave
// it out hands a later emission ids the earlier one's roles already wrote. The
// active set changes whenever a role joins or leaves it; the corpus order puts
// per-repository and global files before the built-ins, so a role file added
// between two emissions would move every built-in role's row. A shipped id keeps
// its row whichever layer resolves it, and a role file joining the corpus moves
// only the other non-shipped rows whose ids sort after its own.
func blockPlaces(corpus []role.Resolved) []string {
	places := slices.Sorted(maps.Keys(role.Builtins()))
	shipped := len(places)
	for i := range corpus {
		if !slices.Contains(places[:shipped], corpus[i].Role.ID) {
			places = append(places, corpus[i].Role.ID)
		}
	}
	slices.Sort(places[shipped:])
	return places
}

// idBlockLine tells the role which ids its records take: the prompt's block, in
// order, with what happens to an id outside it.
//
// Every role starting at f1 is what this replaces. Parallel roles each told only
// §6.1's spelling write the same first id, and `cr merge` refuses the repeat, so
// the block is what lets the fan-out's files merge at all.
func idBlockLine(p *page, round int, ids IDs) {
	if !ids.left() {
		p.line("This prompt's block of ids ends at %s, which a record stored for this pull request "+
			"already holds, so no id past the stored ones is left for a record written here; cr merge "+
			"refuses an id another record holds, with exit code 1 (§6.1).", finding.IDOf(ids.Last))
		return
	}
	first, last := ids.spelled()
	p.line("Give the records you write here the ids %s through %s, in order from %s. No other prompt "+
		"of round %d is given any of them, and none is held by a stored record. An id outside them may "+
		"be another prompt's, and cr merge refuses an id two records carry, with exit code 1 (§6.1).",
		first, last, first, round)
}
