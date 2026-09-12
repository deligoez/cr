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

// reinventionHome is a round whose head adds formatMoneys in order.go while
// money.go already declares FormatMoney with the same parameter count, under a
// profile that indexes Go, so §4.3.1 attaches money.go:3 as a candidate to the
// symbol the round's one unit adds.
func reinventionHome(t *testing.T) state.Layout {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("money.go", "package shop\n\nfunc FormatMoney(amount int) string {\n\treturn \"\"\n}\n")
	mustGit(t, dir, "add", "money.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the existing helper")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("order.go", "package shop\n\nfunc formatMoneys(amount int) string {\n\treturn \"\"\n}\n")
	mustGit(t, dir, "add", "order.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "a second helper")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsureProfile("shop",
		`{"id":"shop","match":{"files":[],"globs":["**/*.go"]},`+
			`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},"symbols":{"lang":"go"}}`))
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		ProfileID: "shop", Round: 1, Head: head,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"order.go","side":"RIGHT","hunk_ranges":[{"start":1,"end":5}],`+
			`"head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// aReinventionRecord is a convention finding on the added helper, citing one
// line of money.go.
func aReinventionRecord(id string, citedLine int) map[string]any {
	return map[string]any{
		"id": id, "kind": "finding", "role": "convention", "class": "reinvented-helper",
		"severity": "medium", "unit": "u1",
		"anchor":    map[string]any{"path": "order.go", "side": "RIGHT", "start_line": 3, "line": 3},
		"summary":   "formatMoneys repeats FormatMoney.",
		"evidence":  "Both take an amount and return a string.",
		"citations": []map[string]any{{"path": "money.go", "line": citedLine}},
	}
}

// §4.3.3 and §4.3.4 through `cr record`: a record citing the candidate §4.3.1
// attached, as path:line, is stored as a question, and a record citing another
// line of the same file keeps the kind the agent gave it.
//
// Both are graded `cited` — the citation lies outside the record's own unit —
// so §6.3's forcing reaches neither. The question on f1 is therefore the
// default §4.3.4 applies before the forcing, not the forcing itself.
func TestAReinventionItemDefaultsToAQuestionBeforeTheForcing(t *testing.T) {
	layout := reinventionHome(t)

	_, err := runCLIPrinting(t, "record", fixturePR,
		writeRecordFile(t, "merged.ndjson", aReinventionRecord("f1", 3), aReinventionRecord("f2", 1)),
		"--repo", fixtureSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	assert.Equal(t, finding.GradeCited, stored[0].Grade, "§6.3's forcing does not reach a cited record")
	assert.Equal(t, finding.KindQuestion, stored[0].Kind,
		"§4.3.4: citing the candidate money.go:3 makes the record a reinvention item, which defaults to a question")
	assert.Equal(t, finding.GradeCited, stored[1].Grade)
	assert.Equal(t, finding.KindFinding, stored[1].Kind,
		"a citation naming no candidate leaves the agent's kind alone")
}
