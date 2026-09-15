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

// staleTestRun runs `cr test` on a round whose laravel-pest profile file holds
// the given bytes, and returns the printed payload's honesty and the file.
//
// The profile's `composer install` and `./vendor/bin/pest` are the shipped
// ones, so a composer shim on PATH answers the setup by writing a pest that
// exits 0 into the sandbox: the run completes and the payload is printed, which
// is all the notice needs.
func staleTestRun(t *testing.T, onDisk []byte) (honesty []any, file string) {
	t.Helper()
	fixture := fixtureRepository(t)
	root := crHome(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	shims := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(shims, "composer"), []byte(
		"#!/bin/sh\nmkdir -p vendor/bin && printf '#!/bin/sh\\nexit 0\\n' > vendor/bin/pest && chmod +x vendor/bin/pest\n"),
		0o700))
	t.Setenv("PATH", shims+string(os.PathListSeparator)+os.Getenv("PATH"))

	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	file = prepared.Profile("laravel-pest")
	require.NoError(t, os.WriteFile(file, onDisk, 0o600))
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

	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(afterHeader(t, throughAPipe(t,
		"test", fixturePR, "--repo", fixtureSlug))), &printed))
	honesty, ok := printed["honesty"].([]any)
	require.True(t, ok, "§11.1's disclosures are a field on the payload")
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, onDisk, after, "loading a profile writes nothing back")
	return honesty, file
}

// A command that loads a profile file byte-equal to one an earlier release
// shipped says so under honesty, naming the release, what the shipped profile
// changed, and that `cr init` updates it; the same file with one value edited is
// the user's and gets no line. The earlier bytes are the ones extracted from the
// v0.2.1 tag for `cr init`'s update, not a copy typed here.
func TestTheTestCommandNamesAProfileAnEarlierReleaseShipped(t *testing.T) {
	previous, err := os.ReadFile(filepath.Join("..", "profile", "builtin", "shipped", "v0.2.1", "laravel-pest.json"))
	require.NoError(t, err)

	t.Run("a byte-equal v0.2.1 file", func(t *testing.T) {
		honesty, file := staleTestRun(t, previous)

		require.Len(t, honesty, 2, "§5.1.6's recreation notice, then the profile's")
		assert.Equal(t, file+" is the laravel-pest profile cr v0.2.1 shipped, unedited, "+
			"and the shipped profile has since changed sandbox.copy; cr init updates the file to it", honesty[1])
	})

	t.Run("the same file with one key edited", func(t *testing.T) {
		edited := bytes.Replace(previous, []byte(`"lang": "php"`), []byte(`"lang": "hack"`), 1)
		require.NotEqual(t, previous, edited)
		honesty, _ := staleTestRun(t, edited)

		require.Len(t, honesty, 1, "§5.1.6's recreation notice alone")
		assert.Contains(t, honesty[0], "§5.1.6", "and that one line is the recreation notice")
	})
}
