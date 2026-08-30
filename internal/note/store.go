package note

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/deligoez/cr/internal/state"
)

// Append records one note against issueKey and returns the note it wrote
// (§3.6.1).
//
// The note answers no record. §3.6.2's answer is the one that does, and it
// reaches the same store through appendNote below, so the two commands cannot
// disagree about how a note is allocated, validated, or published.
func Append(l state.Layout, issueKey, text string, source Source, pr int, at time.Time) (Note, error) {
	return appendNote(l, issueKey, &Note{Text: text, Source: source, PR: pr}, at)
}

// Retract marks one note as retracted and returns it as it now stands
// (§3.6.6).
//
// It takes no issue key because §3.6.1's id already carries one, which is what
// §11's `cr note --remove <note-id>` row says: the id names its own store.
//
// **It marks; it does not delete.** Two things turn on that. §3.6.1 allocates
// an id by counting over the notes the store holds, so removing a line would
// let the next note take the retracted one's number, and §3.3.2's claims and
// §8.1.6's provenance both cite a note by id — a reused id points a claim at a
// fact nobody ever recorded under it. And §3.6.6 asks for a retracted note's
// dependents to be *reported* rather than silently retained, which needs the
// retraction itself to be readable: a note that was deleted and a note that was
// never written are the same file.
//
// The cost is paid in the store. A note retracted precisely because it turned
// out to be wrong keeps its text on disk and keeps printing from `cr context`,
// under its retraction. §3.6.6 makes a note revocable, not erasable, and cr
// keeps the hearsay so the decision stays auditable rather than becoming a gap
// nobody can account for.
//
// Retracting a note that is already retracted is the state the caller asked
// for, so it succeeds and keeps the first retraction's timestamp. A second run
// reports when the decision was actually made instead of overwriting it.
func Retract(l state.Layout, noteID string, at time.Time) (Note, error) {
	issueKey, ok := SplitID(noteID)
	if !ok {
		return Note{}, fmt.Errorf(
			"note id %q: §3.6.1 forms one as <ISSUE-KEY>#n<n>, e.g. CR-1#n2", noteID,
		)
	}
	lock, err := l.LockContext(issueKey)
	if err != nil {
		return Note{}, err
	}
	retracted, err := retractLocked(lock, issueKey, noteID, at)
	// Joined rather than branched, for the reason appendNote gives.
	return retracted, errors.Join(err, lock.Unlock())
}

// retractLocked is Retract's critical section.
//
// The read, the decision, and the write are one for the reason appendLocked's
// are: both rewrite the whole file, so a retraction computed outside the lock
// would be published over a note a concurrent `cr note` had just appended.
func retractLocked(k *state.ContextLock, issueKey, noteID string, at time.Time) (Note, error) {
	notes, err := state.ContextRecords[Note](k)
	if err != nil {
		return Note{}, err
	}
	at = at.UTC()
	found := indexOf(notes, noteID)
	switch StandingOf(notes, noteID) {
	case StandingDangling:
		return Note{}, &UnknownNoteError{ID: noteID, IssueKey: issueKey}
	case StandingRetracted:
		return notes[found], nil
	}
	// UTC above and not at the call site, for the reason appendLocked
	// stamps in UTC: a retraction is ordered against the notes around it,
	// and an offset would make that order depend on where cr was run.
	notes[found].RetractedAt = &at
	if err := state.WriteContextRecords(k, notes); err != nil {
		return Note{}, err
	}
	return notes[found], nil
}

// Load returns every note recorded against issueKey, in the order they were
// recorded. It is what §3.6.4 means by loading the notes for an issue key, and
// what §3.6.5 prints.
//
// The issue key is the whole of the lookup, and that is the requirement rather
// than an economy. §3.6.4 has these notes reach every subsequent round and every
// subsequent pull request that resolves to the same key, and §9.3.5 exempts this
// store from round scoping outright, so there is no round to filter by and no
// pull request either: a note's PR field is the provenance §3.6.1 requires of it
// and is never read as a scope. A parameter here to narrow by would be a way of
// not seeing a fact somebody recorded.
//
// The read takes no lock, per §2.3.2. A store nobody has recorded against is no
// notes rather than a failure, for the reason state.ContextRecords gives.
func Load(l state.Layout, issueKey string) ([]Note, error) {
	return state.ReadContextRecords[Note](l, issueKey)
}

// appendNote records one note against issueKey, taking everything but the id
// and the timestamp from draft. It reads draft and never writes to it: the id
// and the timestamp are set on a copy, so the caller's value is left alone.
//
// The whole of §3.6.1 is here rather than at the command: the id is a counter
// over the notes already stored, so allocating it, appending, and publishing
// the file are one critical section under the lock state.LockContext takes.
// A caller cannot read the store, allocate an id, and write it back itself
// without holding that lock, because ContextRecords is a method on it.
//
// at is the caller's clock reading rather than one taken here, so one note is
// stamped once and a test is not asked to guess when it ran.
func appendNote(l state.Layout, issueKey string, draft *Note, at time.Time) (Note, error) {
	// Re-checked rather than trusted: Source is a string type, so any
	// caller can spell one §3.6.3 does not admit, and §8.1.6 has to name
	// this value in a posted body.
	if _, err := ParseSource(string(draft.Source)); err != nil {
		return Note{}, err
	}
	if strings.TrimSpace(draft.Text) == "" {
		return Note{}, errors.New("the note is empty: pass the fact as text, in quotes")
	}
	if draft.PR < 1 {
		return Note{}, fmt.Errorf(
			"invalid pull request %d: §3.6.1 records the pull request a note came from, so name it, e.g. 42",
			draft.PR,
		)
	}
	lock, err := l.LockContext(issueKey)
	if err != nil {
		return Note{}, err
	}
	recorded, err := appendLocked(lock, issueKey, draft, at)
	// Joined rather than branched: the lock is released whether or not the
	// note was written, and neither failure is traded away for the other.
	return recorded, errors.Join(err, lock.Unlock())
}

// appendLocked is appendNote's critical section.
func appendLocked(k *state.ContextLock, issueKey string, draft *Note, at time.Time) (Note, error) {
	existing, err := state.ContextRecords[Note](k)
	if err != nil {
		return Note{}, err
	}
	recorded := *draft
	recorded.ID = NextID(issueKey, existing)
	// UTC here and not at the call site: a store read on another machine
	// compares timestamps, and an offset would make the order of two notes
	// depend on where each was recorded.
	recorded.RecordedAt = at.UTC()
	if err := state.WriteContextRecords(k, append(existing, recorded)); err != nil {
		return Note{}, err
	}
	return recorded, nil
}
