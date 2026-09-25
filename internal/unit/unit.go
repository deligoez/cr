package unit

import (
	"slices"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/text"
)

// Range is one hunk's line range, both ends inclusive.
//
// A unit records two lists of them. HunkRanges is §3.4.6's "its hunk ranges",
// numbered on the side §3.4.1 numbers the unit's changed lines on — head lines
// for a RIGHT unit, merge-base lines for a LEFT one — which is the numbering a
// reader's anchor on that side counts in. HeadRanges is what §6.2.1 evaluates
// containment against "entirely in head coordinates": "the head-side range of
// one of its hunks — for a hunk that adds no lines, its head-side insertion
// point". git.Hunk.SideRange and git.Hunk.HeadRange answer the two.
type Range struct {
	// Start is the range's first line.
	Start int `json:"start"`
	// End is its last, equal to Start for an insertion point.
	End int `json:"end"`
}

// Unit is §3.4.6's record of one unit of review.
//
// Path is singular, and that is a claim about §3.4.4 rather than a shortening
// of §3.4.6. §3.4.6 writes "its file paths" and §6.2.1 writes "`path` is one of
// that unit's file paths", but every branch of §3.4.4 says "same file" before
// it says anything else, so the set those two sentences range over always holds
// exactly one element. A []string here would advertise a shape no diff can
// produce, and every reader of it would carry a loop, an ordering rule and an
// empty case for a second path that never arrives. The plural survives where it
// is real: §3.4.6's "file by file in ascending path order" is the hash's rule
// for a set of one, and §6.2.1's membership test is an equality.
type Unit struct {
	// ID is the round-scoped `u<n>` id of §3.4.6.
	ID string `json:"id"`
	// Path is the one file every hunk of the unit belongs to.
	Path string `json:"path"`
	// Side is the side the unit's changed lines are numbered on.
	Side git.Side `json:"side"`
	// HunkRanges are the ranges of the unit's hunks, in ascending order,
	// numbered on Side: a LEFT unit's are the merge-base lines its hunks
	// remove, first to last.
	HunkRanges []Range `json:"hunk_ranges"`
	// HeadRanges are the same hunks' head-side ranges, index for index,
	// which Contains measures in. A LEFT unit's are insertion points.
	HeadRanges []Range `json:"head_ranges"`
	// ChangedLines is how many changed lines the unit holds.
	ChangedLines int `json:"changed_lines"`
	// Hash is §3.4.6's unit hash.
	Hash string `json:"hash"`
	// Formation is the branch of §3.4.4 that formed the unit.
	Formation Formation `json:"formation"`
	// Oversized marks §3.4.5's one exception, per Cluster.Oversized.
	Oversized bool `json:"oversized"`
	// TwinOf is the earlier unit of the round this one is §4.6.8's twin
	// of, per MarkTwins, and empty for a unit that repeats none.
	TwinOf string `json:"twin_of,omitempty"`
}

// Units records §3.4.6's fields for the units §3.4.5 left, numbering them `u1`
// upwards in §3.4.6's assignment order.
//
// The ids are round-scoped: they are computed here from the round's own
// clusters and read from nowhere, so §3.4.6's "MUST NOT be carried across
// rounds" is a thing this package cannot do rather than a thing it refrains
// from doing.
//
// The error is §1.4 step 1's, reached when a changed line is not valid UTF-8.
// It is returned rather than swallowed for the reason text.NormalisedHash
// gives: undecodable input fails with exit code 1, and hashing the bytes anyway
// would turn that refusal into a value indistinguishable from a real one.
func Units(clusters []Cluster) ([]Unit, error) {
	units := make([]Unit, 0, len(clusters))
	for _, cluster := range order(clusters) {
		formed, err := cluster.record(len(units) + 1)
		if err != nil {
			return nil, err
		}
		units = append(units, formed)
	}
	return units, nil
}

// order puts §3.4.5's clusters into the order §3.4.6 assigns ids in: ascending
// first file path, then side with `LEFT` before `RIGHT`, then ascending first
// changed line.
//
// The side sits between the two keys §3.4.6 names because this is the one
// comparison in §3.4 that ranges over both sides at once. Everywhere else
// §3.4.4's partition has already separated them, and a line number is read in
// the one coordinate space its partition fixes. Here two units of one file can
// meet whose first changed lines are the same number in different file
// versions — a `LEFT` number naming a merge-base line and a `RIGHT` one naming
// a head line — and comparing them leaves the two units tied. A tie is not a
// harmless arbitrary choice: §2.1.1 requires the same inputs to give the same
// result, and an order that has to break a tie by whatever the input order
// happened to be is not an order at all. `LEFT` before `RIGHT` is §8.3.3's
// convention, taken from there rather than invented so cr sorts by side in one
// direction wherever it sorts by side.
//
// It sorts pointers into a copy of the slice header rather than the clusters
// themselves, so the caller's slice comes back in the order it was given.
func order(clusters []Cluster) []*Cluster {
	ordered := make([]*Cluster, len(clusters))
	for i := range clusters {
		ordered[i] = &clusters[i]
	}
	slices.SortStableFunc(ordered, compareClusters)
	return ordered
}

// compareClusters is order's comparison. It is stable-sorted rather than
// sorted, so two clusters no key separates keep the order §3.4.4 formed them
// in, which is itself fixed by the diff. Nothing a diff produces reaches that
// case — two clusters of one file and one side always differ in their first
// changed line — but a total order that leans on "nothing reaches it" is one
// assertion away from not being total.
func compareClusters(a, b *Cluster) int {
	if by := strings.Compare(a.Path, b.Path); by != 0 {
		return by
	}
	if by := sideRank(a.Side) - sideRank(b.Side); by != 0 {
		return by
	}
	return firstChangedLine(a) - firstChangedLine(b)
}

// sideRank is §8.3.3's ordering of the two sides §9.2 allows: `LEFT` before
// `RIGHT`.
func sideRank(side git.Side) int {
	if side == git.Left {
		return 0
	}
	return 1
}

// firstChangedLine is the cluster's first changed line, numbered on its own
// side, and 0 for a cluster whose hunks changed no line — the fallback branch
// of §3.4.4, which has no line to be ordered by and is ordered by the stable
// sort instead.
func firstChangedLine(c *Cluster) int {
	for i := range c.Hunks {
		if first, _, ok := changedSpan(&c.Hunks[i]); ok {
			return first
		}
	}
	return 0
}

// record writes one cluster down as §3.4.6's unit, numbered n.
func (c *Cluster) record(n int) (Unit, error) {
	hash, err := text.NormalisedHash(c.changedText())
	if err != nil {
		return Unit{}, err
	}
	ranges, head := make([]Range, len(c.Hunks)), make([]Range, len(c.Hunks))
	for i := range c.Hunks {
		start, end := c.Hunks[i].SideRange()
		ranges[i] = Range{Start: start, End: end}
		start, end = c.Hunks[i].HeadRange()
		head[i] = Range{Start: start, End: end}
	}
	return Unit{
		ID:           "u" + strconv.Itoa(n),
		Path:         c.Path,
		Side:         c.Side,
		HunkRanges:   ranges,
		HeadRanges:   head,
		ChangedLines: changedLines(c.Hunks),
		Hash:         hash,
		Formation:    c.Formation,
		Oversized:    c.Oversized,
	}, nil
}

// changedText is the pre-image §3.4.6's unit hash is taken over: the unit's
// changed lines "taken file by file in ascending path order and, within a file,
// ascending line order, joined by LF".
//
// Both orderings are the order the hunks already hold rather than one imposed
// here. A unit has one file, so ascending path order is that file; §3.4.4
// partitions before it compares and every branch keeps the partition's order,
// so the hunks are ascending; and ParseHunks keeps each hunk's changed lines in
// ascending line order.
//
// The lines are joined once and hashed once, the shape finding.AnchorContentHash
// uses for the same reason: a pre-image assembled per hunk and hashed per hunk
// would agree with this on the day it was written and stop agreeing the moment
// either side of it was touched, while both halves still looked right.
func (c *Cluster) changedText() string {
	lines := make([]string, 0, changedLines(c.Hunks))
	for i := range c.Hunks {
		for _, line := range c.Hunks[i].Changed {
			lines = append(lines, line.Text)
		}
	}
	return strings.Join(lines, "\n")
}

// changedLines counts the changed lines of a group of hunks: what §3.4.5
// measures against the cap and §3.4.6 records on the unit.
func changedLines(hunks []git.Hunk) int {
	lines := 0
	for i := range hunks {
		lines += len(hunks[i].Changed)
	}
	return lines
}
