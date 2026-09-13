package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// The sandbox a rendering is asked to print. The head is a full revision and
// the path a real §2.2 one, because both are values the reader is expected to
// paste somewhere else.
const (
	renderedSandboxPath = "/home/dev/.cr/state/acme/web/pr-42/sandbox"
	renderedSandboxHead = "be7e2c75aeb661ba3c96d7a7634f3f8e84bb7b91"
)

// renderedSandbox returns `cr sandbox create`'s payload as one mode prints it.
func renderedSandbox(t *testing.T, mode Mode) string {
	t.Helper()
	var printed bytes.Buffer
	out := &writer{out: &printed, mode: mode}
	require.NoError(t, out.emit(&sandboxCreateResult{
		Path: renderedSandboxPath, Head: renderedSandboxHead,
	}))
	return printed.String()
}

// Both renderings name the worktree and the revision in it.
//
// The two are asserted separately because they fail apart. The JSON document is
// what an agent parses, so it is decoded and each value looked up by its key;
// the terminal rendering is what a person reads, so it is searched for the
// values themselves. A payload that carried both while printing neither would
// pass an assertion on the document alone — and the head in particular is the
// value §5.1.6 checks the sandbox against before every later run, so a reader
// told only that something was created has been told nothing they can check.
func TestTheSandboxCreationNamesTheWorktreeAndItsHead(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var printed map[string]any
		require.NoError(t, json.Unmarshal([]byte(renderedSandbox(t, ModeJSON)), &printed))

		assert.Equal(t, renderedSandboxPath, printed["path"])
		assert.Equal(t, renderedSandboxHead, printed["head"])
	})

	t.Run("terminal", func(t *testing.T) {
		printed := renderedSandbox(t, ModeText)

		assert.Contains(t, printed, renderedSandboxPath)
		assert.Contains(t, printed, renderedSandboxHead)
	})
}

// §5.1.1: the worktree is checked out at the *pull request* head, which is the
// one meta.json recorded and not the one the checkout happens to be on.
//
// The fixture's own HEAD is a different commit, and that is the whole design of
// this test: a command that ran `git worktree add` against the current checkout
// would produce a sandbox that looks perfectly healthy and holds code no
// finding of this round was written against. §5.1.6 would then fail the head
// check on the sandbox's first use, after §5.1.3's setup had already run.
func TestTheSandboxIsCheckedOutAtTheRoundsRecordedHead(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))
	require.NotEqual(t, head, strings.TrimSpace(mustGit(t, fixture, "rev-parse", "HEAD")),
		"the fixture must be sitting on some other commit, or this test cannot tell the two apart")

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 1, Head: head,
	}))
	require.NoError(t, held.Unlock())

	// The repository under review is the directory cr was run from, and
	// this suite runs from internal/cli. The seam is the same one
	// `cr brief` reaches the checkout through.
	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	var printed map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(throughAPipe(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)), &printed))

	assert.Equal(t, prepared.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber), printed["path"])
	assert.Equal(t, head, printed["head"])
	created, ok := printed["path"].(string)
	require.True(t, ok)
	assert.Equal(t, head, strings.TrimSpace(mustGit(t, created, "rev-parse", "HEAD")))
}

// Both renderings say what §5.1.2 copied, what it could not, and what §5.1.3
// ran.
//
// The absent list is the one that has to be printed rather than counted. A
// `sandbox.copy` path the checkout does not hold is not an error — a fresh
// clone has no `vendor` — but it is the reason a suite later fails to start,
// and a reader told only that one path of two was copied cannot tell which.
func TestTheSandboxReportsWhatWasCopiedAndWhatWasRun(t *testing.T) {
	prepared := &sandboxCreateResult{
		Path: renderedSandboxPath, Head: renderedSandboxHead,
		Copied: []string{".env"},
		Absent: []string{"vendor"},
		Setup:  []string{"composer install --no-interaction"},
	}
	render := func(t *testing.T, mode Mode) string {
		t.Helper()
		var printed bytes.Buffer
		out := &writer{out: &printed, mode: mode}
		require.NoError(t, out.emit(prepared))
		return printed.String()
	}

	t.Run("json", func(t *testing.T) {
		var printed map[string]any
		require.NoError(t, json.Unmarshal([]byte(render(t, ModeJSON)), &printed))

		assert.Equal(t, []any{".env"}, printed["copied"])
		assert.Equal(t, []any{"vendor"}, printed["absent"])
		assert.Equal(t, []any{"composer install --no-interaction"}, printed["setup"])
	})

	t.Run("terminal", func(t *testing.T) {
		printed := render(t, ModeText)

		assert.Contains(t, printed, "copied .env")
		assert.Contains(t, printed, "absent vendor")
		assert.Contains(t, printed, "setup  composer install --no-interaction")
	})
}

// §5.1.2 and §5.1.3 come out of the profile the round resolved, and the command
// carries them out in that order.
//
// meta.json's `profile_id` is what is read, because §3.7 makes `cr brief` the
// one writer of it: a command that re-selected a profile of its own could
// prepare the sandbox for one profile while every role of the round was briefed
// against another. The setup script reads the copied file and writes what it
// found outside the sandbox, so the log is evidence of the order rather than a
// restatement of it.
func TestTheSandboxRunsTheProfilesCopyAndSetupSteps(t *testing.T) {
	fixture := fixtureRepository(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(fixture, ".env"), []byte("APP_ENV=testing\n"), 0o600))
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	scripts := t.TempDir()
	log := filepath.Join(scripts, "observed.log")
	setup := filepath.Join(scripts, "setup.sh")
	require.NoError(t, os.WriteFile(setup, []byte("#!/bin/sh\ncat .env >> "+log+" 2>&1\n"), 0o700))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"correctness":true},`+
		`"sandbox":{"copy":[".env","vendor"],"setup":["`+setup+`"]}}`))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "qa", Round: 1, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	var printed map[string]any
	require.NoError(t, json.Unmarshal(
		[]byte(throughAPipe(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)), &printed))

	assert.Equal(t, []any{".env"}, printed["copied"])
	assert.Equal(t, []any{"vendor"}, printed["absent"],
		"the fixture holds no vendor tree, and §5.1.2 reports the path rather than failing on it")
	assert.Equal(t, []any{setup}, printed["setup"])

	observed, err := os.ReadFile(log)
	require.NoError(t, err, "the setup command never ran")
	assert.Equal(t, "APP_ENV=testing\n", string(observed),
		"§5.1.3's command ran in the sandbox, after §5.1.2 had copied into it")
}

// §5.1.5: `cr sandbox destroy` removes the worktree **and its registration**.
//
// The registration is what this test is really about. A `git worktree remove`
// that succeeded and left `.git/worktrees/<name>/` behind would look like a
// clean destruction from every angle a user can see — the directory is gone,
// `git status` is clean — and would make the next `cr sandbox create` fail on
// the one path it is entitled to, because `worktree add` refuses a path that
// is still registered. So the registration directory is read off disk rather
// than inferred from git's own report of it.
//
// Running it twice is the other half. §5.1.5 asks for the worktree to be gone,
// and a pull request that never had one is already in that state; the command
// says so instead of failing, and still prunes, since a registration naming a
// directory somebody deleted by hand is exactly the leftover it exists to
// clear.
func TestDestroyingTheSandboxLeavesNoWorktreeRegistrationBehind(t *testing.T) {
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 1, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	registrations := filepath.Join(fixture, ".git", "worktrees")
	require.NoDirExists(t, registrations, "the fixture starts with no worktree of its own")

	throughAPipe(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	sandboxPath := prepared.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber)
	require.DirExists(t, sandboxPath)
	registered, err := os.ReadDir(registrations)
	require.NoError(t, err)
	require.Len(t, registered, 1, "the creation registered the worktree it made")

	var destroyed map[string]any
	require.NoError(t, json.Unmarshal([]byte(throughAPipe(t,
		"sandbox", "destroy", fixturePR, "--repo", fixtureSlug)), &destroyed))
	assert.Equal(t, sandboxPath, destroyed["path"])
	assert.Equal(t, true, destroyed["removed"])

	assert.NoDirExists(t, sandboxPath, "§5.1.5: the worktree is gone")
	left, err := os.ReadDir(registrations)
	if err == nil {
		assert.Empty(t, left, "§5.1.5: .git/worktrees/ carries no leftover entry")
	} else {
		require.True(t, os.IsNotExist(err), "%v", err)
	}
	// Counted rather than compared against the fixture's own path, which
	// git reports with its symlinks resolved.
	listed := 0
	for line := range strings.SplitSeq(mustGit(t, fixture, "worktree", "list", "--porcelain"), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			listed++
		}
	}
	assert.Equal(t, 1, listed, "git knows of no worktree but the checkout itself")
	assert.NoFileExists(t,
		prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileSandboxBaseline),
		"§5.1.6's baseline described the sandbox that has just been removed")

	var again map[string]any
	require.NoError(t, json.Unmarshal([]byte(throughAPipe(t,
		"sandbox", "destroy", fixturePR, "--repo", fixtureSlug)), &again))
	assert.Equal(t, false, again["removed"],
		"a pull request with no sandbox is told so, rather than being refused")
}
