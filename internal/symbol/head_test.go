package symbol

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/profile"
)

// fixtureGit runs one git command inside the fixture repository.
func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// headOf builds a repository whose single commit holds the files given, keyed
// by repository-relative path. The identity is repository-local: a machine with
// no global user.email cannot commit at all, and CI is such a machine.
func headOf(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--quiet", "--initial-branch=main")
	fixtureGit(t, dir, "config", "user.email", "fixture@cr.test")
	fixtureGit(t, dir, "config", "user.name", "cr fixture")
	fixtureGit(t, dir, "config", "commit.gpgsign", "false")
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "--quiet", "-m", "the head under review")
	return dir
}

// phpProfile is the shape of the shipped laravel-pest profile that decides what
// the index reads: the language, and the source the profile owns.
func phpProfile(globs ...string) *profile.Profile {
	return &profile.Profile{
		ID:      "laravel-pest",
		Match:   profile.Match{Globs: globs},
		Symbols: profile.Symbols{Lang: "php"},
	}
}

// §4.3.1 builds the index from the profile's `symbols.lang`, over the head.
func TestTheHeadIndexHoldsTheDeclarationsOfTheSourceTheProfileOwns(t *testing.T) {
	dir := headOf(t, map[string]string{
		"app/Orders/Refund.php":  "<?php\n\nclass Refund\n{\n    public function handle(Order $o): void\n    {\n    }\n}\n",
		"vendor/laravel/Str.php": "<?php\n\nclass Str\n{\n    public function slug(string $s): string\n    {\n    }\n}\n",
		"README.md":              "# fixture\n",
	})

	index, built, err := Head(dir, "HEAD", phpProfile("app/**/*.php"))

	require.NoError(t, err)
	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "app/Orders/Refund.php", Line: 3, Name: "Refund", Kind: Class},
		{Path: "app/Orders/Refund.php", Line: 5, Name: "handle", Kind: Method, Params: 1},
	}, index.Decls,
		"a profile's match.globs is its own statement of what its source is, "+
			"and a candidate drawn from vendor/ is a question about code nobody here maintains")
}

// A profile declaring no `symbols.lang`, and one naming a language cr has no
// scanner for, are the two states §4.3.1 sends to §4.5.4. Both are answered
// before any git read, and neither comes back as an index holding nothing.
func TestAHeadCrCannotIndexIsReportedRatherThanReturnedEmpty(t *testing.T) {
	dir := headOf(t, map[string]string{"app/Order.php": "<?php\nclass Order {}\n"})

	for name, lang := range map[string]string{
		"the generic profile declares no symbols.lang": "",
		"a language cr ships no scanner for":           "cobol",
	} {
		t.Run(name, func(t *testing.T) {
			p := phpProfile("app/**/*.php")
			p.Symbols.Lang = lang

			index, built, err := Head(dir, "HEAD", p)

			require.NoError(t, err, "a head cr cannot index is not a failed run")
			assert.False(t, built)
			assert.Nil(t, index)
		})
	}
}

// A profile owning no source indexes nothing, which is the same statement its
// empty `match.globs` already makes.
func TestAProfileOwningNoSourceIndexesNothing(t *testing.T) {
	dir := headOf(t, map[string]string{"app/Order.php": "<?php\nclass Order {}\n"})

	index, built, err := Head(dir, "HEAD", phpProfile())

	require.NoError(t, err)
	require.True(t, built)
	assert.Empty(t, index.Decls)
}

// Invariant 2: cr never writes inside the repository under review. Building an
// index is the largest read cr makes of a head — every source file of it — so
// it is the one worth asserting leaves the worktree, the index, and HEAD as it
// found them.
func TestBuildingTheIndexWritesNothingInsideTheRepositoryUnderReview(t *testing.T) {
	dir := headOf(t, map[string]string{
		"app/Order.php":  "<?php\nclass Order {}\n",
		"app/Refund.php": "<?php\nclass Refund {}\n",
	})
	before := fixtureGit(t, dir, "rev-parse", "HEAD")

	_, built, err := Head(dir, "HEAD", phpProfile("app/**/*.php"))

	require.NoError(t, err)
	require.True(t, built)
	assert.Empty(t, fixtureGit(t, dir, "status", "--porcelain"),
		"invariant 2: the worktree and the index are read-only to cr")
	assert.Equal(t, before, fixtureGit(t, dir, "rev-parse", "HEAD"))
}

// A revision that does not resolve is git refusing, and §3.1.3 makes that an
// error rather than a head that declares nothing.
func TestAHeadThatDoesNotResolveIsAnError(t *testing.T) {
	dir := headOf(t, map[string]string{"app/Order.php": "<?php\nclass Order {}\n"})

	_, built, err := Head(dir, "no-such-revision", phpProfile("app/**/*.php"))

	require.Error(t, err)
	assert.False(t, built)
}
