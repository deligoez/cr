package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §7.4.4's repository-wide waiver store is §2.2 state rather than per-PR state,
// so it gets a lock of its own — and everything that follows from that is
// asserted here in one place: where the lock file goes, that a write through it
// lands at §2.2's path, that the read takes no lock, and that a store nobody has
// written to yet is no waivers rather than a failure.
//
// The lock's own path is the part that is easy to get wrong twice. PRLockFile
// already spends locks/<owner>/<repo> as a *directory* of pull-request locks, so
// a lock named locks/<owner>/<repo>.lock could not be created beside it at all —
// and the two are taken together below, because a collision would surface as a
// waiver store nobody can lock rather than as anything a path assertion sees.
func TestTheRepositoryWaiverStoreHasALockOfItsOwn(t *testing.T) {
	l := contextRoot(t)
	const owner, repo, pr = "octocat", "hello", 7

	empty, err := ReadWaiverRecords[numbered](l, owner, repo)
	require.NoError(t, err)
	assert.Empty(t, empty)
	assert.NotNil(t, empty, "an absent store is no waivers, and §12.3 spells that []")
	assert.NoFileExists(t, l.WaiversFile(owner, repo), "reading must not create the store")
	assert.NoDirExists(t, filepath.Dir(l.RepoWaiverLockFile(owner, repo)),
		"reading must not create the lock directory")

	held, err := l.LockRepoWaivers(owner, repo)
	require.NoError(t, err)
	assert.FileExists(t, l.RepoWaiverLockFile(owner, repo))

	waivers, err := WaiverRecords[numbered](held)
	require.NoError(t, err)
	assert.Empty(t, waivers)
	require.NoError(t, WriteWaiverRecords(held, []numbered{{N: 1}, {N: 2}}))

	// The pull-request lock is taken while the waiver lock is held, so the
	// two paths are proved to coexist rather than merely to differ.
	prHeld, err := l.LockPR(owner, repo, pr)
	require.NoError(t, err)
	require.NoError(t, prHeld.Unlock())

	// §2.3.2's reason reaches this file too: §7.4.7 lists it without taking
	// the lock, so a read must not sit behind a triage that is writing.
	read := make(chan []numbered, 1)
	go func() {
		records, err := ReadWaiverRecords[numbered](l, owner, repo)
		assert.NoError(t, err)
		read <- records
	}()
	select {
	case records := <-read:
		assert.Equal(t, []numbered{{N: 1}, {N: 2}}, records)
	case <-time.After(10 * time.Second):
		t.Fatal("§2.3.2: the read blocked behind the writer's lock, so it is not lock-free")
	}

	require.NoError(t, held.Unlock())
	body, err := os.ReadFile(l.WaiversFile(owner, repo))
	require.NoError(t, err)
	assert.Equal(t, "{\"n\":1}\n{\"n\":2}\n", string(body),
		"§7.4.4 stores the repository's waivers as NDJSON at waivers/<owner>/<repo>.ndjson")
}
