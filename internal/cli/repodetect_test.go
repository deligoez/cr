package cli

import (
	"os"
	"path/filepath"
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

// A remote whose owner or repository, once the `.git` suffix is gone, is . or
// .. is refused as `--repo` refuses the same halves, through splitRepo, with
// §11.2's code 2 and the step naming --repo. Audit round 5 measured detection
// answering an owner of `.`, which files a pull request's state under another
// repository's directory: state/./api is state/api.
//
// Both commands are driven with a gh on PATH that leaves a mark when it runs,
// and afterwards neither the mark nor any entry under the state root exists.
func TestADetectedRepositoryThatIsNotTwoPathSegmentsIsRefused(t *testing.T) {
	for name, tc := range map[string]struct{ remote, slug string }{
		"an owner of dot over https":                  {"https://github.com/./api", "./api"},
		"an owner of dot-dot over an ssh URL":         {"ssh://git@github.com/../api.git", "../api"},
		"a repository of dot before the suffix":       {"git@github.com:acme/..git", "acme/."},
		"a repository of dot-dot before the suffix":   {"git@github.com:acme/...git", "acme/.."},
		"a repository holding a separator over https": {"https://github.com/acme/api/extra", "acme/api/extra"},
	} {
		for _, command := range []string{"brief", "status"} {
			t.Run(name+" through "+command, func(t *testing.T) {
				checkoutWithRemotes(t, "origin", tc.remote)
				root := crHome(t)
				shim := t.TempDir()
				mark := filepath.Join(t.TempDir(), "gh-ran")
				require.NoError(t, os.WriteFile(filepath.Join(shim, "gh"),
					[]byte("#!/bin/sh\n: > "+mark+"\nexit 1\n"), 0o700))
				t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))

				err := runCLI(t, command, "1")

				var refused *RepositoryDetectionError
				require.ErrorAs(t, err, &refused)
				assert.Equal(t, "cannot detect the repository under review: remote origin is "+
					tc.remote+": "+repoRefusal(tc.slug), err.Error())
				assert.Equal(t, ExitUsage, exitCodeFor(err))
				assert.Equal(t, "run cr inside a clone whose one remote is its GitHub repository, "+
					"or pass --repo <owner/repo>", hintFor(err))
				assert.Empty(t, entryNames(t, root), "nothing is created under the state root")
				assert.Equal(t, []string{".cr"}, entryNames(t, filepath.Dir(root)),
					"nothing is created beside the state root")
				assert.NoFileExists(t, mark, "gh is not run for a repository cr refused")
			})
		}
	}
}
