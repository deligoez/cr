package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// repoRuleLocksDir holds the advisory locks over the per-repository rule
// directories of §2.2, apart from the pull-request locks for the reason
// waiverLocksDir gives.
const repoRuleLocksDir = "repo-rules"

// RepoRulesLockFile is the advisory lock guarding one repository's rules
// directory (§2.2, §2.6.3.9).
func (l Layout) RepoRulesLockFile(owner, repo string) string {
	return filepath.Join(l.LocksDir(), repoRuleLocksDir, owner, repo+".lock")
}

// RuleExistsError reports a rule id the per-repository layer already holds, on
// a write that was not asked to replace it (§2.6.3.9).
type RuleExistsError struct {
	// Path is the rule file already there.
	Path string
}

func (e *RuleExistsError) Error() string {
	return "the per-repository layer already holds " + e.Path
}

// WriteRepoRule is the one door through which a rule file of the
// per-repository layer is written: it takes that directory's exclusive
// advisory lock, refuses a file already there unless replace is set, publishes
// body at `repos/<owner>/<repo>/rules/<id>.json`, and releases the lock. It
// reports whether a file was replaced.
//
// The existence check sits inside the lock, because two `cr rules add` runs of
// one id would otherwise both see no file and the second would replace the
// first without having been asked to. The caller has already validated id as a
// §2.6 rule id, which is kebab-case and so names one file of that directory.
func (l Layout) WriteRepoRule(owner, repo, id string, body []byte, replace bool) (replaced bool, err error) {
	if err := l.containRepo(owner, repo); err != nil {
		return false, err
	}
	lock := l.RepoRulesLockFile(owner, repo)
	path := l.RepoRule(owner, repo, id)
	if err := makeDirs([]string{filepath.Dir(lock), filepath.Dir(path)}); err != nil {
		return false, err
	}
	held := flock.New(lock)
	if err := held.Lock(); err != nil {
		return false, FileFailure("lock", lock, lockHint, err)
	}
	replaced, err = publishRule(path, body, replace)
	if released := held.Unlock(); err == nil && released != nil {
		err = FileFailure("release", lock, lockHint, released)
	}
	return replaced, err
}

// publishRule is WriteRepoRule's critical section.
func publishRule(path string, body []byte, replace bool) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil && !replace:
		return false, &RuleExistsError{Path: path}
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return false, FileFailure("read", path, writeHint, err)
	}
	return err == nil, writeAtomic(path, body)
}
