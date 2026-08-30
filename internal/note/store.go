package note

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/deligoez/cr/internal/state"
)

// Append records one note against issueKey and returns the note it wrote.
//
// The whole of §3.6.1 is here rather than at the command: the id is a counter
// over the notes already stored, so allocating it, appending, and publishing
// the file are one critical section under the lock state.LockContext takes.
// A caller cannot read the store, allocate an id, and write it back itself
// without holding that lock, because ContextRecords is a method on it.
//
// at is the caller's clock reading rather than one taken here, so one note is
// stamped once and a test is not asked to guess when it ran.
func Append(l state.Layout, issueKey, text string, source Source, pr int, at time.Time) (Note, error) {
	// Re-checked rather than trusted: Source is a string type, so any
	// caller can spell one §3.6.3 does not admit, and §8.1.6 has to name
	// this value in a posted body.
	if _, err := ParseSource(string(source)); err != nil {
		return Note{}, err
	}
	if strings.TrimSpace(text) == "" {
		return Note{}, errors.New("the note is empty: pass the fact as the second argument, in quotes")
	}
	if pr < 1 {
		return Note{}, fmt.Errorf(
			"invalid pull request %d: §3.6.1 records the PR a note came from, so pass --pr, e.g. --pr 42", pr,
		)
	}
	lock, err := l.LockContext(issueKey)
	if err != nil {
		return Note{}, err
	}
	recorded, err := appendLocked(lock, issueKey, text, source, pr, at)
	// Joined rather than branched: the lock is released whether or not the
	// note was written, and neither failure is traded away for the other.
	return recorded, errors.Join(err, lock.Unlock())
}

// appendLocked is Append's critical section.
func appendLocked(
	k *state.ContextLock, issueKey, text string, source Source, pr int, at time.Time,
) (Note, error) {
	existing, err := state.ContextRecords[Note](k)
	if err != nil {
		return Note{}, err
	}
	recorded := Note{
		ID:     NextID(issueKey, existing),
		Text:   text,
		Source: source,
		PR:     pr,
		// UTC here and not at the call site: a store read on another
		// machine compares timestamps, and an offset would make the
		// order of two notes depend on where each was recorded.
		RecordedAt: at.UTC(),
	}
	if err := state.WriteContextRecords(k, append(existing, recorded)); err != nil {
		return Note{}, err
	}
	return recorded, nil
}
