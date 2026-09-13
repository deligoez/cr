package cli

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// assertMergeWroteNothing holds a refused merge to writing neither of the two
// files a merge produces: the output `-o` named and the round's summary.json.
func assertMergeWroteNothing(t *testing.T, layout state.Layout, out string) {
	t.Helper()
	_, err := os.Stat(out)
	assert.True(t, os.IsNotExist(err), "a refused merge writes no output file")
	_, err = os.Stat(layout.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FileSummary))
	assert.True(t, os.IsNotExist(err), "a refused merge writes no summary.json")
}

// §6.1's id row across two per-role files: both carry f1, once as findings of
// different classes and once as duplicates of one anchored line and class,
// which §6.4.3 would otherwise mark with a `duplicate_of` naming f1 itself.
// Either way the merge exits 1 naming the second file, the line f1 sits on in
// it, the first file and line, and the next free id — f4, past the second
// file's own f3 — and writes nothing.
func TestMergeRefusesAnIDTwoPerRoleFilesBothCarry(t *testing.T) {
	for name, class := range map[string]string{
		"different findings": "missing-test",
		"duplicates":         "unchecked-error",
	} {
		t.Run(name, func(t *testing.T) {
			layout := recordedHome(t)
			dir, out := t.TempDir(), mergedOut(t)
			first := writeFanOut(t, dir, "correctness",
				aRoleRecord("f1", "correctness", "unchecked-error", "u1"))
			second := writeFanOut(t, dir, "convention",
				aRoleRecord("f3", "convention", "naming", "u1"),
				aRoleRecord("f1", "convention", class, "u1"))

			_, err := runMergeCLI(t, out, first, second)

			var rejected *finding.RejectedRecordError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, second, rejected.File, "the refusal names the file the agent edits")
			assert.Equal(t, 2, rejected.Line, "the line within that file, not the record's place in the merge")
			assert.Equal(t, "id", rejected.Field)
			assert.Regexp(t, "^"+regexp.QuoteMeta(`"f1" repeats the id of `+first+` line 1;`), rejected.Problem)
			assert.Equal(t, "f4", nextFreeIDIn(t, rejected.Problem))
			assertMergeWroteNothing(t, layout, out)
		})
	}
}

// §6.1's id row against the store: a per-role file carrying f1 while a stored
// record of the pull request holds f1 is refused the way `cr record` refuses
// it, naming the line, the stored record, and the next free id over the store
// and the file — f7, past the stored f6 — and the merge writes nothing.
func TestMergeRefusesAnIDAStoredRecordHolds(t *testing.T) {
	layout := recordedHome(t)
	_, err := runRecord(t, recordPR,
		writeRecordFile(t, "earlier.ndjson", aRecord("f1", "u1"), aRecord("f6", "u2")),
		"--repo", recordSlug)
	require.NoError(t, err)
	summary := layout.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FileSummary)
	before, beforeErr := os.ReadFile(summary)
	dir, out := t.TempDir(), mergedOut(t)
	file := writeFanOut(t, dir, "correctness",
		aRoleRecord("f2", "correctness", "missing-test", "u1"),
		aRoleRecord("f1", "correctness", "unchecked-error", "u1"))

	_, err = runMergeCLI(t, out, file)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, file, rejected.File)
	assert.Equal(t, 2, rejected.Line)
	assert.Equal(t, "id", rejected.Field)
	assert.Regexp(t, `^"f1" is already held by the record stored for this pull request in round `, rejected.Problem)
	assert.Equal(t, "f7", nextFreeIDIn(t, rejected.Problem))
	_, statErr := os.Stat(out)
	assert.True(t, os.IsNotExist(statErr), "a refused merge writes no output file")
	after, afterErr := os.ReadFile(summary)
	assert.Equal(t, os.IsNotExist(beforeErr), os.IsNotExist(afterErr), "a refused merge creates no summary.json")
	assert.Equal(t, string(before), string(after), "a refused merge leaves summary.json as it was")
}
