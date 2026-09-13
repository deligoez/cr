package state

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// numbered is a record shaped like the one §3.6.1 allocates: its number is a
// count over the records already in the file, which is the read-modify-write
// the context lock has to make exclusive.
type numbered struct {
	N int `json:"n"`
}

// contextRoot returns a layout with the global tree already created.
func contextRoot(t *testing.T) Layout {
	t.Helper()
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	return l
}

// A key nobody has recorded against holds no notes, and asking for them is not
// a failure: §3.6's file is created by the first note appended to it, so every
// first `cr note` reads a store that is not there.
func TestAnUnwrittenContextStoreHoldsNoRecords(t *testing.T) {
	l := contextRoot(t)

	held, err := l.LockContext("CR-1")
	require.NoError(t, err)
	defer func() { assert.NoError(t, held.Unlock()) }()

	records, err := ContextRecords[numbered](held)
	require.NoError(t, err)
	assert.Empty(t, records)
	assert.NotNil(t, records, "an absent store is no records, and §12.3 spells that []")
	assert.NoFileExists(t, l.ContextFile("CR-1"), "reading must not create the store")
}

// §2.3.2 makes reads lock-free, and this is the file where that has teeth:
// §3.6.5's `cr context` prints a store §3.6.1's `cr note` may be appending to at
// that moment. The read is asserted to complete while another holder has the
// exclusive lock, which is the whole of the claim — a reader that took the lock
// would sit here until the writer let go, and no assertion about the records it
// eventually returned would notice.
//
// What makes the lock-free read safe rather than merely quick is the rename:
// WriteContextRecords publishes the file whole, so the reader sees one version
// of it and never half of two.
func TestTheContextStoreIsReadWhileAWriterHoldsTheLock(t *testing.T) {
	l := contextRoot(t)

	held, err := l.LockContext("CR-1")
	require.NoError(t, err)
	require.NoError(t, WriteContextRecords(held, []numbered{{N: 1}, {N: 2}}))

	read := make(chan []numbered, 1)
	go func() {
		records, err := ReadContextRecords[numbered](l, "CR-1")
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
}

// A read of the context store writes nothing, which is the other half of why it
// does not take the lock: LockContext creates the directories it will write in,
// so a §3.6.5 read that locked would put a lock file and its directory inside
// ~/.cr in order to answer a question about a store that may not exist.
//
// A key nobody has recorded against is no records rather than a failure, for
// the reason the locked read gives: §3.6's file is created by the first note
// appended to it, so a key with no notes yet and one nobody will ever record
// against are the same absent file, and cr does not pretend to tell them apart.
func TestReadingTheContextStoreCreatesNothing(t *testing.T) {
	l := contextRoot(t)

	records, err := ReadContextRecords[numbered](l, "CR-1")
	require.NoError(t, err)
	assert.Empty(t, records)
	assert.NotNil(t, records, "an absent store is no records, and §12.3 spells that []")

	assert.NoFileExists(t, l.ContextFile("CR-1"), "the read created the store")
	assert.NoDirExists(t, filepath.Dir(l.ContextLockFile("CR-1")), "the read created the lock directory")
	entries, err := os.ReadDir(l.ContextDir())
	require.NoError(t, err)
	assert.Empty(t, entries, "the read left something in §2.2's context directory")
}

// The lock-free read builds the same path the locked write does, so it is held
// to the same rule: §2.2 stores the notes at context/<ISSUE-KEY>.ndjson and the
// key arrives as a positional argument, and a key that is not one name would
// read a file outside §2.2's tree entirely.
//
// The bait is a real file with real records in it, placed exactly where
// `../../escaped` resolves to, so a refusal that only looked like one — an
// empty result from a path that happens to hold nothing — cannot pass.
func TestReadingRefusesAnIssueKeyThatIsNotOneName(t *testing.T) {
	l := contextRoot(t)
	bait := filepath.Join(filepath.Dir(l.Root()), "escaped.ndjson")
	require.NoError(t, os.WriteFile(bait, []byte("{\"n\":9}\n"), 0o600))

	for _, key := range []string{"", ".", "..", "a/b", `a\b`, "../../escaped"} {
		t.Run("refuses "+key, func(t *testing.T) {
			records, err := ReadContextRecords[numbered](l, key)
			require.Error(t, err)
			assert.Nil(t, records, "a refused key returned records, so something outside §2.2 was read")
		})
	}
}

// The context store is keyed by issue and not by pull request, so §2.3.1's lock
// cannot serialise it: two runs against one key from two pull requests would
// take two different PR locks. §3.6.1's id is a counter over the records
// already in the file, so the read, the allocation, and the write are one
// critical section — without mutual exclusion two writers read the same file
// and allocate the same number, and one of the notes is lost.
func TestConcurrentContextWritersSerialise(t *testing.T) {
	l := contextRoot(t)

	const writers, each = 8, 25
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			for range each {
				if !appendNumbered(t, l) {
					return
				}
			}
		})
	}
	wg.Wait()

	held, err := l.LockContext("CR-1")
	require.NoError(t, err)
	defer func() { assert.NoError(t, held.Unlock()) }()

	records, err := ContextRecords[numbered](held)
	require.NoError(t, err)
	require.Len(t, records, writers*each, "records were lost, so the writes were not serialised")

	allocated := make([]int, 0, len(records))
	for _, record := range records {
		allocated = append(allocated, record.N)
	}
	slices.Sort(allocated)
	want := make([]int, 0, len(records))
	for n := range writers * each {
		want = append(want, n+1)
	}
	assert.Equal(t, want, allocated, "a number was allocated twice, so the read and the write were not one section")
}

// appendNumbered performs one locked read-allocate-write against the store, and
// reports whether it succeeded. It releases the lock on every path out, so a
// failed assertion fails the test rather than stranding the other writers.
func appendNumbered(t *testing.T, l Layout) bool {
	t.Helper()
	held, err := l.LockContext("CR-1")
	if !assert.NoError(t, err) {
		return false
	}
	defer func() { assert.NoError(t, held.Unlock()) }()

	records, err := ContextRecords[numbered](held)
	if !assert.NoError(t, err) {
		return false
	}
	return assert.NoError(t, WriteContextRecords(held, append(records, numbered{N: len(records) + 1})))
}

// §2.2 stores the notes at context/<ISSUE-KEY>.ndjson, and the key arrives as a
// positional argument. Invariant 2 keeps every write cr makes inside ~/.cr, so
// a key that is not one name is refused where the path is built rather than
// wherever a command happens to pass one along.
func TestAnIssueKeyCannotReachOutOfTheContextDirectory(t *testing.T) {
	l := contextRoot(t)
	outside := filepath.Join(filepath.Dir(l.Root()), "escaped.ndjson")

	for _, key := range []string{"", ".", "..", "a/b", "../escaped", `a\b`, "../../escaped"} {
		t.Run("refuses "+key, func(t *testing.T) {
			held, err := l.LockContext(key)
			require.Error(t, err)
			assert.Nil(t, held)
		})
	}

	assert.NoFileExists(t, outside)
	entries, err := os.ReadDir(l.ContextDir())
	require.NoError(t, err)
	assert.Empty(t, entries, "a refused key must leave nothing behind")

	held, err := l.LockContext("CR-1")
	require.NoError(t, err)
	require.NoError(t, WriteContextRecords(held, []numbered{{N: 1}}))
	require.NoError(t, held.Unlock())
	assert.FileExists(t, filepath.Join(l.ContextDir(), "CR-1.ndjson"))
}
