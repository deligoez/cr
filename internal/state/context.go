package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

// contextLocksDir holds the advisory locks over the context stores.
//
// It is a directory of its own inside §2.2's locks/ because the two locks
// already there are named after a repository — PRLockFile nests owner and
// repository, and §5.6's probe lock is a repository path with a profile id —
// while an issue key is neither. Keeping issue keys in their own directory
// means a key can never be read as a repository name, or the reverse.
const contextLocksDir = "context"

// ContextLockFile is the advisory lock guarding one issue's context store.
func (l Layout) ContextLockFile(issueKey string) string {
	return filepath.Join(l.LocksDir(), contextLocksDir, issueKey+".lock")
}

// ContextLock is the exclusive advisory lock over one issue's context store
// (§3.6), and the only route to that file's records.
//
// The store is not per-PR state, so §2.3's lock is the wrong one. That lock is
// keyed by pull request, while §3.6.4 has this file read by every later round
// and every later pull request that resolves to the same issue key, and §9.3.5
// exempts it from round scoping outright. Two `cr note` runs against one key
// from two different pull requests would take two different PR locks, see the
// same records, and allocate the same §3.6.1 id.
//
// So the lock is keyed by the issue key, and it covers more than the write:
// §3.6.1's `<ISSUE-KEY>#n<n>` is a counter over the records already in the
// file, which makes the read, the allocation, and the write one critical
// section. That is why Read is a method on the held lock rather than on the
// layout — a caller cannot count the notes without holding the lock it will
// write under.
//
// The write still publishes by rename. §2.3.2's reason applies to this file
// even though §2.3 does not: `cr context` reads it without taking the lock, so
// a reader arriving mid-write must see the whole previous file rather than half
// of the new one.
type ContextLock struct {
	held *flock.Flock
	path string
}

// LockContext blocks until it holds the exclusive advisory lock over one
// issue's context store. Like LockPR it creates the directory it will write in,
// so a writer never depends on EnsureContext having run first.
func (l Layout) LockContext(issueKey string) (*ContextLock, error) {
	if err := checkIssueKey(issueKey); err != nil {
		return nil, err
	}
	lock := l.ContextLockFile(issueKey)
	if err := makeDirs([]string{filepath.Dir(lock), l.ContextDir()}); err != nil {
		return nil, err
	}
	held := flock.New(lock)
	if err := held.Lock(); err != nil {
		return nil, fmt.Errorf("cannot lock %s: %w", lock, err)
	}
	return &ContextLock{held: held, path: l.ContextFile(issueKey)}, nil
}

// Unlock releases the lock and leaves its file on disk, for the reason
// Lock.Unlock does: an advisory lock binds to an inode and not to a path.
func (k *ContextLock) Unlock() error {
	if err := k.held.Unlock(); err != nil {
		return fmt.Errorf("cannot release %s: %w", k.held.Path(), err)
	}
	return nil
}

// ContextRecords decodes the locked issue's context store.
//
// A store that is not there yet is no records rather than a failure: §3.6's
// file is created by the first note appended to it, and a key nobody has
// recorded against holds exactly as many notes as an empty file does.
func ContextRecords[T any](k *ContextLock) ([]T, error) {
	return contextRecords[T](k.path)
}

// ReadContextRecords decodes one issue's context store without taking the lock,
// per §2.3.2. It is the read `cr context` makes (§3.6.5).
//
// A lock-free read of this file is safe for the reason §2.3.2 gives for the
// per-PR ones: WriteContextRecords publishes by rename, so a reader arriving
// mid-append sees the whole of one version rather than half of two.
//
// Taking the lock to read would be worse than merely slower. LockContext
// creates the directories it will write in, so a read-only command would write
// inside ~/.cr in order to answer a question, and a reader would then block
// behind every writer for a file the rename already made safe to read.
//
// The key is checked here for the reason checkIssueKey gives: it arrives as a
// positional argument and becomes a path segment, and a read is where a key
// naming a path would read a file outside §2.2's tree.
func ReadContextRecords[T any](l Layout, issueKey string) ([]T, error) {
	if err := checkIssueKey(issueKey); err != nil {
		return nil, err
	}
	return contextRecords[T](l.ContextFile(issueKey))
}

// contextRecords decodes one context store, whether or not its caller holds the
// lock. Both readers share it so a note is decoded the same way by the command
// that appends one and the command that prints them.
func contextRecords[T any](path string) ([]T, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return make([]T, 0), nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	return decodeRecords[T](path, body)
}

// WriteContextRecords publishes the locked issue's context store, replacing it
// with records.
func WriteContextRecords[T any](k *ContextLock, records []T) error {
	body, err := encodeRecords(k.path, records)
	if err != nil {
		return err
	}
	return writeAtomic(k.path, body)
}

// checkIssueKey refuses an issue key that would not name one file inside the
// context directory.
//
// §2.2 puts the store at context/<ISSUE-KEY>.ndjson, so the key reaches the
// filesystem as a path segment, and it arrives as a positional argument.
// Invariant 2 keeps every write cr makes inside ~/.cr, and a key carrying a
// separator or a parent reference would leave it. The check sits here, where
// the path is built, rather than at each command that passes a key along.
func checkIssueKey(issueKey string) error {
	switch {
	case issueKey == "":
		return errors.New("the issue key is empty: name the tracker issue, e.g. CR-1")
	case issueKey == "." || issueKey == ".." || strings.ContainsAny(issueKey, `/\`):
		return fmt.Errorf(
			"issue key %q is not a single name: §2.2 stores it at context/<ISSUE-KEY>.ndjson, so it cannot hold a path",
			issueKey,
		)
	}
	return nil
}
