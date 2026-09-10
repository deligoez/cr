package review

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
)

// orderHunk is one hunk of src/Order.php covering merge-base lines 20–22 and
// head lines 20–24: an edit that grew the file by two lines.
func orderHunk() git.Hunk {
	return git.Hunk{Path: "src/Order.php", BaseStart: 20, BaseLines: 3, HeadStart: 20, HeadLines: 5, Side: git.Right}
}

// thread is one ingested thread on src/Order.php, opened by a human unless the
// case says otherwise.
func thread(id string, side git.Side, start, line int) gh.Thread {
	return gh.Thread{
		ID:         id,
		Anchor:     gh.Anchor{Path: "src/Order.php", Side: side, StartLine: start, Line: line},
		AuthorType: gh.AuthorHuman,
		Comment:    gh.Comment{Author: "ayse", Body: "why the retry?"},
	}
}

// A human thread inside the unit's hunks or within threads.proximity_lines of
// them is attached, and one a line further out is not (§3.5.3).
//
// Both edges of the window are asserted on both sides of the hunk, because a
// window off by one in either direction attaches a thread about the next
// function or drops the one about this one — and on the LEFT side against the
// merge-base range, where the hunk covers three lines rather than five, so a
// comparison made in the wrong coordinates lands on the wrong edge.
func TestAHumanThreadWithinTheWindowIsAttachedAndOneBeyondIsNot(t *testing.T) {
	for _, tc := range []struct {
		name     string
		thread   gh.Thread
		attached bool
	}{
		{"inside the head range", thread("t1", git.Right, 22, 22), true},
		{"ten lines below the head range", thread("t2", git.Right, 34, 34), true},
		{"eleven lines below it", thread("t3", git.Right, 35, 35), false},
		{"ten lines above it", thread("t4", git.Right, 10, 10), true},
		{"eleven lines above it", thread("t5", git.Right, 9, 9), false},
		{"a range reaching into the window", thread("t6", git.Right, 1, 12), true},
		{"a LEFT line ten below the merge-base range", thread("t7", git.Left, 32, 32), true},
		{"a LEFT line eleven below it", thread("t8", git.Left, 33, 33), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attached := Threads([]git.Hunk{orderHunk()}, []gh.Thread{tc.thread}, 10)

			assert.Equal(t, tc.attached, len(attached) == 1)
		})
	}
}

// A bot's thread and a thread beyond the window are both left unattached, and
// so are a thread on another file and one the head has moved past; a resolved
// human thread inside the window is attached all the same (§3.5.1, §3.5.3).
func TestABotThreadAndAThreadBeyondTheWindowAreLeftUnattached(t *testing.T) {
	bot := thread("bot", git.Right, 21, 21)
	bot.AuthorType = gh.AuthorBot
	far := thread("far", git.Right, 60, 60)
	elsewhere := thread("elsewhere", git.Right, 21, 21)
	elsewhere.Anchor.Path = "src/Money.php"
	outdated := thread("outdated", git.Right, 0, 0)
	outdated.Outdated, outdated.Anchor.OriginalLine = true, 21
	resolved := thread("resolved", git.Right, 21, 21)
	resolved.Resolved = true

	attached := Threads([]git.Hunk{orderHunk()},
		[]gh.Thread{bot, far, elsewhere, outdated, resolved}, 10)

	assert.Equal(t, []gh.Thread{resolved}, attached)
	assert.Equal(t, []gh.Thread{}, Threads([]git.Hunk{orderHunk()}, []gh.Thread{bot}, 10),
		"none attached is [] rather than nil, per §12")
}
