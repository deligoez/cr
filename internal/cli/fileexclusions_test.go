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
	"github.com/deligoez/cr/internal/unit"
)

// exclusionsHome is a pull request whose head changes one source file and adds
// three that must form no unit: vendor/dep.go, which the repository's
// `ignore.globs` excludes (§3.4.2); logo.png, which git diffs as binary; and
// api.pb.go, which the base's .gitattributes declares linguist-generated
// (§3.4.7). It is briefed through `cr brief`, so the round is the one the
// command forms from that diff.
func exclusionsHome(t *testing.T) state.Layout {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("lib.go", "package lib\n\nfunc Load() {}\n")
	write(".gitattributes", "*.pb.go linguist-generated\n")
	mustGit(t, dir, "add", "lib.go", ".gitattributes")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("lib.go", "package lib\n\nfunc Load() {\n\tparse()\n}\n")
	write("vendor/dep.go", "package dep\n\nfunc Dep() {}\n")
	write("logo.png", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	write("api.pb.go", "package lib\n\nfunc Generated() {}\n")
	mustGit(t, dir, "add", "lib.go", "vendor/dep.go", "logo.png", "api.pb.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the change under review")
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
	require.NoError(t, layout.EnsureRepo(fixtureOwner, fixtureProject))
	ignoring(t, layout, "vendor/**")
	return layout
}

// ignoring sets the repository's `ignore.globs`.
func ignoring(t *testing.T, layout state.Layout, globs ...string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"ignore": map[string]any{"globs": globs}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(layout.RepoConfig(fixtureOwner, fixtureProject), body, 0o600))
}

// briefExclusions runs `cr brief` over exclusionsHome's pull request.
func briefExclusions(t *testing.T) (units []unit.Unit, files unit.Files) {
	t.Helper()
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": load parses.\n"), 0o600))
	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	var briefed struct {
		Units []unit.Unit `json:"units"`
		Files unit.Files  `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	return briefed.Units, briefed.Files
}

// statusExclusions runs `cr status` and reads its files report and honesty.
func statusExclusions(t *testing.T) (files unit.Files, honesty []string) {
	t.Helper()
	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report struct {
		Files   unit.Files `json:"files"`
		Honesty []string   `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report.Files, report.Honesty
}

// An ignored file is excluded and counted, a binary and a generated file are
// listed, none of the three produces a unit, and both counts reach `cr status`
// (§3.4.2, §3.4.7).
func TestExcludedAndListedFilesFormNoUnitAndReachTheStatusReport(t *testing.T) {
	exclusionsHome(t)

	units, briefed := briefExclusions(t)
	paths := make([]string, 0, len(units))
	for i := range units {
		paths = append(paths, units[i].Path)
	}
	assert.Equal(t, []string{"lib.go"}, paths,
		"§3.4.2 and §3.4.7: the ignored, binary and generated files form no unit")
	want := unit.Files{Excluded: 1, Listed: []unit.Listed{
		{Path: "api.pb.go", Kind: unit.KindGenerated},
		{Path: "logo.png", Kind: unit.KindBinary},
	}}
	assert.Equal(t, want, briefed, "the brief counts the excluded file and lists the other two")

	reported, honesty := statusExclusions(t)
	assert.Equal(t, want, reported, "`cr status` re-derives the same count and listing at the round's head")
	assert.NotContains(t, strings.Join(honesty, "\n"), "§3.4.2:",
		"globs unchanged since the brief disclose no drift")

	shown := throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color")
	for _, expected := range []string{
		"1 file(s) excluded by ignore.globs, per §3.4.2; 2 listed and not clustered, per §3.4.7",
		"  api.pb.go (generated)",
		"  logo.png (binary)",
	} {
		assert.Contains(t, shown, expected)
	}
}

// `ignore.globs` changed after the round was formed is disclosed in both
// directions rather than re-counted silently: a path the globs now exclude that
// holds a unit, and a path they no longer exclude that holds none.
func TestStatusDisclosesGlobsThatDriftedFromTheRoundsUnits(t *testing.T) {
	layout := exclusionsHome(t)
	briefExclusions(t)

	ignoring(t, layout, "lib.go")
	files, honesty := statusExclusions(t)

	assert.Equal(t, 1, files.Excluded, "the count is the one the globs resolve to now")
	assert.Contains(t, honesty,
		"§3.4.2: lib.go holds a unit of round 1, but ignore.globs as resolved now excludes it, "+
			"so the excluded count is not the one the round was formed under")
	assert.Contains(t, honesty,
		"§3.4.2: vendor/dep.go holds no unit of round 1, but ignore.globs as resolved now does not exclude it, "+
			"so the excluded count is not the one the round was formed under")
}
