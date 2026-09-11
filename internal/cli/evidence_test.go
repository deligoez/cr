package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// gradedDraft records a `probed` record on the fixture's mutation probe p1 and
// a `cited` one on app.go:1, runs `cr draft` over them, and returns the draft
// and the rendered.json it wrote for gradedHome's round 2.
func gradedDraft(t *testing.T) (drafted string, rendered map[string]string) {
	t.Helper()
	layout := gradedHome(t)
	probed := aGradedRecord("f1")
	probed["probe"], probed["severity"] = "p1", "high"
	cited := aGradedRecord("f2")
	cited["citations"] = []map[string]any{{"path": "app.go", "line": 1}}
	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", probed, cited), "--repo", fixtureSlug)
	require.NoError(t, err)
	graded := gradesOf(t, layout)
	require.Equal(t, finding.GradeProbed, graded["f1"])
	require.Equal(t, finding.GradeCited, graded["f2"])

	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	written, err := os.ReadFile(layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileDraft))
	require.NoError(t, err)
	body, err := layout.ReadRound(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileRendered)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &rendered))
	return string(written), rendered
}

// §8.1.7 through the commands: a record `cr record` graded `probed` reaches the
// draft with the probe's kind, target, result, the patch cr executed and the
// runner's output beneath its body, and a record graded `cited` with its
// citation as path:line. rendered.json holds neither region, so §7.1.6's
// byte-exact comparison is still between agent regions alone.
//
// This is the end-to-end half the draft package cannot give: the probe is read
// out of probes.ndjson, where `cr probe run` puts it, and the cap is §2.7's
// default, resolved the way every other setting is.
func TestTheDraftCarriesEachAssertingRecordsEvidence(t *testing.T) {
	drafted, rendered := gradedDraft(t)

	assert.Contains(t, blockOf(t, drafted, "f1"), "<!-- cr:evidence -->\n"+
		"kind: mutation\n"+
		"target: app.go:3\n"+
		"result: no-test-failed\n"+
		"input:\n```\n--- a/app.go\n+++ b/app.go\n```\n"+
		"output_tail:\n```\nTests:  12 passed\n```\n"+
		"<!-- cr:/evidence -->")
	assert.Contains(t, blockOf(t, drafted, "f2"), "<!-- cr:evidence -->\n"+
		"citation: app.go:1\n"+
		"<!-- cr:/evidence -->")

	require.Len(t, rendered, 2)
	for id, entry := range rendered {
		assert.NotContains(t, entry, "<!-- cr:", "%s: rendered.json holds the agent region alone", id)
	}
}

// A `probed` record naming a probe probes.ndjson does not hold was read from
// state cr did not write, and `cr draft` refuses it with §11.2's code 3,
// naming the record and the probe, and writes no draft.
func TestADraftOfAProbedRecordWithoutItsProbeExitsThree(t *testing.T) {
	orphan := aStoredRecord("f1", finding.StateDraft)
	orphan.Grade, orphan.Probe = finding.GradeProbed, "p9"
	layout := draftedHome(t, orphan)

	_, err := runDraft(t, draftPR, "--repo", draftSlug)

	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Contains(t, err.Error(), "f1")
	assert.Contains(t, err.Error(), "p9")
	assert.NoFileExists(t, layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft))
}

// post.max_probe_input_bytes reaches the region through §2.7's layers: set in
// the environment, it cuts the patch and the region says so.
func TestTheProbeInputCapIsReadFromTheConfiguration(t *testing.T) {
	t.Setenv("CR_POST_MAX_PROBE_INPUT_BYTES", "8")
	drafted, _ := gradedDraft(t)

	assert.Contains(t, blockOf(t, drafted, "f1"),
		"input (truncated to 8 of 26 bytes by post.max_probe_input_bytes):\n```\n--- a/ap\n```\n")
}
