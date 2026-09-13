package state

import (
	"path/filepath"
)

// triageLocksDir holds the advisory locks over the per-repository triage
// ledgers of §7.3, apart from the pull-request locks for the reason
// ruleStatsLocksDir gives: PRLockFile already spends locks/<owner>/<repo> as a
// directory, so a lock file of that name could not sit beside it.
const triageLocksDir = "triage"

// RepoTriageLockFile is the advisory lock guarding one repository's
// triage.ndjson (§2.2, §7.3).
func (l Layout) RepoTriageLockFile(owner, repo string) string {
	return filepath.Join(l.LocksDir(), triageLocksDir, owner, repo+".lock")
}

// UpdateTriage is the one door through which one repository's triage.ndjson is
// written: it takes the ledger's exclusive advisory lock, hands change the
// events the file holds, publishes what change returns, and releases the lock.
//
// §2.3.1 does not reach this file, for the reason it does not reach
// rule-stats.ndjson: §7.3.4 computes its rate over one repository's events
// across every pull request, so the file is §2.2 state shared by all of them
// and two commands on two pull requests would take two different §2.3.1 locks
// and each publish the whole file.
//
// The lock covers the read as well as the write because §7.3.1's events are
// keyed and overwritten rather than blindly appended — deciding whether an
// event is new means reading the ones already there — so the read, the
// decision and the write are one critical section.
func UpdateTriage[T any](l Layout, owner, repo string, change func(held []T) []T) error {
	if err := l.containRepo(owner, repo); err != nil {
		return err
	}
	return updateRepoStore(
		l.RepoTriageLockFile(owner, repo), l.RepoTriage(owner, repo), change)
}

// ReadTriageEventRecords decodes one repository's triage ledger without taking
// the lock, per §2.3.2's rule for reads. It is what §7.3.2's counts and
// §7.3.4's rate are computed over, and taking the lock for it would make a read
// create directories inside ~/.cr.
func ReadTriageEventRecords[T any](l Layout, owner, repo string) ([]T, error) {
	return storeRecords[T](l.RepoTriage(owner, repo))
}
