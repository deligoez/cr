package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/sandbox"
	"github.com/deligoez/cr/internal/state"
)

// briefedWithProfile writes a round of the fixture pull request whose meta.json
// names profileID, with body as that profile's file when profileID is not
// empty, and points the command at the fixture checkout.
func briefedWithProfile(t *testing.T, profileID, body string) (fixture string, layout state.Layout) {
	t.Helper()
	fixture = fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	layout = state.New(root)
	require.NoError(t, layout.Init())
	if profileID != "" {
		require.NoError(t, layout.EnsureProfile(profileID, body))
	}
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: profileID, Round: 1, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })
	return fixture, layout
}

// worktreesListed counts the worktrees git knows of in checkout, the checkout
// itself included. It counts rather than compares, because git reports paths
// with their symlinks resolved.
func worktreesListed(t *testing.T, checkout string) int {
	t.Helper()
	listed := 0
	for line := range strings.SplitSeq(mustGit(t, checkout, "worktree", "list", "--porcelain"), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			listed++
		}
	}
	return listed
}

// QA D-S06-1: a `sandbox.setup` command that fails leaves no half-built
// sandbox behind, so the next `cr sandbox create` runs instead of exiting 4.
//
// Before, the worktree and its `.git/worktrees/` registration stayed after the
// exit 3, and a user who repaired the profile met §5.1.1's refusal to create
// over an existing sandbox. The registration is read off disk as well as asked
// of git, because a removal that deleted the directory and left the entry
// would make `worktree add` refuse the same path.
func TestASandboxWhoseSetupFailedIsRemovedSoTheNextCreateRuns(t *testing.T) {
	refusing := filepath.Join(t.TempDir(), "refuse.sh")
	require.NoError(t, os.WriteFile(refusing, []byte("#!/bin/sh\necho the tool refused >&2\nexit 1\n"), 0o700))
	withSetup := func(setup string) string {
		return `{"id":"qa","match":{"files":[],"globs":[]},"axes":{"correctness":true}` + setup + `}`
	}
	fixture, layout := briefedWithProfile(t, "qa", withSetup(`,"sandbox":{"setup":["`+refusing+`"]}`))
	sandboxPath := layout.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber)

	printed, err := runIn(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	require.Error(t, err)
	assert.Empty(t, printed)
	assert.Equal(t, ExitFile, exitCodeFor(err), "§3.1.3: a failed setup command exits 3")
	var failed *sandbox.SetupError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, "sandbox.setup "+refusing+": exit status 1: the tool refused", err.Error())

	assert.NoDirExists(t, sandboxPath, "the half-built worktree is removed")
	left, readErr := os.ReadDir(filepath.Join(fixture, ".git", "worktrees"))
	if readErr == nil {
		assert.Empty(t, left, "the half-built worktree's registration is removed")
	} else {
		require.True(t, os.IsNotExist(readErr), "%v", readErr)
	}
	assert.Equal(t, 1, worktreesListed(t, fixture), "git knows of no worktree but the checkout itself")
	assert.NoFileExists(t, layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileSandboxBaseline))

	require.NoError(t, os.WriteFile(layout.Profile("qa"), []byte(withSetup("")), 0o600))
	var created map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(throughAPipe(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)), &created),
		"the next creation runs rather than refusing a sandbox that is already there")
	assert.Equal(t, sandboxPath, created["path"])
	assert.DirExists(t, sandboxPath)
	assert.Equal(t, 2, worktreesListed(t, fixture))
}

// QA D-S06-2: a profile that does not declare the field a test run needs is
// refused with a hint naming the profile file, by its path, rather than
// `cr config --resolved`, which prints no profile field at all.
//
// Driven through `cr test` and `cr probe run`, the two commands that build the
// runner's argv. The message and the hint are compared whole.
func TestAProfileMissingATestFieldIsHintedByItsFile(t *testing.T) {
	const (
		cmdNeeds = "§2.4 makes an absent test command a disabled test axis, and §5.2.1 has cr run it " +
			"inside the sandbox"
		flagNeeds = "§5.2.1 passes --filter as that flag, so this profile has no way to narrow a run to a subset"
	)
	for _, fixture := range []struct {
		name      string
		profileID string
		body      string
		field     string
		needs     string
		// named says the hint names a profile file; without one there is
		// none to name.
		named bool
	}{
		{
			name: "no tests.filter_flag", profileID: "qa",
			body: `{"id":"qa","match":{"files":[],"globs":[]},"axes":{"test":true},` +
				`"tests":{"cmd":["/bin/true"],"globs":["*_test.txt"]}}`,
			field: "tests.filter_flag", needs: flagNeeds, named: true,
		},
		{
			name: "no tests.cmd", profileID: "qa",
			body:  `{"id":"qa","match":{"files":[],"globs":[]},"axes":{"test":true}}`,
			field: "tests.cmd", needs: cmdNeeds, named: true,
		},
		{
			name: "no profile resolved", field: "tests.cmd", needs: cmdNeeds,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, layout := briefedWithProfile(t, fixture.profileID, fixture.body)
			file := ""
			message := fmt.Sprintf("no profile was resolved for this pull request, so %s is unset: %s",
				fixture.field, fixture.needs)
			hint := fmt.Sprintf("no profile file was resolved for this round: add a profile declaring %s "+
				"under the profiles directory of cr's state root, select it with match.files or the "+
				"`profile` configuration key, and run `cr brief <pr>` so the round records it", fixture.field)
			if fixture.named {
				file = layout.Profile(fixture.profileID)
				message = fmt.Sprintf("%s: %s is not set: %s", file, fixture.field, fixture.needs)
				hint = fmt.Sprintf("declare %s in the profile file %s, which is the profile this round "+
					"resolved; `cr config --resolved` does not show profile fields", fixture.field, file)
			}
			patch := filepath.Join(t.TempDir(), "mutation.diff")
			require.NoError(t, os.WriteFile(patch, []byte("--- a/app.go\n+++ b/app.go\n"+
				"@@ -3 +3 @@\n-func Retry() { backoff() }\n+func Retry() {}\n"), 0o600))

			for _, args := range [][]string{
				{"test", fixturePR, "--repo", fixtureSlug, "--filter", "retries"},
				{"probe", "run", fixturePR, "--repo", fixtureSlug, "--kind", "mutation",
					"--patch", patch, "--filter", "retries"},
			} {
				printed, err := runIn(t, args...)
				require.Error(t, err, "%v", args)
				assert.Empty(t, printed, "%v", args)
				assert.Equal(t, ExitFile, exitCodeFor(err), "%v", args)
				var unavailable *profile.UnavailableError
				require.ErrorAs(t, err, &unavailable, "%v", args)
				assert.Equal(t, file, unavailable.File, "%v", args)
				assert.Equal(t, message, err.Error(), "%v", args)
				assert.Equal(t, hint, hintFor(err), "%v", args)
			}
		})
	}
}
