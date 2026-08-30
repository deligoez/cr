package cli

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

// prPlaceholder is the token a PR-scoped command carries in its Use line.
// Record ids are scoped to a PR (spec/0.1.0.md §11), so a command naming one
// is meaningless without the pull request that gives the id its scope.
const prPlaceholder = "<pr>"

// parsePR reads a pull request number from a positional argument.
func parsePR(arg string) (int, error) {
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid pull request %q: pass the pull request number, e.g. 42", arg)
	}
	return n, nil
}

// prArgs validates the positional arguments of a PR-scoped command: exactly
// total of them, the first being the pull request number.
func prArgs(total int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != total {
			return fmt.Errorf("%s takes %d argument(s), got %d", cmd.Name(), total, len(args))
		}
		_, err := parsePR(args[0])
		return err
	}
}

// repoOf reads the owner and repository a PR-scoped command works against.
//
// §11.1 registers `--repo <owner/repo>` as the override for repository
// detection, and v0.1 has not built the detection yet, so today the flag is the
// only source and a command that has to locate §2.2's state directory requires
// it. When detection lands the flag goes back to being the override §11.1 calls
// it, and this is the one place that has to change.
func repoOf(cmd *cobra.Command) (owner, repo string, err error) {
	named, err := cmd.Flags().GetString("repo")
	if err != nil {
		return "", "", err
	}
	if named == "" {
		return "", "", errors.New(
			"--repo is required: cr does not detect the repository yet, so name it as owner/repo",
		)
	}
	return splitRepo(named)
}
