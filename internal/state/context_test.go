package state

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

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
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				if !appendNumbered(t, l) {
					return
				}
			}
		}()
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

