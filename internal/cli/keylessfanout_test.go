package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// keylessHome is statusHome's checkout behind a pull request whose branch, title
// and body name no issue key, with the `generic` profile configured, and nothing
// briefed yet.
func keylessHome(t *testing.T) state.Layout {
	t.Helper()
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.go"), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("package lib\n\nfunc Load() {}\n")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("package lib\n\nfunc Load() {\n\tparse()\n\tstore()\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })

	shim, answers := t.TempDir(), t.TempDir()
	pr := filepath.Join(answers, "pr.json")
	require.NoError(t, os.WriteFile(pr, []byte(`{"data":{"repository":{"pullRequest":{`+
		`"number":`+fixturePR+`,"title":"retry the upload","body":"","headRefName":"`+fixtureHeadBranch+`",`+
		`"headRefOid":"`+head+`","baseRefName":"main","baseRefOid":"`+base+`"}}}}`), 0o600))
	threads := filepath.Join(answers, "threads.json")
	require.NoError(t, os.WriteFile(threads, []byte(`{"data":{"repository":{"pullRequest":{`+
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}}}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(shim, "gh"), []byte(
		"#!/bin/sh\ncase \"$*\" in\n  *reviewThreads*) exec cat "+threads+" ;;\n  *) exec cat "+pr+" ;;\nesac\n"), 0o700))
	t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	config := layout.RepoConfig(fixtureOwner, fixtureProject)
	require.NoError(t, os.MkdirAll(filepath.Dir(config), 0o700))
	require.NoError(t, os.WriteFile(config, []byte(`{"profile":"generic"}`), 0o600))
	return layout
}

// fanned is the part of `cr review`'s document this test reads.
type fanned struct {
	Prompts []review.Prompt `json:"prompts"`
}

// fanOut runs `cr review` with args and decodes its prompts.
func fanOut(t *testing.T, args ...string) fanned {
	t.Helper()
	printed, err := runCLIPrinting(t, append([]string{"review", fixturePR, "--repo", fixtureSlug}, args...)...)
	require.NoError(t, err)
	var out fanned
	require.NoError(t, json.Unmarshal([]byte(printed), &out))
	return out
}

// §4.6.6 through the whole fan-out: a pull request that resolves no issue key
// is briefed, reviewed across every active axis without `cr map record` being
// required, refused `cr map record` when one is attempted, filled cell by cell,
// and reported — and at no step does a unit become an unmapped-unit question or
// a claim an intent-gap entry. An absent tracker is not an unmapped unit.
func TestAKeylessPullRequestRaisesNoUnmappedUnitAndNoIntentGap(t *testing.T) {
	layout := keylessHome(t)
	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var briefed struct {
		Axes struct {
			Unavailable []struct {
				Axis string `json:"axis"`
			} `json:"unavailable"`
		} `json:"axes"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	require.Len(t, briefed.Axes.Unavailable, 1, "the fixture is only worth anything if §4.5.3 applies")
	require.Equal(t, axis.Intent, briefed.Axes.Unavailable[0].Axis)

	all := fanOut(t)
	require.NotEmpty(t, all.Prompts, "§4.6.6: the remaining axes wait for no mapping")
	for _, prompt := range all.Prompts {
		assert.NotEqual(t, axis.Intent, prompt.Axis, "§4.6.6: there is no intent role")
		assert.NotContains(t, prompt.Text, "Unmapped unit (§4.1.2)", "%s on %s", prompt.Role, prompt.Unit)
		assert.Contains(t, prompt.Text, "its mapping is empty (§4.6.6)", "%s on %s", prompt.Role, prompt.Unit)
	}
	assert.Empty(t, fanOut(t, "--axis", axis.Intent).Prompts, "no intent pass exists to run")

	empty := filepath.Join(t.TempDir(), "mapping.ndjson")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	refused := runCLI(t, "map", "record", fixturePR, empty, "--repo", fixtureSlug)
	var notAccepted *mapping.NotAcceptedError
	require.ErrorAs(t, refused, &notAccepted, "§4.6.6: cr map record is not accepted")
	assert.Equal(t, ExitState, exitCodeFor(refused))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	assert.False(t, meta.MappingRecorded(), "the refused run stamped nothing")

	cells := make([]string, 0, len(all.Prompts))
	for _, prompt := range all.Prompts {
		cells = append(cells, `{"unit":"`+prompt.Unit+`","role":"`+prompt.Role+`","result":"pass"}`)
	}
	cellFile := filepath.Join(t.TempDir(), "cells.ndjson")
	require.NoError(t, os.WriteFile(cellFile, []byte(strings.Join(cells, "\n")+"\n"), 0o600))
	require.NoError(t, runCLI(t, "cells", "record", fixturePR, cellFile, "--repo", fixtureSlug))

	// A mapping stamp a build before §4.6.6's refusal could have left
	// behind makes the round read as mapped; its units are still not
	// unmapped, because the axis that would map them never ran.
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.StampMapping(meta.Round, meta.Head))
	require.NoError(t, held.Unlock())
	for _, prompt := range fanOut(t).Prompts {
		assert.NotContains(t, prompt.Text, "Unmapped unit (§4.1.2)", "%s on %s", prompt.Role, prompt.Unit)
		assert.Contains(t, prompt.Text, "its mapping is empty (§4.6.6)", "%s on %s", prompt.Role, prompt.Unit)
	}

	status, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report struct {
		Intent intentCoverage `json:"intent"`
	}
	require.NoError(t, json.Unmarshal([]byte(status), &report))
	assert.Empty(t, report.Intent.Gaps, "§4.1.3 raises no entry for a round with no tracker")
	stored, err := layout.ReadPR(fixtureOwner, fixtureProject, fixturePRNumber, state.FileIntentGaps)
	require.NoError(t, err)
	assert.Empty(t, stored, "intent-gaps.ndjson holds no entry")
}
