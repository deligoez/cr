package state

import (
	"errors"
	"fmt"
)

// NotBriefedError reports per-PR state no `cr brief` has opened a round in.
//
// §3.7 turns `cr brief`'s permission to persist the derived inputs of §3.3
// through §3.6 into the obligation the rest of cr rests on: §4.5.6 and §4.1.6
// reject a cell or a mapping naming an unknown unit id, §9.3.1 and §9.3.3 read
// meta.json's recorded head, and §3.5.3 attaches the ingested threads. Every one
// of those treats units.ndjson, threads.ndjson and meta.json as authoritative,
// so a command that found them absent has two ways forward and only one of them
// is honest: recompute the units itself, and answer §4.1.6 against a unit set no
// round ever recorded, or refuse and name the command that writes them.
//
// §11.2 codes it 4. It is a state conflict rather than a missing file: the
// command line is right, every file the caller named was read, and what refuses
// is where the pull request stands — no round has been opened on it, which no
// retyping and no edit to the input can change.
type NotBriefedError struct {
	// Owner, Repo, and PR name the pull request that has no round.
	Owner string
	Repo  string
	PR    int
	// Round is the round index meta.json carried, and 0 when the file was
	// not there at all. §9.3.3 numbers rounds from 1, so 0 is a state
	// directory no brief has opened rather than a round of its own.
	Round int
	// Err is the read that failed, and nil when meta.json read cleanly and
	// named no round.
	Err error
}

func (e *NotBriefedError) Error() string {
	return fmt.Sprintf(
		"%s/%s#%d has no round: §3.7 has `cr brief` write %s, %s and %s, "+
			"and §4.1.6, §4.5.6, §9.3.1 and §3.5.3 all read them as authoritative rather than recomputing them; "+
			"run `cr brief %d --repo %s/%s` first%s",
		e.Owner, e.Repo, e.PR, FileMeta, FileUnits, FileThreads,
		e.PR, e.Owner, e.Repo, e.because(),
	)
}

// because appends the read that failed, and nothing when meta.json was readable
// and simply named no round. The two are different situations to whoever is
// reading: one is a state directory that is not there, the other is one that is.
func (e *NotBriefedError) because() string {
	if e.Err == nil {
		return ""
	}
	return " (" + e.Err.Error() + ")"
}

// Unwrap exposes the read that failed, so a caller can tell a state directory
// that is absent from one that is present and holds no round.
func (e *NotBriefedError) Unwrap() error { return e.Err }

// Briefed returns the metadata of a pull request `cr brief` has opened a round
// on together with §9.3.1's comparison, and refuses one no brief has opened.
//
// It is the one door every command that reads §2.3's per-round files comes
// through, so both the refusal and the comparison are properties of the state
// layout rather than checks each command remembers. A command that reads
// meta.json directly is reading a file that may hold round 0, and round 0 is
// what a state directory created by something other than a brief carries:
// Layout.EnsurePR writes the whole §2.3 table with the identity alone, and
// every NDJSON file in it is empty. Such a directory answers every read and
// answers all of them with nothing, which is the failure this exists to make
// impossible to reach quietly.
//
// current is read only once a round has been established, so a pull request no
// brief has touched costs no network read at all. The order matters the other
// way too: a caller that cannot reach GitHub is told that, rather than being
// told the round is current.
//
// The read takes no lock, per §2.3.2.
func (l Layout) Briefed(owner, repo string, pr int, current CurrentHead) (Round, error) {
	recorded, err := l.ReadMeta(owner, repo, pr)
	var stem *ProfileIDError
	if errors.As(err, &stem) {
		// A round is recorded, and it names a profile no file of §2.2's
		// profiles directory can be: a brief would not repair the file.
		return Round{}, err
	}
	if err != nil {
		return Round{}, &NotBriefedError{Owner: owner, Repo: repo, PR: pr, Err: err}
	}
	if recorded.Round < 1 {
		return Round{}, &NotBriefedError{Owner: owner, Repo: repo, PR: pr, Round: recorded.Round}
	}
	if current == nil {
		return Round{}, &NoCurrentHeadError{Owner: owner, Repo: repo, PR: pr, Round: recorded.Round}
	}
	head, err := current()
	if err != nil {
		return Round{}, err
	}
	return Round{Meta: recorded, Current: head}, nil
}

// CurrentHead reads the head GitHub reports for the pull request under review.
//
// It is a function rather than a string for two reasons, and both are about
// when the read happens. §9.3.1 compares the current head against a round's
// recorded head, so there is nothing to compare until a round has been
// established — a pull request no `cr brief` has opened is refused above before
// anything reaches the network, and cr asks GitHub nothing about it. And the
// read is not cr's to take from the checkout: gh/pr.go argues the current head
// must be GitHub's `headRefOid`, because a local branch of the same name may
// sit anywhere and on a fork names a different history altogether, which would
// make a round's staleness a property of what the user happened to have checked
// out.
//
// It is a parameter rather than a field so that no command can read per-PR
// state without saying where the current head comes from. §9.3.1 binds every
// such command, and a compiler that demands the argument is the only version of
// that rule which a command written next month cannot forget.
type CurrentHead func() (string, error)

// Round is one round of §2.3's per-PR state, read together with §9.3.1's
// comparison.
//
// The comparison is part of what the door hands back rather than a check each
// command remembers to perform. §9.3.1 binds "every command that reads per-PR
// state", Briefed is where that reading happens, and a command therefore cannot
// obtain a round without also obtaining the answer to whether the head moved
// under it — which is the question §9.3.2 then makes every writer refuse on.
type Round struct {
	// Meta is meta.json as the round recorded it.
	Meta
	// Current is the head GitHub reports for the pull request now.
	Current string
}

// Stale is §9.3.1's comparison.
//
// The three methods here take a pointer receiver because a round carries the
// whole of meta.json and copying it per call is what gocritic's hugeParam
// objects to. finding.HonestyDisclosure is therefore satisfied by *Round, which
// is what a caller holding a round has anyway.
func (r *Round) Stale() bool { return r.Current != r.Head }

// Disclosure is the stale-round report §11.1 names among the seven `--quiet`
// may never suppress, in the shape finding.HonestyDisclosure fixes.
//
// It names both heads whether or not they differ. §9.3.1 asks for both only
// when they differ, and the wider sentence is what makes the report readable as
// an answer rather than as an alarm: a reader told nothing cannot tell a round
// that is current from a comparison that never ran, and §11.1's other six
// disclosures print their own zero case for the same reason.
func (r *Round) Disclosure() string {
	if !r.Stale() {
		return fmt.Sprintf(
			"§9.3.1: round %d was opened at head %s, which is still the pull request's current head",
			r.Round, r.Head,
		)
	}
	return fmt.Sprintf(
		"§9.3.1: round %d was opened at head %s and the pull request's current head is %s",
		r.Round, r.Head, r.Current,
	)
}

// RefuseStale is §9.3.2, and it is nil exactly when the heads agree.
//
// It is a method on the round rather than a check written into each command, so
// the refusal and the comparison cannot come apart: whatever obtains a round
// obtains this, and a writer that does not call it is visible as a writer that
// never mentions §9.3 at all.
func (r *Round) RefuseStale() error {
	if !r.Stale() {
		return nil
	}
	return &StaleRoundError{At: *r}
}

// StaleRoundError is §9.3.2's refusal: the head moved under a round, so every
// command that writes per-PR state stops and names both heads.
//
// §11.2 codes it 4. Nothing about the invocation is wrong — the command line is
// right and every file it named was read — and what refuses is where the pull
// request stands, which no retyping and no edit to an input can change. The
// only way forward is §9.3.3's: `cr brief` opens the round the head now belongs
// to, and the hint names it.
type StaleRoundError struct {
	// At is the round and the comparison that refused.
	At Round
}

func (e *StaleRoundError) Error() string {
	return fmt.Sprintf(
		"%s; §9.3.2 refuses every write to per-PR state until the round moves with it: "+
			"run `cr brief %d --repo %s/%s`",
		e.At.Disclosure(), e.At.PR, e.At.Owner, e.At.Repo,
	)
}

// NoCurrentHeadError reports a round read with no way to learn the current
// head.
//
// §9.3.1 makes the comparison unconditional, so a caller supplying no reader
// has not asked a cheaper question — it has asked one cr must not answer.
// Refusing here rather than reading the absence as "unchanged" is the
// difference between a command that cannot run and a command that quietly
// reports a round as current because nobody looked. §11.2 codes it 4 with the
// rest of §9.3.
type NoCurrentHeadError struct {
	// Owner, Repo, and PR name the pull request whose head went unread.
	Owner string
	Repo  string
	PR    int
	// Round is the round that was read.
	Round int
}

func (e *NoCurrentHeadError) Error() string {
	return fmt.Sprintf(
		"%s/%s#%d round %d was read with no source for the current head, "+
			"and §9.3.1 has cr compare the two on every command that reads per-PR state",
		e.Owner, e.Repo, e.PR, e.Round,
	)
}
