package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePR(t *testing.T) {
	// 1 is the lowest pull request GitHub issues, and it is the value that tells
	// `n < 1` from `n <= 1`. Testing only 42 passes under either bound.
	for arg, want := range map[string]int{"1": 1, "42": 42} {
		t.Run("accepts "+arg, func(t *testing.T) {
			n, err := parsePR(arg)
			require.NoError(t, err)
			assert.Equal(t, want, n)
		})
	}

	for _, arg := range []string{"", "0", "-1", "4.2", "#42", " 42", "forty-two"} {
		t.Run("rejects "+arg, func(t *testing.T) {
			_, err := parsePR(arg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "pass the pull request number")
		})
	}
}

func TestPRArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "answer <pr> <record-id> <text>"}

	assert.NoError(t, prArgs(1)(cmd, []string{"42"}))
	assert.NoError(t, prArgs(3)(cmd, []string{"42", "r-1", "text"}))

	assert.Error(t, prArgs(1)(cmd, nil), "a missing pull request is a usage error")
	assert.Error(t, prArgs(3)(cmd, []string{"42", "r-1"}), "a missing record id is a usage error")
	assert.Error(t, prArgs(3)(cmd, []string{"42", "r-1", "text", "extra"}))
	assert.Error(t, prArgs(2)(cmd, []string{"r-1", "42"}), "the pull request comes first")
}

// Record ids are scoped to a PR (spec/0.1.0.md §11), so a command naming one
// must take the pull request ahead of it and must reject an argument that is
// not a pull request number. The walk covers the whole tree, so a command
// registered later cannot quietly drop the scope.
func TestRecordIDCommandsTakeThePullRequest(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, sub := range cmd.Commands() {
			walk(sub)
		}

		if i := strings.Index(cmd.Use, "<record-id>"); i >= 0 {
			pr := strings.Index(cmd.Use, prPlaceholder)
			require.GreaterOrEqual(t, pr, 0,
				"%q names a record id without the PR that scopes it", cmd.Use)
			assert.Less(t, pr, i, "%q must take the PR before the record id", cmd.Use)
		}

		if !strings.Contains(cmd.Use, prPlaceholder) {
			return
		}
		require.NotNil(t, cmd.Args, "%q takes a PR but validates no arguments", cmd.Use)
		assert.Error(t, cmd.Args(cmd, []string{"not-a-pull-request"}),
			"%q accepts an argument that is not a pull request", cmd.Use)
	}

	walk(newRootCmd())
}

// standingIn builds a working directory that is a git repository naming slug
// as its origin, which is what a repository detection has to work from.
func standingIn(t *testing.T, slug string) string {
	t.Helper()
	dir := t.TempDir()
	for _, argv := range [][]string{
		{"init", "--quiet"},
		{"remote", "add", "origin", "https://github.com/" + slug + ".git"},
	} {
		out, err := exec.Command("git", append([]string{"-C", dir}, argv...)...).CombinedOutput()
		require.NoError(t, err, "git %v: %s", argv, out)
	}
	return dir
}

// §11.1 makes `--repo <owner/repo>` an override of repository detection, and an
// override is not a contribution: cr works against the repository the flag
// names, and what the working directory says is not consulted alongside it.
//
// The run therefore stands inside a repository naming a different remote, with
// a configuration layer waiting under each of the two slugs. Detection would
// name other/elsewhere from that remote (repodetect_test.go drives it), so the
// run is the adversarial one: a detection that supplemented rather than yielded
// would put elsewhere's layer into the same answer.
//
// The two layers therefore set different settings rather than the same one at
// different values. An override and a merge agree about `post.max_comments`
// whichever wins, and disagree about whether `render.lang` came back as the
// standing repository asked or as the default nobody overrode — so the second
// assertion is the one that tells them apart, in both worlds.
func TestRepoOverridesRepositoryDetection(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".cr")
	t.Setenv(state.HomeEnv, root)
	named := filepath.Join(root, "repos", "acme", "web")
	standing := filepath.Join(root, "repos", "other", "elsewhere")
	require.NoError(t, os.MkdirAll(named, 0o700))
	require.NoError(t, os.MkdirAll(standing, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"post": {"max_comments": 9}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(named, "config.json"),
		[]byte(`{"post": {"max_comments": 7}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(standing, "config.json"),
		[]byte(`{"render": {"lang": "en"}}`), 0o600))

	t.Chdir(standingIn(t, "other/elsewhere"))

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"config", "--repo", "acme/web"})
	require.NoError(t, cmd.Execute())

	var printed map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &printed))
	assert.Equal(t, float64(7), printed["post.max_comments"],
		"§11.1: --repo names the repository whose layer is in force")
	assert.Equal(t, "tr", printed["render.lang"],
		"§11.1: the repository cr was standing in contributed a layer of its own")
}

// GitHub names one repository by every spelling of its slug, so `--repo`
// reaches the same state whatever its case and with or without `.git`. The
// comparison is on splitRepo's answer rather than on a lookup, because a lookup
// on macOS folds case in the filesystem and would pass without the fold.
func TestEverySpellingOfASlugNamesOneRepository(t *testing.T) {
	for _, spelled := range []string{"acme/web", "Acme/Web", "ACME/WEB", "acme/web.git", "Acme/Web.git"} {
		owner, name, err := splitRepo(spelled)
		require.NoError(t, err, spelled)
		assert.Equal(t, []string{"acme", "web"}, []string{owner, name}, spelled)
	}
	_, _, err := splitRepo("acme/.git")
	assert.Error(t, err, "a name that is only the suffix names no repository")
}
