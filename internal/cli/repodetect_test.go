package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// checkoutWithRemotes makes the repository under review a fresh checkout
// declaring the given remotes, name then URL, and no other.
func checkoutWithRemotes(t *testing.T, nameThenURL ...string) {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	for i := 0; i+1 < len(nameThenURL); i += 2 {
		mustGit(t, dir, "remote", "add", nameThenURL[i], nameThenURL[i+1])
	}
	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
}

// repoOfWith runs repoOf over a command carrying §11.1's --repo flag, set to
// named when named is not empty.
func repoOfWith(t *testing.T, named string) (owner, repo string, err error) {
	t.Helper()
	cmd := &cobra.Command{Use: "probe"}
	cmd.Flags().String("repo", "", "")
	if named != "" {
		require.NoError(t, cmd.Flags().Set("repo", named))
	}
	return repoOf(cmd)
}

// §11.1: with no --repo, owner/repo come from the repository under review's
// GitHub remote, in either the ssh or the https spelling git accepts, and a
// named --repo wins over the remote.
func TestTheRepositoryIsDetectedFromItsGitHubRemoteAndRepoOverridesIt(t *testing.T) {
	for name, remote := range map[string]string{
		"ssh, scp form":  "git@github.com:acme/api.git",
		"ssh URL":        "ssh://git@github.com/acme/api.git",
		"https":          "https://github.com/acme/api.git",
		"https, no .git": "https://github.com/acme/api",
	} {
		t.Run(name, func(t *testing.T) {
			checkoutWithRemotes(t, "origin", remote)

			owner, repo, err := repoOfWith(t, "")
			require.NoError(t, err)
			assert.Equal(t, [2]string{"acme", "api"}, [2]string{owner, repo})

			owner, repo, err = repoOfWith(t, "octocat/hello")
			require.NoError(t, err)
			assert.Equal(t, [2]string{"octocat", "hello"}, [2]string{owner, repo},
				"--repo is the override, and it wins over the remote")
		})
	}
}

// Each case detection cannot answer is refused with exit 2 and a step naming
// --repo, rather than guessed: no remote, several remotes, and a remote on
// another host.
func TestRepositoryDetectionRefusesWhatItCannotName(t *testing.T) {
	for name, tc := range map[string]struct {
		remotes []string
		says    string
	}{
		"no remote": {nil, "declares no remote"},
		"several remotes": {
			[]string{"origin", "git@github.com:acme/api.git", "upstream", "git@github.com:base/api.git"},
			"declares 2 remotes (origin, upstream)",
		},
		"a remote that is not GitHub": {
			[]string{"origin", "git@gitlab.com:acme/api.git"}, "which is not a github.com owner/repo",
		},
	} {
		t.Run(name, func(t *testing.T) {
			checkoutWithRemotes(t, tc.remotes...)

			_, _, err := repoOfWith(t, "")

			var refused *RepositoryDetectionError
			require.ErrorAs(t, err, &refused)
			assert.Contains(t, err.Error(), tc.says)
			assert.Equal(t, ExitUsage, exitCodeFor(err))
			assert.Contains(t, hintFor(err), "--repo <owner/repo>")
		})
	}
}
