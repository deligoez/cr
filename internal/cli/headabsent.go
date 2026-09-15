package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/git"
)

// headNotFetched is the one predicate that classifies a command's git failure
// as the pull request's head or base not being present in the repository under
// review. `cr brief`, `cr status`, `cr review` and `cr post` return every
// failure through it, so the four give the same `git fetch` step with code 3.
//
// It asks after the failure rather than before the first read, so a run whose
// reads succeed is not changed by it. A *git.CommandError is classified by
// asking the clone for the head and the base GitHub reports for the pull
// request, the two §3.4.1's merge base is taken from: when the clone lacks one,
// head first, the failure is that commit's *git.MissingCommitError. A git
// failure while the clone holds both is a refusal of another kind and keeps its
// own error and hint, and so does one where the repository or the pull request
// cannot be read to ask. Whatever it returns then goes through
// withRemoteMismatch.
func headNotFetched(cmd *cobra.Command, owner, repo string, pr int, err error) error {
	var failed *git.CommandError
	if errors.As(err, &failed) {
		err = absentCommit(owner, repo, pr, err)
	}
	return withRemoteMismatch(cmd, owner, repo, pr, err)
}

// absentCommit is headNotFetched's question to the clone: the
// *git.MissingCommitError of the pull request's head or base when the
// repository under review does not hold it, and err otherwise.
func absentCommit(owner, repo string, pr int, err error) error {
	dir, dirErr := repoDir()
	if dirErr != nil {
		return err
	}
	opened, readErr := ghClient().PullRequest(owner, repo, pr)
	if readErr != nil {
		return err
	}
	for _, commit := range []string{opened.Head, opened.Base} {
		if commit == "" {
			continue
		}
		var missing *git.MissingCommitError
		if asked := git.RequireCommit(dir, commit); errors.As(asked, &missing) {
			return missing
		}
	}
	return err
}
