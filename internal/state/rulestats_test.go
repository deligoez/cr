package state

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.6.1.6's ledger is §2.2 state with a lock of its own, apart from the
// pull-request locks for the reason the waiver store's is, and its one write
// door hands change what the file holds and publishes what change returns.
//
// An absent ledger reads as no entries — through the lock-free read and inside
// the door alike — and reading it creates nothing, because a read-only command
// must not write inside ~/.cr to answer a question.
func TestTheRuleLedgerIsReadThroughOneLockedDoorAndWrittenByRename(t *testing.T) {
	l := contextRoot(t)
	const owner, repo = "octocat", "hello"

	empty, err := ReadRuleStatsRecords[numbered](l, owner, repo)
	require.NoError(t, err)
	assert.NotNil(t, empty, "an absent ledger is no entries, and §12.3 spells that []")
	assert.Empty(t, empty)
	assert.NoFileExists(t, l.RepoRuleStats(owner, repo), "reading must not create the ledger")
	assert.Equal(t, filepath.Join(l.LocksDir(), "rule-stats", owner, repo+".lock"),
		l.RepoRuleStatsLockFile(owner, repo))

	var handed [][]numbered
	for _, n := range []int{1, 2} {
		require.NoError(t, UpdateRuleStats(l, owner, repo, func(held []numbered) []numbered {
			handed = append(handed, held)
			return append(held, numbered{N: n})
		}))
	}

	assert.Equal(t, [][]numbered{{}, {{N: 1}}}, handed, "change is handed what the file held")
	assert.FileExists(t, l.RepoRuleStatsLockFile(owner, repo))
	written, err := ReadRuleStatsRecords[numbered](l, owner, repo)
	require.NoError(t, err)
	assert.Equal(t, []numbered{{N: 1}, {N: 2}}, written)

	prHeld, err := l.LockPR(owner, repo, 7)
	require.NoError(t, err, "the ledger's lock and a pull request's coexist")
	require.NoError(t, prHeld.Unlock())
}
