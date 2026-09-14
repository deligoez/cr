package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// unitFilesHome is a pull request whose head adds a panic to lib.go, which forms
// a unit, and the same panic to two files that form none: vendor/dep.go, which
// `ignore.globs` excludes (§3.4.2), and api.pb.go, which the base's
// .gitattributes declares linguist-generated (§3.4.7). The no-panic rule
// matches all three added lines, each on line 4 of its file. The round is
// opened through `cr brief`, so the units are the ones the command forms.
func unitFilesHome(t *testing.T) state.Layout {
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
	write("lib.go", "package lib\n\nfunc Load() {\n\tpanic(\"one\")\n}\n")
	write("vendor/dep.go", "package dep\n\nfunc Dep() {\n\tpanic(\"vendored\")\n}\n")
	write("api.pb.go", "package lib\n\nfunc Generated() {\n\tpanic(\"generated\")\n}\n")
	mustGit(t, dir, "add", "lib.go", "vendor/dep.go", "api.pb.go")
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
	require.NoError(t, os.WriteFile(layout.Rule("no-panic"), []byte(panicRule), 0o600))

	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": load panics.\n"), 0o600))
	_, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	return layout
}

// place is where a hit or a ledger entry points.
type place struct {
	Path string
	Line int
}

// ledgerHits are the hit entries of the fixture repository's rule ledger.
func ledgerHits(t *testing.T, layout state.Layout) []place {
	t.Helper()
	held, err := rule.ReadStats(layout, fixtureOwner, fixtureProject)
	require.NoError(t, err)
	hits := make([]place, 0)
	for i := range held {
		if held[i].Event == rule.EventHit {
			hits = append(hits, place{held[i].Path, held[i].Line})
		}
	}
	return hits
}

// Rule detection reads only the changed files that formed a unit after §3.4's
// exclusion and listing: `cr rules check` reports and stores no hit in the
// ignored or the generated file, a record citing the generated file's matched
// line is stamped `origin: agent`, and `cr review` attaches and stores none.
//
// Measured before rules-detect-unit-files-only (QA D-W1-5): `cr rules check`
// reported and stored the hits at api.pb.go:4 and vendor/dep.go:4 beside
// lib.go:4, attached to no unit.
func TestRuleHitsAreDetectedOnlyInFilesThatFormedAUnit(t *testing.T) {
	layout := unitFilesHome(t)

	var checked rulesCheckResult
	require.NoError(t, json.Unmarshal(runRulesCheck(t), &checked))
	reported := make([]place, 0, len(checked.Hits))
	for _, hit := range checked.Hits {
		reported = append(reported, place{hit.Path, hit.Line})
	}
	assert.Equal(t, []place{{"lib.go", 4}}, reported, "cr rules check reports hits in lib.go alone")
	require.Len(t, checked.Units, 1)
	assert.Equal(t, []rule.Hit{checked.Hits[0]}, checked.Units[0].Hits)
	assert.Equal(t, []place{{"lib.go", 4}}, ledgerHits(t, layout), "cr rules check stores hits in lib.go alone")

	record := confirming("f1", 4)
	record["citations"] = []map[string]any{{"path": "lib.go", "line": 4}, {"path": "api.pb.go", "line": 4}}
	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", record), "--repo", fixtureSlug)
	require.NoError(t, err)
	stored := storedFindings(t, layout)
	require.Len(t, stored, 1)
	origins := map[place]finding.Origin{}
	for _, citation := range stored[0].Citations {
		origins[place{citation.Path, citation.Line}] = citation.Origin
	}
	assert.Equal(t, map[place]finding.Origin{
		{"lib.go", 4}:    finding.OriginRule,
		{"api.pb.go", 4}: finding.OriginAgent,
	}, origins, "a citation into the generated file has no detector behind it")

	pairs := filepath.Join(t.TempDir(), "mapping.ndjson")
	require.NoError(t, os.WriteFile(pairs, nil, 0o600))
	_, err = runCLIPrinting(t, "map", "record", fixturePR, pairs, "--repo", fixtureSlug)
	require.NoError(t, err)
	printed, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var fanout struct {
		Prompts []struct {
			Role string `json:"role"`
			Text string `json:"prompt"`
		} `json:"prompts"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &fanout))
	require.NotEmpty(t, fanout.Prompts)
	for _, prompt := range fanout.Prompts {
		hitLines := make([]string, 0)
		for line := range strings.SplitSeq(prompt.Text, "\n") {
			if strings.HasPrefix(line, "- rule ") {
				hitLines = append(hitLines, line)
			}
		}
		assert.Equal(t, []string{`- rule no-panic at lib.go:4: panic("one")`}, hitLines,
			"the %s prompt attaches the lib.go hit alone", prompt.Role)
	}
	assert.Equal(t, []place{{"lib.go", 4}}, ledgerHits(t, layout), "cr review stores hits in lib.go alone")
}
