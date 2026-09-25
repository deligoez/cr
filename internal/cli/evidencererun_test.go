package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// rerunHome is gradedHome's round under a profile whose tests.count_pattern
// reads `Tests:  <n> passed|failed`, with four probes at the round's head: p1,
// a mutation nothing noticed whose tail lists two passing tests before its
// recap; p2, a re-run of p1 that a test noticed; p3, a mutation a test
// noticed; and p4, a re-run of p3 nothing noticed.
func rerunHome(t *testing.T) state.Layout {
	t.Helper()
	layout := gradedHome(t)
	require.NoError(t, layout.EnsureProfile("qa", `{"id":"qa",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["./run"],"globs":["*_test.txt"],`+
		`"count_pattern":"Tests:  ([0-9]+) (?:failed|passed)","failed_pattern":"Tests:  ([0-9]+) failed"}}`))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.ProfileID = "qa"
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	probe := func(id, result, tail, rerunOf string) string {
		line := `{"id":"` + id + `","kind":"mutation","head":"` + meta.Head + `","round":2,` +
			`"input":"--- a/app.go\n+++ b/app.go\n","result":"` + result + `",` +
			`"baseline":"r1","target":"app.go:3","duration_ms":520,"output_tail":` + mustJSON(t, tail)
		if rerunOf != "" {
			line += `,"rerun_of":"` + rerunOf + `"`
		}
		return line + "}\n"
	}
	require.NoError(t, held.Write(state.FileProbes, []byte(
		probe("p1", "no-test-failed", "PASS  Retry backs off\nPASS  Retry gives up\nTests:  12 passed\n", "")+
			probe("p2", "test-failed", "Tests:  1 failed\n", "p1")+
			probe("p3", "test-failed", "Tests:  1 failed\n", "")+
			probe("p4", "no-test-failed", "Tests:  12 passed\n", "p3"))))
	require.NoError(t, held.Unlock())
	return layout
}

// draftOf records the given records and returns the draft `cr draft` wrote.
func draftOf(t *testing.T, layout state.Layout, records ...map[string]any) string {
	t.Helper()
	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", records...), "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	written, err := os.ReadFile(layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileDraft))
	require.NoError(t, err)
	return string(written)
}

// §8.1.7 through `cr draft`: a probed record whose probe nothing failed under
// shows, in place of the output tail, only the lines tests.count_pattern
// matches, and carries the probe's re-run at the round's head with its result.
// Measured on tarfin-labs/backend#6328 with cr 0.13.0: a probed comment
// carried sixty lines of passing tests.
func TestAProbedRecordShowsItsCountedLinesAndItsReruns(t *testing.T) {
	layout := rerunHome(t)
	probed := aGradedRecord("f1")
	probed["probe"], probed["severity"] = "p1", "high"
	drafted := draftOf(t, layout, probed)
	require.Equal(t, finding.GradeProbed, gradesOf(t, layout)["f1"])

	block := blockOf(t, drafted, "f1")
	assert.Contains(t, block, "result: no-test-failed\n"+
		"input:\n```\n--- a/app.go\n+++ b/app.go\n```\n"+
		"output_tail, the lines tests.count_pattern matches:\n```\nTests:  12 passed\n```\n"+
		"rerun: p2, result: test-failed\n"+
		"<!-- cr:/evidence -->")
	assert.False(t, strings.Contains(block, "PASS  Retry"), "the passing tests' own lines are not shown")
}

// §8.1.7 through `cr draft`: a record that asserts nothing still carries, in
// an evidence region of its own beneath its body, each re-run at the round's
// head of the probe it names, with the re-run's result. Measured on
// tarfin-labs/backend#6328 with cr 0.13.0: a question a re-run had refuted by
// experiment reached the draft with nothing saying so. A record naming no
// probe, the control, carries no region.
func TestAnArguedRecordCarriesItsProbesReruns(t *testing.T) {
	layout := rerunHome(t)
	argued := aGradedRecord("f3")
	argued["probe"] = "p3"
	plain := aGradedRecord("f4")
	drafted := draftOf(t, layout, argued, plain)
	require.Equal(t, finding.GradeArgued, gradesOf(t, layout)["f3"])

	assert.Contains(t, blockOf(t, drafted, "f3"),
		"<!-- cr:evidence -->\nrerun: p4, result: no-test-failed\n<!-- cr:/evidence -->")
	assert.NotContains(t, blockOf(t, drafted, "f4"), "<!-- cr:evidence -->")
}
