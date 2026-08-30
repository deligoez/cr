package state

import "fmt"

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
// on, and refuses one it has not.
//
// It is the one door every command that reads §2.3's per-round files comes
// through, so the refusal is a property of the state layout rather than a check
// each command remembers. A command that reads meta.json directly is reading a
// file that may hold round 0, and round 0 is what a state directory created by
// something other than a brief carries: Layout.EnsurePR writes the whole §2.3
// table with the identity alone, and every NDJSON file in it is empty. Such a
// directory answers every read and answers all of them with nothing, which is
// the failure this exists to make impossible to reach quietly.
//
// The read takes no lock, per §2.3.2.
func (l Layout) Briefed(owner, repo string, pr int) (Meta, error) {
	recorded, err := l.ReadMeta(owner, repo, pr)
	if err != nil {
		return Meta{}, &NotBriefedError{Owner: owner, Repo: repo, PR: pr, Err: err}
	}
	if recorded.Round < 1 {
		return Meta{}, &NotBriefedError{Owner: owner, Repo: repo, PR: pr, Round: recorded.Round}
	}
	return recorded, nil
}
