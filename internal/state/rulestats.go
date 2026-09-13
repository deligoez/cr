package state

import (
	"fmt"
	"path/filepath"

	"github.com/gofrs/flock"
)

// ruleStatsLocksDir holds the advisory locks over the per-repository rule
// ledgers of §2.6.1.6, apart from the pull-request locks for the reason
// waiverLocksDir gives: PRLockFile already spends locks/<owner>/<repo> as a
// directory, so a lock file of that name could not sit beside it.
const ruleStatsLocksDir = "rule-stats"

// RepoRuleStatsLockFile is the advisory lock guarding one repository's
// rule-stats.ndjson (§2.2, §2.6.1.6).
func (l Layout) RepoRuleStatsLockFile(owner, repo string) string {
	return filepath.Join(l.LocksDir(), ruleStatsLocksDir, owner, repo+".lock")
}

// UpdateRuleStats is the one door through which one repository's
// rule-stats.ndjson is written: it takes the ledger's exclusive advisory lock,
// hands change the entries the file holds, publishes what change returns, and
// releases the lock.
//
// §2.3.1 does not reach this file, for the reason it does not reach the waiver
// store: the ledger is §2.2 state shared by every pull request of one
// repository, so two commands on two pull requests take two different §2.3.1
// locks and would each publish the whole file. The lock covers the read as
// well as the write because §2.6.1.6's entries are keyed and overwritten
// rather than blindly appended — deciding whether an entry is new means reading
// the ones already there — so the read, the decision and the write are one
// critical section, and there is no exported way to hold the lock and do
// anything else.
//
// updateRepoStore below is the mechanism, shared with §7.3's triage ledger,
// and holds what the two stores have in common.
func UpdateRuleStats[T any](l Layout, owner, repo string, change func(held []T) []T) error {
	if err := l.containRepo(owner, repo); err != nil {
		return err
	}
	return updateRepoStore(
		l.RepoRuleStatsLockFile(owner, repo), l.RepoRuleStats(owner, repo), change)
}

// updateRepoStore is the body every repository-scoped NDJSON store of §2.2 is
// written through: rule-stats.ndjson and triage.ndjson both keep keyed entries
// that a re-run overwrites, so both need the read, the decision and the write
// inside one critical section, and neither has an exported way to hold the
// lock and do anything else.
//
// Like LockRepoWaivers it creates the directories it will write in, so a writer
// never depends on EnsureRepo having run first. A store that is not there yet
// holds no entries, for the reason storeRecords gives.
//
// The write publishes by rename, so §2.3.2's lock-free readers see one whole
// version of the file rather than half of two.
func updateRepoStore[T any](lock, store string, change func(held []T) []T) error {
	if err := makeDirs([]string{filepath.Dir(lock), filepath.Dir(store)}); err != nil {
		return err
	}
	held := flock.New(lock)
	if err := held.Lock(); err != nil {
		return fmt.Errorf("cannot lock %s: %w", lock, err)
	}
	err := rewriteStore(store, change)
	// The lock is released on the way out of every branch, and the write's
	// own failure is what the caller is told about when there was one.
	if released := held.Unlock(); err == nil && released != nil {
		err = fmt.Errorf("cannot release %s: %w", lock, released)
	}
	return err
}

// rewriteStore reads one NDJSON store, passes its records through change, and
// publishes the result by rename.
func rewriteStore[T any](path string, change func([]T) []T) error {
	existing, err := storeRecords[T](path)
	if err != nil {
		return err
	}
	body, err := encodeRecords(path, change(existing))
	if err != nil {
		return err
	}
	return writeAtomic(path, body)
}

// ReadRuleStatsRecords decodes one repository's rule ledger without taking
// the lock, per §2.3.2's rule for reads. It is the read §6.2.5 stamps a
// citation's origin against, and taking the lock for it would make a read
// create directories inside ~/.cr.
func ReadRuleStatsRecords[T any](l Layout, owner, repo string) ([]T, error) {
	return storeRecords[T](l.RepoRuleStats(owner, repo))
}
