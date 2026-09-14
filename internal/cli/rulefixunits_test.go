package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// unitlessFixHome is a checkout whose pull request adds a panic to lib.go,
// which forms the round's one unit, and another to gen.go, which forms none —
// the file §3.4.2 excludes as generated or §3.4.7 lists. The rule carries a fix
// block, so a hit in either file has a replacement to offer.
//
// lib.go's hit is on head line 4 and gen.go's is on head line 4 too, with
// different matched text, so a suggestion reveals which file it was built from.
func unitlessFixHome(t *testing.T) state.Layout {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("lib.go", "package lib\n\nfunc Load() {}\n")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("lib.go", "package lib\n\nfunc Load() {\n\tpanic(\"one\")\n}\n")
	write("gen.go", "package lib\n\nfunc gen() {\n\tpanic(\"generated\")\n}\n")
	mustGit(t, dir, "add", "lib.go", "gen.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, os.WriteFile(layout.Rule("no-panic"), []byte(panicFixRule), 0o600))
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber, Round: 1, Head: head,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":1,"end":5}],`+
			`"head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// §2.6.1.1 through `cr record`: the fix a record naming a rule receives is
// built from hits over the files that formed a unit, the same scope
// `cr rules check` detects over.
//
// Both records name the rule, anchor on lib.go's hit line and differ only in
// the citation that says which hit they confirm. The one citing lib.go's hit
// takes that hit's replacement and an `origin: rule` citation. The one citing
// gen.go's hit takes nothing: no suggestion, and an `origin: agent` citation,
// because gen.go formed no unit and so no hit of it exists — generating a
// replacement from its line would put text from a file cr declined to review
// under a record in a file it did.
func TestARuleFixIsBuiltOnlyFromHitsInFilesThatFormedAUnit(t *testing.T) {
	layout := unitlessFixHome(t)
	runRulesCheck(t)

	inside := confirming("f1", 4)
	outside := confirming("f2", 4)
	outside["citations"] = []map[string]any{{"path": "gen.go", "line": 4}}

	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", inside, outside), "--repo", fixtureSlug)
	require.NoError(t, err)

	stored := storedFindings(t, layout)
	require.Len(t, stored, 2)
	byID := map[string]*finding.Finding{stored[0].ID: &stored[0], stored[1].ID: &stored[1]}

	assert.Equal(t, "\treturn fmt.Errorf(\"one\")", byID["f1"].Suggestion,
		"§2.6.2.1: a hit inside a unit still attaches its fix")
	assert.Equal(t, finding.OriginRule, byID["f1"].SuggestionOrigin)
	assert.Equal(t, []finding.Citation{{Path: "lib.go", Line: 4, Origin: finding.OriginRule}},
		citedPlaces(byID["f1"]))

	assert.Empty(t, byID["f2"].Suggestion,
		"§2.6.1.1: gen.go formed no unit, so no fix is built from its line")
	assert.Empty(t, byID["f2"].SuggestionOrigin)
	assert.Equal(t, []finding.Citation{{Path: "gen.go", Line: 4, Origin: finding.OriginAgent}},
		citedPlaces(byID["f2"]), "§6.2.5: no hit exists outside the units to confirm")
}

// citedPlaces is a record's citations without the content hash `cr record`
// computes for each, which says nothing about which hit a citation confirms.
func citedPlaces(record *finding.Finding) []finding.Citation {
	places := make([]finding.Citation, 0, len(record.Citations))
	for _, citation := range record.Citations {
		citation.ContentHash = ""
		places = append(places, citation)
	}
	return places
}
