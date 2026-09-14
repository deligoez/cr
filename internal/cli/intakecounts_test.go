package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// waivingHome is gradedHome with one repository-wide waiver covering a
// long-function finding on the fixture unit's line, and the record it drops.
func waivingHome(t *testing.T) (layout state.Layout, waived map[string]any) {
	t.Helper()
	layout = gradedHome(t)
	waiveLongFunction(t, layout)
	return layout, longFunction("f3")
}

// longFunction is aGradedRecord in the class waiveLongFunction's waiver covers.
func longFunction(id string) map[string]any {
	record := aGradedRecord(id)
	record["class"] = "long-function"
	return record
}

// waiveLongFunction writes the repository-wide waiver waivingHome holds.
func waiveLongFunction(t *testing.T, layout state.Layout) {
	t.Helper()
	// §7.4.1's key hash of aGradedRecord's anchor: line 3 of app.go at the
	// fixture's head, below its two lines of context and with none after it.
	stamped, err := finding.ContextKeyHash(
		[]string{"package app", ""}, []string{"func Retry() { backoff() }"}, []string{})
	require.NoError(t, err)
	waiver := finding.Waiver{
		WaiverKey: finding.WaiverKey{
			Path: "app.go", Side: "RIGHT", Class: "long-function", ContentHash: stamped,
		},
		Disposition: finding.DispositionWrong,
	}
	_, err = finding.Waive(layout, fixtureOwner, fixtureProject, &waiver,
		finding.WaiverProvenance{Round: 1, PR: fixturePRNumber, Head: "0a1b2c3"})
	require.NoError(t, err)
}

// intakeOf reads the round summary's raised, waived and already_posted
// sections, whole.
func intakeOf(t *testing.T, layout state.Layout) (raised int, waived finding.Drops, posted finding.PostedDrops) {
	t.Helper()
	body, err := layout.ReadRound(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileSummary)
	require.NoError(t, err)
	var document struct {
		Raised        *int                 `json:"raised"`
		Waived        *finding.Drops       `json:"waived"`
		AlreadyPosted *finding.PostedDrops `json:"already_posted"`
	}
	require.NoError(t, json.Unmarshal(body, &document))
	require.NotNil(t, document.Raised, "summary.json holds raised")
	require.NotNil(t, document.Waived, "summary.json holds waived")
	require.NotNil(t, document.AlreadyPosted, "summary.json holds already_posted")
	return *document.Raised, *document.Waived, *document.AlreadyPosted
}

// QA D-S05-3 through the commands: a file handed straight to `cr record`, with
// no `cr merge` run for the round, is counted in the round summary as a merged
// one is — raised is every record it held, the stored ones and the one an
// active waiver dropped, and waived is the drop `cr record` itself reported.
//
// Before, only `cr merge` wrote those three sections: such a round's summary had
// no raised at all, and waived read zero beside a record run that had dropped
// one.
func TestAFileRecordedWithoutAMergeIsCountedInTheRoundSummary(t *testing.T) {
	layout, waived := waivingHome(t)
	other := aGradedRecord("f2")
	other["class"] = "missing-test"
	file := writeRecordFile(t, "review-correctness.ndjson", aGradedRecord("f1"), other, waived)

	printed, err := runRecord(t, fixturePR, file, "--repo", fixtureSlug)
	require.NoError(t, err)

	var reported recordResult
	require.NoError(t, json.Unmarshal([]byte(printed), &reported))
	stored := storedFindings(t, layout)
	require.Len(t, stored, 2, "the waived record is not stored")
	require.Equal(t, 1, reported.Waived.Dropped)

	raised, summaryWaived, posted := intakeOf(t, layout)
	assert.Equal(t, len(stored)+reported.Waived.Dropped, raised,
		"raised is the stored records and the one the waiver dropped")
	assert.Equal(t, reported.Waived, summaryWaived, "waived is what cr record dropped")
	assert.Equal(t, waivedID(t, layout), summaryWaived.Waivers[0])
	assert.Equal(t, finding.PostedDrops{Dropped: 0, Posted: []string{}}, posted)
}

// Recording a file is counted once, however often it is recorded. A file whose
// every record a waiver drops stores nothing, so nothing refuses a second run of
// it, and a count added to on each run would climb.
func TestRecordingOneFileAgainLeavesTheIntakeCountsUnchanged(t *testing.T) {
	layout, waived := waivingHome(t)
	file := writeRecordFile(t, "review-correctness.ndjson", waived)

	for range 2 {
		_, err := runRecord(t, fixturePR, file, "--repo", fixtureSlug)
		require.NoError(t, err)
	}

	raised, summaryWaived, _ := intakeOf(t, layout)
	assert.Empty(t, storedFindings(t, layout))
	assert.Equal(t, 1, raised)
	assert.Equal(t, 1, summaryWaived.Dropped)
}

// A merge run after a file was recorded without one adds its own records to the
// counts rather than replacing them.
func TestAMergeAfterADirectRecordAddsToTheIntakeCounts(t *testing.T) {
	layout, waived := waivingHome(t)
	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "direct.ndjson", aGradedRecord("f1"), waived), "--repo", fixtureSlug)
	require.NoError(t, err)

	dir := t.TempDir()
	later := aGradedRecord("f4")
	later["class"] = "missing-test"
	file := writeFanOut(t, dir, "correctness", later)
	_, err = runCLIPrinting(t, "merge", file, "-o", filepath.Join(dir, "merged.ndjson"),
		"--repo", fixtureSlug, "--pr", fixturePR)
	require.NoError(t, err)

	raised, summaryWaived, _ := intakeOf(t, layout)
	assert.Equal(t, 3, raised, "f1 stored, f3 waived by cr record, f4 in the merge's output")
	assert.Equal(t, 1, summaryWaived.Dropped, "the drop cr record applied stays counted")
	assert.Equal(t, []string{waivedID(t, layout)}, summaryWaived.Waivers)
}

// A drop `cr record` applies to the merge's own output — a waiver written after
// the merge ran — is counted in waived and not a second time in raised, since
// the merge's share already holds the record. A later merge of the same role
// file applies that drop itself, and the drop is then counted once, as the
// merge's.
func TestADropCrRecordAppliesToAMergeOutputIsCountedOnce(t *testing.T) {
	layout := gradedHome(t)
	dir := t.TempDir()
	file := writeFanOut(t, dir, "correctness", longFunction("f1"))
	out := filepath.Join(dir, "merged.ndjson")
	merge := func() {
		t.Helper()
		_, err := runCLIPrinting(t, "merge", file, "-o", out, "--repo", fixtureSlug, "--pr", fixturePR)
		require.NoError(t, err)
	}
	merge()
	waiveLongFunction(t, layout)

	_, err := runRecord(t, fixturePR, out, "--repo", fixtureSlug)
	require.NoError(t, err)
	raised, waived, _ := intakeOf(t, layout)
	assert.Empty(t, storedFindings(t, layout))
	assert.Equal(t, 1, raised, "f1 is in the merge's output once, and dropped by cr record")
	assert.Equal(t, 1, waived.Dropped)

	merge()
	raised, waived, _ = intakeOf(t, layout)
	assert.Equal(t, 1, raised, "the second merge dropped f1 itself")
	assert.Equal(t, 1, waived.Dropped, "cr record's drop over the first merge's output is not counted beside it")
}
