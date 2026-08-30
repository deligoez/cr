package unit

import (
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/text"
)

// Range is one hunk's line range as §3.4.6 records it, both ends inclusive.
//
// The numbers are head-side, whatever side the unit's changed lines are
// numbered on, because §6.2.1 evaluates containment "entirely in head
// coordinates" against "the head-side range of one of its hunks — for a hunk
// that adds no lines, its head-side insertion point". That is exactly what
// git.Hunk.HeadRange answers, so the record carries the coordinates the one
// question asked of it is answered in rather than a second pair a caller would
// have to convert.
type Range struct {
	// Start is the range's first head line.
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
	// HunkRanges are the head-side ranges of the unit's hunks, in
	// ascending order.
	HunkRanges []Range `json:"hunk_ranges"`
	// ChangedLines is how many changed lines the unit holds.
	ChangedLines int `json:"changed_lines"`
	// Hash is §3.4.6's unit hash.
	Hash string `json:"hash"`
	// Formation is the branch of §3.4.4 that formed the unit.
	Formation Formation `json:"formation"`
	// Oversized marks §3.4.5's one exception, per Cluster.Oversized.
	Oversized bool `json:"oversized"`
}

// Units records §3.4.6's fields for the units §3.4.5 left, numbering them `u1`
// upwards in the order they arrive.
//
// The error is §1.4 step 1's, reached when a changed line is not valid UTF-8.
// It is returned rather than swallowed for the reason text.NormalisedHash
// gives: undecodable input fails with exit code 1, and hashing the bytes anyway
// would turn that refusal into a value indistinguishable from a real one.
func Units(clusters []Cluster) ([]Unit, error) {
	units := make([]Unit, 0, len(clusters))
	for i := range clusters {
		formed, err := clusters[i].record(len(units) + 1)
		if err != nil {
			return nil, err
		}
		units = append(units, formed)
	}
	return units, nil
}

// record writes one cluster down as §3.4.6's unit, numbered n.
func (c *Cluster) record(n int) (Unit, error) {
	hash, err := text.NormalisedHash(c.changedText())
	if err != nil {
		return Unit{}, err
	}
	ranges := make([]Range, len(c.Hunks))
	for i := range c.Hunks {
		start, end := c.Hunks[i].HeadRange()
		ranges[i] = Range{Start: start, End: end}
	}
	return Unit{
		ID:           "u" + strconv.Itoa(n),
		Path:         c.Path,
		Side:         c.Side,
		HunkRanges:   ranges,
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
