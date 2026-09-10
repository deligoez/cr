package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// panicRule is the rule the round is checked against: a detector for a call a
// library must not make, declaring `kind: finding` and `severity: critical`, so
// what a hit is not can be read against what the rule would like it to be.
const panicRule = `{"id":"no-panic","title":"A library returns errors rather than panicking.",` +
	`"rationale":"A panic in a library takes down every caller's process.",` +
	`"class":"panic-in-library","kind":"finding","severity":"critical",` +
	`"detect":{"mode":"regex","pattern":"panic\\("}}`

// detectedHome is a checkout whose pull request adds three panics, a state root
// holding that pull request briefed at round 1 with the one unit its change
// forms, and a `gh` first on the PATH answering with the checkout's own two
// revisions.
//
// The change rewrites line 3 of lib.go and adds lines 4 to 7, three of which
// call panic, so §2.6.1.1's evaluation has exactly three hits to report — on
// lines 4, 5 and 6 of the head — and §3.4's one unit spans lines 3 to 7.
func detectedHome(t *testing.T) state.Layout {
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
	write("package lib\n\nfunc Load() {\n\tpanic(\"one\")\n\tpanic(\"two\")\n\tpanic(\"three\")\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, os.WriteFile(layout.Rule("no-panic"), []byte(panicRule), 0o600))
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber, Round: 1, Head: head,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":3,"end":7}],`+
			`"head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// runRulesCheck runs `cr rules check` against the fixture's pull request and
// returns the JSON document it printed.
func runRulesCheck(t *testing.T) []byte {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"rules", "check", fixturePR, "--repo", fixtureSlug})
	require.NoError(t, cmd.Execute())
	return out.Bytes()
}

// storedFindings reads findings.ndjson for the fixture's pull request.
func storedFindings(t *testing.T, layout state.Layout) []finding.Finding {
	t.Helper()
	stored, err := state.ReadRecords[finding.Finding](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	return stored
}

// confirming is the record an agent writes to confirm one hit: §2.6 item 3's
// rule id, and §2.6.1.3's citation of the matched path and line.
func confirming(id string, line int) map[string]any {
	return map[string]any{
		"id": id, "kind": "finding", "role": "convention", "class": "panic-in-library",
		"rule": "no-panic", "severity": "critical", "unit": "u1",
		"anchor": map[string]any{
			"path": "lib.go", "side": "RIGHT", "start_line": line, "line": line,
			"content_hash": "0123456789abcdef",
		},
		"citations": []map[string]any{{"path": "lib.go", "line": line}},
		"summary":   "Load panics where it should return an error.",
		"evidence":  "The call aborts the caller's process.",
	}
}

// §2.6.1.5 through the commands: detection produces three hits, the agent
// confirms one, and the other two never reach findings.ndjson.
//
// The drop is an absence, and that is what makes it hold. `cr rules check`
// reports hits and writes no record, and `cr record` stores what the agent's
// own file carries; nothing between them turns a hit into a record. So a hit
// the agent wrote nothing for has no road to the draft, which §7.1 renders from
// findings.ndjson alone — and the check below reads every stored record for any
// trace of the two lines it was never told about.
func TestTwoOfThreeHitsTheAgentNeverConfirmsNeverReachFindings(t *testing.T) {
	layout := detectedHome(t)

	var checked rulesCheckResult
	require.NoError(t, json.Unmarshal(runRulesCheck(t), &checked))
	require.Len(t, checked.Hits, 3, "the change adds three panics")
	lines := make([]int, 0, len(checked.Hits))
	for _, hit := range checked.Hits {
		assert.Equal(t, "no-panic", hit.RuleID)
		assert.Equal(t, "lib.go", hit.Path)
		lines = append(lines, hit.Line)
	}
	require.Equal(t, []int{4, 5, 6}, lines)
	require.Len(t, checked.Units, 1)
	assert.Len(t, checked.Units[0].Hits, 3, "§4.3.6: every hit is attached to the unit holding it")
	assert.Empty(t, storedFindings(t, layout), "detection writes no record of its own")

	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", confirming("f1", checked.Hits[0].Line)),
		"--repo", fixtureSlug)
	require.NoError(t, err)

	stored := storedFindings(t, layout)
	require.Len(t, stored, 1, "the one confirmed hit is the one record")
	assert.Equal(t, "f1", stored[0].ID)
	for _, dropped := range checked.Hits[1:] {
		for i := range stored {
			held := &stored[i]
			assert.NotEqual(t, dropped.Line, held.Anchor.Line,
				"§2.6.1.5: an unconfirmed hit on line %d is dropped, not anchored", dropped.Line)
			assert.False(t, slices.ContainsFunc(held.Citations, func(c finding.Citation) bool {
				return c.Path == dropped.Path && c.Line == dropped.Line
			}), "§2.6.1.5: an unconfirmed hit on line %d is dropped, not cited", dropped.Line)
		}
	}
}

// §2.6.1.5's last sentence: no detect block buys §6.3's assertion register on
// its own.
//
// The rule declares `kind: finding` and `severity: critical`, which is the most
// a rule file can ask for. What detection reports carries neither — each hit is
// the four fields of a match and nothing a verdict is made of — and running it
// twice leaves findings.ndjson as empty as it found it, so the only way the
// rule's standard reaches a draft is a record the agent chose to write.
func TestAHitCarriesNoVerdictWhateverItsRuleDeclares(t *testing.T) {
	layout := detectedHome(t)

	for range 2 {
		var document map[string]any
		require.NoError(t, json.Unmarshal(runRulesCheck(t), &document))
		assert.ElementsMatch(t, []string{"round", "head", "hits", "units"}, keysOf(document))

		hits, isList := document["hits"].([]any)
		require.True(t, isList)
		require.Len(t, hits, 3)
		for _, hit := range hits {
			fields, isObject := hit.(map[string]any)
			require.True(t, isObject)
			assert.ElementsMatch(t, []string{"rule", "path", "line", "text"}, keysOf(fields),
				"a hit is a match, and carries no kind, severity, or grade")
		}
	}
	assert.Empty(t, storedFindings(t, layout),
		"a rule declaring kind finding still produces no record until the agent writes one")
}

// keysOf lists one JSON object's keys.
func keysOf(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	return keys
}

// `cr rules check` reads the round's units, so a pull request no brief has
// opened a round on is refused with §11.2's code 4 naming `cr brief`.
func TestRulesCheckRefusesAnUnbriefedPullRequest(t *testing.T) {
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())

	err := runCLI(t, "rules", "check", fixturePR, "--repo", fixtureSlug)

	var missing *state.NotBriefedError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, ExitState, exitCodeFor(err))
	assert.Contains(t, err.Error(), "cr brief "+fixturePR)
}
