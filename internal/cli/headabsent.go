package cli

import (
	"errors"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/git"
)

// The commits of a pull request a command's git reads can reach in the
// repository under review.
const (
	readsHead = "head"
	readsBase = "base"
)

// headReads names, for each command whose git failures go through
// headNotFetched, keyed as the command is typed, the commits its git reads
// reach: the head, and the merge base GitHub's base is taken into where the
// command reads one. headNotFetched asks the clone for those commits and no
// other, so a command that never reads the merge base is not told to fetch a
// base it does not need. TestEveryCommandIsClassifiedAsAHeadReaderOrExempt
// holds this table to the command tree.
//
// §6.1.2 reads a LEFT anchor at the merge base, which takes `cr draft`,
// `cr record` and `cr merge` there. The sandbox is checked out at the head and
// nothing else, so `cr sandbox create`, `cr test` and `cr probe run` reach the
// head alone. §5.7's targets are head-side without exception, so
// `cr proposals record` reaches the head alone too.
//
// `cr recheck` reaches the head alone, and §9.4.2 is why it reaches nothing
// else: the migration reads the current head's tree and the record's own
// stored anchor, and never the superseded commit — which a force-push removes
// from a fresh clone, where it cannot be fetched by SHA.
var headReads = map[string][]string{
	"brief":            {readsHead, readsBase},
	"status":           {readsHead, readsBase},
	"review":           {readsHead, readsBase},
	"post":             {readsHead, readsBase},
	"draft":            {readsHead, readsBase},
	"record":           {readsHead, readsBase},
	"merge":            {readsHead, readsBase},
	"rules check":      {readsHead, readsBase},
	"sandbox create":   {readsHead},
	"test":             {readsHead},
	"probe run":        {readsHead},
	"proposals record": {readsHead},
	"recheck":          {readsHead},
}

// headNotFetched is the one predicate that classifies a command's git failure
// as the pull request's head or base not being present in the repository under
// review. Every command that reads the clone's head or merge base returns its
// git failures through it, and headReads names the commits each one's reads
// reach, so each gives the same `git fetch` step with code 3.
//
// It asks after the failure rather than before the first read, so a run whose
// reads succeed is not changed by it. A *git.CommandError is classified by
// asking the clone for the commits headReads names for cmd, of the head and the
// base GitHub reports for the pull request: when the clone lacks one, head
// first, the failure is that commit's *git.MissingCommitError. A git failure
// while the clone holds each commit the command reads is a refusal of another
// kind and keeps its own error and hint, and so does one where the repository
// or the pull request cannot be read to ask, or one of a command headReads does
// not name. Whatever it returns then goes through withRemoteMismatch.
func headNotFetched(cmd *cobra.Command, owner, repo string, pr int, err error) error {
	if _, ok := errors.AsType[*git.CommandError](err); ok {
		reads := headReads[strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")]
		err = absentCommit(owner, repo, pr, reads, err)
	}
	return withRemoteMismatch(cmd, owner, repo, pr, err)
}

// absentCommit is headNotFetched's question to the clone: the
// *git.MissingCommitError of the pull request's head or base, of those reads
// names, when the repository under review does not hold it, and err otherwise.
func absentCommit(owner, repo string, pr int, reads []string, err error) error {
	if len(reads) == 0 {
		return err
	}
	dir, dirErr := repoDir()
	if dirErr != nil {
		return err
	}
	opened, readErr := ghClient().PullRequest(owner, repo, pr)
	if readErr != nil {
		return err
	}
	var commits []string
	if slices.Contains(reads, readsHead) {
		commits = append(commits, opened.Head)
	}
	if slices.Contains(reads, readsBase) {
		commits = append(commits, opened.Base)
	}
	for _, commit := range commits {
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
