package note

import (
	"fmt"
	"time"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// InvalidRecordIDError reports a record id that is not spelled the way §6.1
// spells one.
//
// Like an unlisted source it is the invocation being wrong rather than data in
// a file, which §11.2 already codes 2 for an error internal/cli does not map,
// so it needs no mapping of its own.
type InvalidRecordIDError struct {
	// Value is what the argument carried.
	Value string
}

func (e *InvalidRecordIDError) Error() string {
	return fmt.Sprintf("record id %q: §6.1 spells a record id f<n>, e.g. f3", e.Value)
}

// NoStateError reports a pull request cr holds no state for.
//
// §2.2's state directory is opened by `cr brief`, and the issue key an answer
// is filed under is read out of it, so a pull request cr has never been briefed
// on has no key to file against. It is a file cr expected and did not find.
type NoStateError struct {
	// Owner, Repo, and PR name the pull request that has no state.
	Owner string
	Repo  string
	PR    int
	// Err is the read that failed.
	Err error
}

func (e *NoStateError) Error() string {
	return fmt.Sprintf(
		"cr holds no state for %s/%s#%d, so it knows no issue key to file the answer under: run `cr brief %d` first (%v)",
		e.Owner, e.Repo, e.PR, e.PR, e.Err,
	)
}

func (e *NoStateError) Unwrap() error { return e.Err }

// NoIssueKeyError reports a pull request that resolved to no issue key.
//
// §3.2 leaves the key empty when none of its four sources yields one and has
// the run continue, so this is recorded state rather than a broken file or a
// mistyped command line. What it means here is that §3.6.2 has nowhere to keep
// the answer: §2.2 stores the context store at context/<ISSUE-KEY>.ndjson and
// §3.6.4 loads it by that key, so a note filed under no key is a note no round
// will ever read. cr refuses rather than inventing a key or dropping the fact,
// which §11.2 codes 1.
type NoIssueKeyError struct {
	// Owner, Repo, and PR name the pull request that resolved to no key.
	Owner string
	Repo  string
	PR    int
}

func (e *NoIssueKeyError) Error() string {
	return fmt.Sprintf(
		"%s/%s#%d resolved to no issue key, so §3.6.2 has no store to record the answer against: "+
			"re-run `cr brief %d --issue <KEY>` to name one",
		e.Owner, e.Repo, e.PR, e.PR,
	)
}

// Answer records the answer to a posted question as a note against the issue
// key the pull request resolved to (§3.6.2).
//
// The command is addressed by pull request because §11 scopes a record id to
// one, while the note it writes belongs to an issue key, so the key has to come
// from somewhere. It is read out of the pull request's §2.3 metadata rather
// than typed again: that is the key the round actually resolved under §3.2, and
// a key retyped at the command line can differ from it, which would file the
// answer in a store no later round loads and lose the very fact §3.6.2 exists
// to keep.
//
// Nothing here reads or writes the record itself. §3.6.2 forbids the answer to
// change the record's state, and the way that is kept is that this path never
// takes the exclusive per-PR lock that every write to §2.3's files goes
// through, and never opens findings at all: the record id is checked for the
// spelling §6.1 gives it and is not resolved.
//
// Not resolving it is deliberate. Reading the record file to prove the id names
// something would tie an answer to the current round's records, which §9.3.5
// scopes to that round while exempting this store from the scoping outright —
// so an answer arriving after `cr brief` opened the next round would be refused
// though the question was really asked and really answered. The risk taken
// instead is a note referencing a record nobody wrote. That is provenance
// pointing at nothing inside a record §3.6.6 already calls unverified hearsay,
// and it costs nothing with the author, while refusing a true fact is the loss
// §3.6.2 was written to prevent.
func Answer(
	l state.Layout, owner, repo string, pr int, recordID, text string, source Source, at time.Time,
) (Note, error) {
	if !finding.ValidID(recordID) {
		return Note{}, &InvalidRecordIDError{Value: recordID}
	}
	issueKey, err := issueKeyOf(l, owner, repo, pr)
	if err != nil {
		return Note{}, err
	}
	return appendNote(l, issueKey, &Note{Text: text, Source: source, PR: pr, Record: recordID}, at)
}

// issueKeyOf reads the issue key one pull request resolved to. The read takes
// no lock, per §2.3.2.
func issueKeyOf(l state.Layout, owner, repo string, pr int) (string, error) {
	recorded, err := l.ReadMeta(owner, repo, pr)
	if err != nil {
		return "", &NoStateError{Owner: owner, Repo: repo, PR: pr, Err: err}
	}
	if recorded.IssueKey == "" {
		return "", &NoIssueKeyError{Owner: owner, Repo: repo, PR: pr}
	}
	return recorded.IssueKey, nil
}
