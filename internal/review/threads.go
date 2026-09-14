package review

import (
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
)

// Threads is §3.5.3 for one unit: the ingested human threads whose anchor
// falls inside the unit's hunks or within proximity lines of them, in the order
// they were ingested.
//
// It locates and stops. §3.5.3 forbids cr to assign a class to a thread or to
// decide suppression, and §3.5.4 gives the agent the question of whether an
// attached thread already covers a finding, so what a unit is handed here is
// every nearby thread a human wrote, resolved ones included — §3.5.1 records
// the resolution state and never drops a thread on it.
//
// A thread a bot opened is not attached: §3.5.2 tags the author so that §3.5.3
// can attach the human ones, and a bot's comment is not a colleague's concern
// that a finding might duplicate.
//
// # Coordinates
//
// An anchor is numbered on its own side, so it is compared against the hunk's
// range on that side: a RIGHT anchor against the head lines the hunk covers, a
// LEFT one against its merge-base lines. A hunk whose range on a side is empty
// is the insertion point git writes as the header's start, which is where a
// comment on that side can sit. An anchor whose line is zero — GitHub's answer
// for a thread the head has moved past — names no current line, so it falls
// inside nothing rather than being matched on the line it once named.
func Threads(hunks []git.Hunk, threads []gh.Thread, proximity int) []gh.Thread {
	attached := make([]gh.Thread, 0)
	for i := range threads {
		thread := &threads[i]
		if thread.AuthorType != gh.AuthorHuman || thread.Anchor.Line == 0 {
			continue
		}
		for h := range hunks {
			if near(&hunks[h], &thread.Anchor, proximity) {
				attached = append(attached, *thread)
				break
			}
		}
	}
	return attached
}

// unplacedOn is the ingested human threads on one file that name no current
// line, in the order they were ingested: the ones GitHub reports outdated, and
// the file-level ones written on the file as a whole.
//
// These are exactly the human threads on the file that Threads leaves out for
// their zero line. An outdated one is the one a push most needs to carry
// forward, because the code it was written on is the code the author has since
// changed; a file-level one never had a line to fall near. Both are returned
// apart rather than attached, because §3.5.3 attaches by where an anchor falls
// and neither anchor falls anywhere at the current head. An outdated thread
// GitHub still gives a current line is attached by Threads and not listed here.
func unplacedOn(path string, threads []gh.Thread) []gh.Thread {
	listed := make([]gh.Thread, 0)
	for i := range threads {
		thread := &threads[i]
		if thread.AuthorType == gh.AuthorHuman && thread.Anchor.Line == 0 && thread.Anchor.Path == path {
			listed = append(listed, *thread)
		}
	}
	return listed
}

// near reports whether an anchor's range lies within proximity lines of the
// hunk's range on the anchor's side.
func near(hunk *git.Hunk, anchor *gh.Anchor, proximity int) bool {
	if hunk.Path != anchor.Path {
		return false
	}
	// gh.Anchor carries StartLine equal to Line for a one-line thread, so
	// the range is always the pair as ingested.
	low, high := sideRange(hunk, anchor.Side)
	return anchor.StartLine <= high+proximity && anchor.Line >= low-proximity
}

// sideRange is the hunk's range on one side, both ends inclusive, and its
// insertion point when that side covers no line.
func sideRange(hunk *git.Hunk, side git.Side) (low, high int) {
	start, count := hunk.HeadStart, hunk.HeadLines
	if side == git.Left {
		start, count = hunk.BaseStart, hunk.BaseLines
	}
	if count == 0 {
		return start, start
	}
	return start, start + count - 1
}
