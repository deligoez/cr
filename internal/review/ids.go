package review

import (
	"maps"
	"slices"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/proposal"
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

// Start is the block's first id, whether a stored record holds it or not.
func (b IDs) Start() int {
	return b.Last - idBlock + 1
}

// Holds reports whether n is one of the block's hundred ids, stored or not.
func (b IDs) Holds(n int) bool {
	return n >= b.Start() && n <= b.Last
}

// BlockOf is the block `cr review` gives the prompt for roleID over unitID in
// round, for a command that names an id inside a prompt's block rather than
// emitting the prompt: held is every record the pull request stores, corpus is
// §2.5.5's resolved corpus, and units are the round's unit ids in id order. It
// is Round.ids over the same inputs, so the block is the one that prompt
// carried. It is false when the corpus resolves no such role or the round holds
// no such unit, for which no prompt was given a block.
//
// The Round it builds holds units only for their count, which is all ids reads
// of them.
func BlockOf(held []finding.Finding, round int, corpus []role.Resolved, units []string, roleID, unitID string) (IDs, bool) {
	r := &Round{Round: round, Held: held, Places: blockPlaces(corpus), Units: make([]Unit, len(units))}
	at := slices.Index(units, unitID)
	if at < 0 || !slices.Contains(r.Places, roleID) {
		return IDs{}, false
	}
	return r.ids(r.idBase(), roleID, at), true
}

// spelled names the run as the prompt's JSON carries it, and as two empty
// strings when no id past the stored ones is left.
func (b IDs) spelled() (first, last string) {
	return b.spelledAs(finding.IDOf)
}

// spelledAs is spelled under another sequence's spelling, which is how §5.7's
// `x<n>` proposal ids reach a prompt through the same block type.
func (b IDs) spelledAs(spell func(int) string) (first, last string) {
	if !b.left() {
		return "", ""
	}
	return spell(b.First), spell(b.Last)
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
	return baseOf(r.heldSuffixes(), r.Round)
}

// heldID is one stored id reduced to what the block arithmetic reads: the round
// that wrote it, and its numeric suffix.
type heldID struct {
	round, n int
}

// heldSuffixes is Round.Held under §6.1's spelling.
func (r *Round) heldSuffixes() []heldID {
	suffixes := make([]heldID, 0, len(r.Held))
	for i := range r.Held {
		if n, ok := finding.IDSuffix(r.Held[i].ID); ok {
			suffixes = append(suffixes, heldID{round: r.Held[i].Round, n: n})
		}
	}
	return suffixes
}

// baseOf is the highest suffix held by an id an earlier round stored, which
// every block of this round starts past. This round's own are left out for the
// reason idBase gives.
func baseOf(held []heldID, round int) int {
	base := 0
	for _, id := range held {
		if id.round != round && id.n > base {
			base = id.n
		}
	}
	return base
}

// blockFor is the grid arithmetic both §6.1's record ids and §5.7's proposal
// ids use: the prompt's place gives it a hundred ids past base, and the block's
// first free id is past every stored id inside it.
//
// The two sequences share the grid and nothing else. They are spelled
// differently — `f<n>` and `x<n>` — so neither can hand out an id the other
// holds, and a proposal block never has to agree with a record block about
// anything but which cell of the grid a prompt sits in.
func blockFor(held []heldID, base, place int) IDs {
	block := IDs{First: base + place*idBlock + 1}
	block.Last = block.First + idBlock - 1
	for _, id := range held {
		if id.n >= block.First && id.n <= block.Last {
			block.First = id.n + 1
		}
	}
	return block
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
	return blockFor(r.heldSuffixes(), base, slices.Index(r.Places, roleID)*len(r.Units)+at)
}

// proposalIDs is the block of §5.7 proposal ids this round's prompt for roleID
// over the unit at index at is given, over the proposals the pull request
// stores.
func (r *Round) proposalIDs(roleID string, at int) IDs {
	suffixes := proposalSuffixes(r.HeldProposals)
	return blockFor(suffixes, baseOf(suffixes, r.Round),
		slices.Index(r.Places, roleID)*len(r.Units)+at)
}

// proposalSuffixes is a stored proposal set under §5.7's spelling.
func proposalSuffixes(held []proposal.Proposal) []heldID {
	suffixes := make([]heldID, 0, len(held))
	for i := range held {
		if n, ok := proposal.IDSuffix(held[i].ID); ok {
			suffixes = append(suffixes, heldID{round: held[i].Round, n: n})
		}
	}
	return suffixes
}

// ProposalIDs is the block of §5.7 proposal ids `cr review` gives the prompt for
// roleID over unitID in round, and false when no prompt of the round was given
// one: held is every proposal the pull request stores, corpus is §2.5.5's
// resolved corpus, and units are the round's unit ids in id order.
//
// It is the grid Round.ids lays record ids out on, over proposals.ndjson's own
// sequence, which is what §4.6.2's "a block disjoint from every other prompt's
// of the round" comes to for proposals.
func ProposalIDs(
	held []proposal.Proposal, round int, corpus []role.Resolved, units []string, roleID, unitID string,
) (IDs, bool) {
	places := blockPlaces(corpus)
	at := slices.Index(units, unitID)
	row := slices.Index(places, roleID)
	if at < 0 || row < 0 {
		return IDs{}, false
	}
	suffixes := proposalSuffixes(held)
	return blockFor(suffixes, baseOf(suffixes, round), row*len(units)+at), true
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
