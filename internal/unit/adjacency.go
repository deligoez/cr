package unit

import "github.com/deligoez/cr/internal/git"

// Adjacency groups the hunks §3.4.3 sends to the adjacency branch. Two hunks
// join when the gap between the last changed line of one and the first changed
// line of the next is at most gapLines, which is `cluster.gap_lines`.
//
// hunks are one file's, on one side, in ascending line order: §3.4.4
// partitions before it compares, and this compares. It returns no error,
// because this is where §3.4.3 arrives when no symbol could be found and an
// error is the one thing that must not come out of that.
//
// A hunk with no changed line has nothing to measure a gap from and joins
// nothing on either side of it. git does not emit a hunk of pure context, so
// this is a shape the parser admits rather than one a diff produces; reading
// it as adjacent would put lines nobody changed into somebody's unit.
func Adjacency(hunks []git.Hunk, gapLines int) [][]git.Hunk {
	groups := make([][]git.Hunk, 0, len(hunks))
	var current []git.Hunk
	prevLast, prevOK := 0, false
	for i := range hunks {
		first, last, ok := changedSpan(&hunks[i])
		adjacent := ok && prevOK && first-prevLast <= gapLines
		if len(current) > 0 && !adjacent {
			groups, current = append(groups, current), nil
		}
		current = append(current, hunks[i])
		prevLast, prevOK = last, ok
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

// changedSpan returns h's first and last changed line, and false when it has
// none. ParseHunks keeps a hunk's changed lines in ascending order, so the
// span is its two ends.
func changedSpan(h *git.Hunk) (first, last int, ok bool) {
	if len(h.Changed) == 0 {
		return 0, 0, false
	}
	return h.Changed[0].Line, h.Changed[len(h.Changed)-1].Line, true
}
