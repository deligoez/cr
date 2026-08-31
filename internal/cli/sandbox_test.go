package cli

import (
	"bytes"
	"encoding/json"
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
