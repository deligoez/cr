package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// testAdequacyU2Block is how a refusal names the block `cr review` gives the
// test-adequacy prompt on recordedHome's u2: the round's two units times the
// shipped roles in id order put that prompt eighth, so its hundred ids are
// f701 through f800.
const testAdequacyU2Block = "f701..f800, the block cr review gave the test-adequacy prompt on unit u2,"

// QA D-S05-1, the S05 collision on recordedHome's round: the correctness and
// test-adequacy roles both wrote f1 on u2, outside their blocks. The refusal
// names the test-adequacy line and the first free id of that prompt's own
// block, f701 — not the round-wide f2, which is inside the convention prompt's
// block f1..f100 — through both `cr merge` and `cr record`.
func TestARepeatedIDIsHintedInsideTheRefusedPromptsBlock(t *testing.T) {
	layout := recordedHome(t)
	dir, out := t.TempDir(), mergedOut(t)
	correctness := writeFanOut(t, dir, "correctness", aRoleRecord("f1", "correctness", "unchecked-error", "u2"))
	testAdequacy := writeFanOut(t, dir, "test-adequacy", aRoleRecord("f1", "test-adequacy", "missing-test", "u2"))
	problem := func(where string) string {
		return `"f1" repeats the id of ` + where + "; §6.1 makes a record id stable for the life of the pull " +
			"request, so give each record an id of its own; the next free id in " + testAdequacyU2Block + " is f701"
	}

	_, err := runMergeCLI(t, out, correctness, testAdequacy)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, finding.RejectedRecordError{
		File: testAdequacy, Line: 1, Field: "id", Problem: problem(correctness + " line 1"),
	}, *rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assertMergeWroteNothing(t, layout, out)

	direct := writeRecordFile(t, "merged.ndjson",
		aRoleRecord("f1", "correctness", "unchecked-error", "u2"), aRoleRecord("f1", "test-adequacy", "missing-test", "u2"))
	_, err = runRecord(t, recordPR, direct, "--repo", recordSlug)
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, finding.RejectedRecordError{File: direct, Line: 2, Field: "id", Problem: problem("line 1")}, *rejected)
}

// The S05 follow-up: the test-adequacy role followed the old round-wide hint
// and wrote f2, while the convention role wrote f1 and f2 in its own block on
// u1. The refusal blames the file that left its block, though it was read
// first, names the convention line it collides with, and hints f701; following
// the hints — f301 for correctness, f701 for test-adequacy — lets `cr merge`
// accept all three files.
func TestARepeatedIDBlamesTheFileThatLeftItsBlockAndTheHintMerges(t *testing.T) {
	recordedHome(t)
	dir := t.TempDir()
	testAdequacy := writeFanOut(t, dir, "test-adequacy", aRoleRecord("f2", "test-adequacy", "missing-test", "u2"))
	convention := writeFanOut(t, dir, "convention",
		aRoleRecord("f1", "convention", "naming", "u1"), aRoleRecord("f2", "convention", "long-function", "u1"))

	_, err := runMergeCLI(t, mergedOut(t), testAdequacy, convention)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, finding.RejectedRecordError{
		File: testAdequacy, Line: 1, Field: "id",
		Problem: `"f2" repeats the id of ` + convention + " line 2; §6.1 makes a record id stable for the life of " +
			"the pull request, so give each record an id of its own; the next free id in " + testAdequacyU2Block + " is f701",
	}, *rejected)

	followed := t.TempDir()
	out := mergedOut(t)
	printed, err := runMergeCLI(t, out,
		writeFanOut(t, followed, "correctness", aRoleRecord("f301", "correctness", "unchecked-error", "u2")),
		writeFanOut(t, followed, "test-adequacy", aRoleRecord("f701", "test-adequacy", "missing-test", "u2")),
		writeFanOut(t, followed, "convention",
			aRoleRecord("f1", "convention", "naming", "u1"), aRoleRecord("f2", "convention", "long-function", "u1")))
	require.NoError(t, err, "the hinted ids merge")
	assert.Equal(t, 4, decodeMergeResult(t, printed).Merged)
	assert.FileExists(t, out)
}

// A block whose last id a stored record holds has no free id left, and the
// refusal says so rather than naming an id of another prompt's block.
func TestAFullBlockIsNamedFull(t *testing.T) {
	recordedHome(t)
	_, err := runRecord(t, recordPR,
		writeRecordFile(t, "earlier.ndjson", aRoleRecord("f800", "test-adequacy", "missing-test", "u2")),
		"--repo", recordSlug)
	require.NoError(t, err)
	file := writeFanOut(t, t.TempDir(), "test-adequacy", aRoleRecord("f800", "test-adequacy", "missing-test", "u2"))

	_, err = runMergeCLI(t, mergedOut(t), file)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, finding.RejectedRecordError{
		File: file, Line: 1, Field: "id",
		Problem: `"f800" is already held by the record stored for this pull request in round 2 at head ` + recordHead +
			"; §6.1 makes a record id stable for the life of the pull request, so give this record an id no stored " +
			"record holds; " + testAdequacyU2Block + " holds no id that is neither stored nor carried by the input, " +
			"so that block is full",
	}, *rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
}
