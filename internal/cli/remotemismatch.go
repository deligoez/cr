package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/git"
)

// RemoteMismatchError is a commit the repository under review does not hold,
// met while `--repo` names a repository that none of that repository's GitHub
// remotes points at.
//
// It changes the step and nothing else. The message stays the missing commit's
// and the code stays its 3, because nothing is refused for the mismatch alone:
// a clone of a fork that holds the pull request's commits is a legitimate
// repository under review for the upstream `--repo` names. What the mismatch
// changes is that a `git fetch` from the clone's own remotes is no longer known
// to bring the commit, so the hint names both repositories instead.
type RemoteMismatchError struct {
	// Err is the failure as the command met it, which carries the
	// *git.MissingCommitError.
	Err error
	// Named is the owner/repo `--repo` names.
	Named string
	// Remotes are the clone's GitHub remotes, none of which points at Named.
	Remotes []PointedRemote
	// PR is the pull request whose commit is missing.
	PR int
}

// PointedRemote is one remote of the repository under review and the GitHub
// owner/repo its fetch URL points at.
type PointedRemote struct {
	// Name is the remote's name.
	Name string
	// Slug is the owner/repo its URL names.
	Slug string
}

func (e *RemoteMismatchError) Error() string {
	return e.Err.Error()
}

// Unwrap hands the failure on, so every reader that asks for the missing commit
// still finds it.
func (e *RemoteMismatchError) Unwrap() error {
	return e.Err
}

// Hint is §12.4's next step with both repositories named: the one `--repo`
// names, and the one each GitHub remote of the repository under review points
// at.
func (e *RemoteMismatchError) Hint() string {
	pointed := make([]string, 0, len(e.Remotes))
	for _, remote := range e.Remotes {
		pointed = append(pointed, remote.Name+" points at "+remote.Slug)
	}
	return fmt.Sprintf("`--repo` names %s, and no remote of the repository under review points at it (%s), "+
		"so a `git fetch` from its remotes may not bring the commit the message names; check that `--repo` "+
		"names the pull request's repository; if it does, fetch the commit from it with "+
		"`git fetch https://github.com/%s pull/%d/head` and run the command again, or run the command in a clone of %s",
		e.Named, strings.Join(pointed, ", "), e.Named, e.PR, e.Named)
}

// withRemoteMismatch decorates err as a *RemoteMismatchError when it carries a
// commit the repository under review does not hold, cmd names the repository
// with `--repo`, and no GitHub remote of that repository points at owner/repo.
// Every other err is returned as it came.
//
// Without `--repo` the repository was detected from the clone's one remote, so
// the two cannot differ. The remotes are read through internal/git's runner and
// compared as owner/repo after githubSlug has taken the ssh, `ssh://`, https and
// `.git` spellings off, letter case folded as GitHub folds it. A remote on
// another host names no GitHub repository to compare, and a clone with no
// GitHub remote at all, or whose remotes git cannot list, keeps the fetch hint:
// a mismatch cr cannot establish is not named.
func withRemoteMismatch(cmd *cobra.Command, owner, repo string, pr int, err error) error {
	var missing *git.MissingCommitError
	if !errors.As(err, &missing) {
		return err
	}
	if already := (*RemoteMismatchError)(nil); errors.As(err, &already) {
		return err
	}
	if flagged, flagErr := cmd.Flags().GetString("repo"); flagErr != nil || flagged == "" {
		return err
	}
	remotes, listErr := git.Remotes(missing.Dir)
	if listErr != nil {
		return err
	}
	named := owner + "/" + repo
	pointed := make([]PointedRemote, 0, len(remotes))
	for _, remote := range remotes {
		slug, ok := githubSlug(remote.URL)
		if !ok {
			continue
		}
		if strings.EqualFold(slug, named) {
			return err
		}
		pointed = append(pointed, PointedRemote{Name: remote.Name, Slug: slug})
	}
	if len(pointed) == 0 {
		return err
	}
	return &RemoteMismatchError{Err: err, Named: named, Remotes: pointed, PR: pr}
}
