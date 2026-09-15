package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// laravelPestSandbox runs `cr sandbox create` against a fixture clone holding
// the given untracked files, with the laravel-pest profile exactly as `cr init`
// writes it, and returns the printed payload beside the sandbox path.
//
// The profile is the shipped one rather than a copy of its list, because the
// list is what is under test. Its one setup command is `composer install`, so a
// shim answers for composer on PATH: the test is about what §5.1.2 copied, and
// a real composer would reach the network.
func laravelPestSandbox(t *testing.T, files map[string]string) (printed map[string]any, sandboxPath string) {
	t.Helper()
	fixture := fixtureRepository(t)
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(fixture, name), []byte(body), 0o600))
	}
	root := crHome(t)
	runInit(t)

	shims := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(shims, "composer"), []byte("#!/bin/sh\nexit 0\n"), 0o700))
	t.Setenv("PATH", shims+string(os.PathListSeparator)+os.Getenv("PATH"))

	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))
	prepared := state.New(root)
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "laravel-pest", Round: 1, Head: head,
	}))
	require.NoError(t, held.Unlock())

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	require.NoError(t, json.Unmarshal(
		[]byte(throughAPipe(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)), &printed))
	return printed, prepared.Sandbox(fixtureOwner, fixtureProject, fixturePRNumber)
}

// The shipped laravel-pest profile copies `.env.testing` into the sandbox
// beside `.env` (spec/field-feedback.md, 2.1).
//
// Laravel runs its tests under APP_ENV=testing and reads `.env.testing` when it
// exists. A sandbox without it was measured, in a field trial, running the
// suite against the database `.env` names — the developer's own. So the file
// has to land in the sandbox whenever the clone holds one, and a clone that
// holds none is §5.1.2's ordinary absent path: named, and not a failure.
func TestTheLaravelPestSandboxCarriesTheTestingEnvironmentFile(t *testing.T) {
	t.Run("a clone holding both files", func(t *testing.T) {
		printed, sandboxPath := laravelPestSandbox(t, map[string]string{
			".env":         "DB_DATABASE=app\n",
			".env.testing": "DB_DATABASE=app_testing\n",
		})

		assert.Equal(t, []any{".env", ".env.testing"}, printed["copied"])
		assert.Equal(t, []any{"vendor"}, printed["absent"])
		for name, body := range map[string]string{
			".env":         "DB_DATABASE=app\n",
			".env.testing": "DB_DATABASE=app_testing\n",
		} {
			landed, err := os.ReadFile(filepath.Join(sandboxPath, name))
			require.NoError(t, err, "%s is not in the sandbox", name)
			assert.Equal(t, body, string(landed))
		}
	})

	t.Run("a clone holding only .env", func(t *testing.T) {
		printed, sandboxPath := laravelPestSandbox(t, map[string]string{
			".env": "DB_DATABASE=app\n",
		})

		assert.Equal(t, []any{".env"}, printed["copied"])
		assert.Equal(t, []any{".env.testing", "vendor"}, printed["absent"],
			"§5.1.2 names the path the clone does not hold and creates the sandbox anyway")
		assert.NoFileExists(t, filepath.Join(sandboxPath, ".env.testing"))
		assert.FileExists(t, filepath.Join(sandboxPath, ".env"))
	})
}
