package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// emptyRoundHome is a checkout whose pull request changes nothing — its head
// branch is one empty commit over main — briefed through `cr brief`, so the
// round it leaves is the one the command forms from that diff rather than one
// a fixture wrote by hand.
func emptyRoundHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package lib\n\nfunc Load() {}\n"), 0o600))
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	mustGit(t, dir, "commit", "--quiet", "--allow-empty", "-m", "a change that changes nothing")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": load the configuration.\n"), 0o600))

	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	var briefed struct {
		Units []json.RawMessage `json:"units"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	require.Empty(t, briefed.Units, "the fixture is only worth anything if its diff yields no unit")
}

// A round whose diff yielded no unit is not reported complete, in either
// output shape, and the verdict says why (§10.2).
//
// Measured before the fix on this fixture's shape: `cr status` printed "round
// complete, per §10.2" over zero units, because every condition over an empty
// set holds. The verdict still reaches the reader only through
// coverage.Lenses.Verdict, so the lenses that did not run are printed beside
// it exactly as they are beside any other verdict.
func TestARoundThatFormedNoUnitIsNotReportedComplete(t *testing.T) {
	emptyRoundHome(t)

	report := readCompleteness(t)
	shown := throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color")

	reason := "§10.2.2: this round formed no unit from its diff, so no cell was filled " +
		"and there is no row of cells its coverage could be complete over"
	assert.False(t, report.Completeness.Complete)
	assert.Equal(t, []string{reason}, report.Completeness.Reasons)
	for shape, said := range map[string]string{
		"the document": strings.Join(report.Honesty, "\n"), "the terminal": shown,
	} {
		assert.Containsf(t, said, "round not complete, per §10.2: "+reason, "%s", shape)
		assert.NotContainsf(t, said, "round complete, per §10.2", "%s", shape)
		assert.Containsf(t, said, "lens convention/reinvention unavailable, per §4.3.1",
			"%s: the verdict is still printed together with the lenses that did not run", shape)
	}
}
