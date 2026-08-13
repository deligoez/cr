package state

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// increment performs one locked read-modify-write of the counter, and reports
// whether it succeeded. It releases the lock on every path out, so a failed
// assertion fails the test rather than stranding the other writers.
func increment(t *testing.T, l Layout) bool {
	t.Helper()
	held, err := l.LockPR("acme", "web", 42)
	if !assert.NoError(t, err) {
		return false
	}
	defer func() { assert.NoError(t, held.Unlock()) }()

	body, err := l.ReadPR("acme", "web", 42, "findings.ndjson")
	if !assert.NoError(t, err) {
		return false
	}
	count, err := strconv.Atoi(string(body))
	if !assert.NoError(t, err, "a writer read a value no writer ever wrote") {
		return false
	}
	return assert.NoError(t, held.Write("findings.ndjson", []byte(strconv.Itoa(count+1))))
}

// lockedPR returns a layout with one pull request's state already created.
func lockedPR(t *testing.T) Layout {
	t.Helper()
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	require.NoError(t, l.EnsurePR("acme", "web", 42))
	return l
}

// §2.3.1: every write takes the exclusive advisory lock. A read-modify-write is
// the shape that proves it — without mutual exclusion two writers read the same
// value and one of the increments is lost, so a final count below the expected
// one is a lock that did not exclude.
func TestConcurrentWritersSerialise(t *testing.T) {
	l := lockedPR(t)

	first, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, first.Write("findings.ndjson", []byte("0")))
	require.NoError(t, first.Unlock())

	const writers, each = 8, 25
	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				if !increment(t, l) {
					return
				}
			}
		}()
	}
	wg.Wait()

	body, err := l.ReadPR("acme", "web", 42, "findings.ndjson")
	require.NoError(t, err)
	assert.Equal(t, strconv.Itoa(writers*each), string(body),
		"increments were lost, so the writes were not serialised")
}

