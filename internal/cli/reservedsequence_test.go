package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// quotingTheMarker is a sentence a role writes about cr's own draft grammar: it
// quotes §7.1.1's record marker, so it carries the reserved sequence the way the
// measured record did — a convention role reviewing draft/marker.go.
const quotingTheMarker = "Every block opens with `<!-- cr:record id=\"f1\" -->`, which this helper does not write."

// M-1.1: `cr record` refuses a record whose `summary` or `evidence` carries
// §8.1.3's reserved sequence, naming the file, the line, the record id and the
// field, and stores nothing.
//
// §8.1.2 renders a block's first body out of those two fields verbatim and
// §8.1.3 refuses a body holding the sequence, so a record stored with it can
// never be drafted: `cr draft` refuses it, `cr triage` needs the draft that
// refusal prevents, and no command rewrites a stored record's prose. The door
// the record enters by is the only place the role can still be told.
//
// The offending record sits on the second line of three, so the refusal names
// its own line rather than the first record of the file, and the other two are
// valid — which is what makes "nothing is stored" a statement about the whole
// file.
//
// Measured against the unfixed tree on 2026-09-18, through `go test -overlay`
// over HEAD's record.go, merge.go and exit.go: both cases were stored without
// complaint ("an error is expected but got nil"), as was the merge below, while
// the draft-time refusal below still read the old hint.
func TestRecordRefusesAReservedSequenceInSummaryOrEvidence(t *testing.T) {
	for _, tc := range []struct {
		field string
		text  string
	}{
		{field: "summary", text: quotingTheMarker},
		{field: "evidence", text: "The body would be read as a region: " + render.Reserved},
	} {
		t.Run(tc.field, func(t *testing.T) {
			layout := recordedHome(t)
			quoting := aRecord("f2", "u2")
			quoting[tc.field] = tc.text
			file := writeRecordFile(t, "merged.ndjson",
				aRecord("f1", "u1"), quoting, aRecord("f3", "u2"))

			_, err := runRecord(t, recordPR, file, "--repo", recordSlug)

			require.Error(t, err)
			assert.Equal(t, ExitValidation, exitCodeFor(err), "§8.1.3's refusals exit with code 1")
			var refused *ReservedSequenceError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, file, refused.File, "the refusal names the file")
			assert.Equal(t, 2, refused.Line, "and the line the record sits on")
			assert.Equal(t, tc.field, refused.Field, "and the field")
			assert.Equal(t, "f2", refused.Record, "and the record id")
			assert.Contains(t, err.Error(), render.Reserved, "and the sequence it may not carry")
			assert.Contains(t, hintFor(err), "rewrite that record's text",
				"§12.4's step is the role's to take on its own sentence")

			assert.Equal(t, []string{}, storedRound(t, layout),
				"the whole file is refused, the two valid records with it")
		})
	}
}

// The negative direction: the same file without the sequence is stored. Without
// it the refusal above would pass on a build that refused every record.
func TestRecordStoresTheSameFileWithoutTheReservedSequence(t *testing.T) {
	layout := recordedHome(t)
	quoting := aRecord("f2", "u2")
	quoting["evidence"] = strings.ReplaceAll(quotingTheMarker, render.Reserved, "the record marker")
	file := writeRecordFile(t, "merged.ndjson",
		aRecord("f1", "u1"), quoting, aRecord("f3", "u2"))

	_, err := runRecord(t, recordPR, file, "--repo", recordSlug)

	require.NoError(t, err)
	assert.Equal(t, []string{"f1/draft", "f2/draft", "f3/draft"}, storedRound(t, layout),
		"a role that names the marker without quoting it is recorded like any other")
}

// `cr merge` refuses it at the role's own file, before any merged output exists
// for `cr record` to be handed.
//
// The line and the file both matter here: a refusal that named the merged
// output would name a line the role never wrote, and the role is who has to
// rewrite the sentence.
func TestMergeRefusesAReservedSequenceInARolesOutputFile(t *testing.T) {
	recordedHome(t)
	dir := t.TempDir()
	out := mergedOut(t)
	correctness := writeFanOut(t, dir, "correctness",
		aRoleRecord("f1", "correctness", "unchecked-error", "u1"))
	quoting := aRoleRecord("f2", "convention", "missing-test", "u2")
	quoting["evidence"] = quotingTheMarker
	convention := writeFanOut(t, dir, "convention",
		aRoleRecord("f3", "convention", "unchecked-error", "u1"), quoting)

	_, err := runMergeCLI(t, out, correctness, convention)

	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§8.1.3's refusals exit with code 1")
	var refused *ReservedSequenceError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, convention, refused.File, "the role's own file, not the merged output")
	assert.Equal(t, 2, refused.Line)
	assert.Equal(t, "evidence", refused.Field)
	assert.Equal(t, "f2", refused.Record)
	assert.NoFileExists(t, out, "a refused merge writes no output for `cr record` to read")
}

// A record stored before the record-time check still refuses at draft time, and
// its hint now names a step that exists.
//
// This is the measured defect (field feedback M-1.1) from the other side: the
// record is in findings.ndjson, `cr draft` refuses to render it, and the old
// hint said to edit the body in a draft this very refusal prevents from being
// written. The fix at the door cannot reach a record already stored, so the
// refusal stays and the step is the one repair left.
func TestADraftTimeReservedSequenceNamesAStepThatExists(t *testing.T) {
	stored := aStoredRecord("f1", finding.StateDraft)
	stored.Evidence = quotingTheMarker
	layout := draftedHome(t, stored)

	_, err := runDraft(t, draftPR, "--repo", draftSlug)

	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§8.1.3 fixes the code at 1")
	var body *render.BodyError
	require.ErrorAs(t, err, &body)
	assert.Equal(t, "f1", body.Record)

	hint := hintFor(err)
	assert.Contains(t, hint, "findings.ndjson",
		"the step names the file the record is repaired in, since no command edits a stored record")
	assert.Contains(t, hint, "stored before",
		"and says why the draft the old step named does not exist")

	assert.NoFileExists(t,
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft),
		"the refusal is what makes the old step impossible: there is no draft to edit")
	_, err = state.ReadRecords[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err, "and the line to repair is readable where the step says it is")
	assert.FileExists(t, filepath.Join(layout.PRDir(draftOwner, draftRepo, draftPRNum), state.FileFindings))
}
