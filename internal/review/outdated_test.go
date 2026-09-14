package review

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
)

// The threads a file lists apart are the human ones on that file with no
// current line — the ones GitHub marks outdated and the file-level ones — and
// nothing else: not a bot's, not one on another file, not a current thread, and
// not a thread GitHub marks outdated yet still places on a line (which §3.5.3
// attaches by that line). Attachment by position over the same threads is
// unchanged (§3.5.3).
func TestAFileListsItsHumanThreadsWithNoCurrentLineAndNothingElse(t *testing.T) {
	outdated := thread("outdated", git.Right, 0, 0)
	outdated.Outdated, outdated.Anchor.OriginalStartLine, outdated.Anchor.OriginalLine = true, 21, 21
	bot := outdated
	bot.ID, bot.AuthorType = "bot", gh.AuthorBot
	elsewhere := outdated
	elsewhere.ID, elsewhere.Anchor.Path = "elsewhere", "src/Money.php"
	current := thread("current", git.Right, 21, 21)
	placed := thread("placed", git.Right, 22, 22)
	placed.Outdated = true
	fileLevel := thread("file-level", git.Right, 0, 0)
	fileLevelBot := fileLevel
	fileLevelBot.ID, fileLevelBot.AuthorType = "file-level-bot", gh.AuthorBot
	all := []gh.Thread{outdated, bot, elsewhere, current, placed, fileLevel, fileLevelBot}

	assert.Equal(t, []gh.Thread{outdated, fileLevel}, unplacedOn("src/Order.php", all))
	assert.Equal(t, []gh.Thread{elsewhere}, unplacedOn("src/Money.php", all))
	assert.Equal(t, []gh.Thread{}, unplacedOn("src/TaxRate.php", all),
		"none listed is [] rather than nil, per §12")
	assert.Equal(t, []gh.Thread{current, placed}, Threads([]git.Hunk{orderHunk()}, all, 10))
}

// A thread with no current line is marked by what it is: an outdated one by the
// lines it named when written, a file-level one by its file, and one that is
// both by its file.
func TestAThreadWithNoCurrentLineIsMarkedOutdatedOrFileLevel(t *testing.T) {
	outdated := thread("outdated", git.Right, 0, 0)
	outdated.Outdated, outdated.Anchor.OriginalStartLine, outdated.Anchor.OriginalLine = true, 20, 21
	fileLevel := thread("file-level", git.Right, 0, 0)
	both := fileLevel
	both.Outdated = true

	assert.Equal(t, "outdated, originally at src/Order.php:20-21 (RIGHT)", placement(&outdated))
	assert.Equal(t, "file-level, on src/Order.php as a whole", placement(&fileLevel))
	assert.Equal(t, "outdated and file-level, on src/Order.php as a whole", placement(&both))
}
