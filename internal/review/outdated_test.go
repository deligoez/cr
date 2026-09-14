package review

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
)

// The outdated threads a file lists are the human ones GitHub marks outdated
// with no current line, on that file, and nothing else: not a bot's, not one on
// another file, not a current thread, not a thread GitHub marks outdated yet
// still places on a line (which §3.5.3 attaches by that line), and not a
// thread with no line that GitHub does not call outdated. Attachment by
// position over the same threads is unchanged (§3.5.3).
func TestAFileListsItsOutdatedHumanThreadsAndNothingElse(t *testing.T) {
	outdated := thread("outdated", git.Right, 0, 0)
	outdated.Outdated, outdated.Anchor.OriginalStartLine, outdated.Anchor.OriginalLine = true, 21, 21
	bot := outdated
	bot.ID, bot.AuthorType = "bot", gh.AuthorBot
	elsewhere := outdated
	elsewhere.ID, elsewhere.Anchor.Path = "elsewhere", "src/Money.php"
	current := thread("current", git.Right, 21, 21)
	placed := thread("placed", git.Right, 22, 22)
	placed.Outdated = true
	lineless := thread("lineless", git.Right, 0, 0)
	all := []gh.Thread{outdated, bot, elsewhere, current, placed, lineless}

	assert.Equal(t, []gh.Thread{outdated}, outdatedOn("src/Order.php", all))
	assert.Equal(t, []gh.Thread{elsewhere}, outdatedOn("src/Money.php", all))
	assert.Equal(t, []gh.Thread{}, outdatedOn("src/TaxRate.php", all),
		"none listed is [] rather than nil, per §12")
	assert.Equal(t, []gh.Thread{current, placed}, Threads([]git.Hunk{orderHunk()}, all, 10))
}
