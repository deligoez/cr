package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// asMergeOutput records file as the output `cr merge` last wrote for the
// fixture round, by putting its digest where `cr merge` puts it, and returns
// the path. It is for a test whose subject is what `cr record` does with a
// merged file's `duplicate_of`, and whose file is one a real merge could not be
// made to write — a representative missing from the file, say.
func asMergeOutput(t *testing.T, layout state.Layout, file string) string {
	t.Helper()
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	digest, err := mergedDigest(body)
	require.NoError(t, err)
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, state.UpdateRoundSection(
		held, recordRound, state.FileSummary, summaryMergedHash, digest))
	require.NoError(t, held.Unlock())
	return file
}

// recordStore is every record findings.ndjson holds for the fixture pull
// request of recordedHome.
func recordStore(t *testing.T, layout state.Layout) []finding.Finding {
	t.Helper()
	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	return stored
}

// §6.1's rows that name every value they may take, through `cr record`: a
// value outside the row is refused with exit code 1 naming the file, the line
// and the field, and nothing is stored.
//
// The faulty record is the second line, after a record that is valid, so the
// line named is the record's own and the refusal takes the whole file.
func TestRecordRefusesAValueSection61DoesNotName(t *testing.T) {
	for name, tc := range map[string]struct {
		field, value, problem string
	}{
		"an id not of the form f<n>": {
			field: "id", value: "x9",
			problem: `reads "x9", and §6.1 spells a record id f<n>, numbered from one`,
		},
		"a kind outside the register": {
			field: "kind", value: "assertion",
			problem: `reads "assertion", and §6.1 allows only "finding", "question"`,
		},
		"a severity outside the four": {
			field: "severity", value: "urgent",
			problem: `reads "urgent", and §6.1 allows only "critical", "high", "medium", "low"`,
		},
		"a suggestion_origin outside agent and rule": {
			field: "suggestion_origin", value: "human",
			problem: `reads "human", and §6.1 allows only "agent", "rule"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			layout := recordedHome(t)
			faulty := aRecord("f2", "u2")
			faulty[tc.field] = tc.value
			if tc.field == "suggestion_origin" {
				faulty["suggestion"] = "\tif err != nil {\n\t\treturn err\n\t}"
			}
			file := writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"), faulty)

			_, err := runRecord(t, recordPR, file, "--repo", recordSlug)

			var rejected *finding.RejectedRecordError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.1.3's refusals exit 1")
			assert.Equal(t, &finding.RejectedRecordError{
				File: file, Line: 2, Field: tc.field, Problem: tc.problem,
			}, rejected)
			assert.Empty(t, recordStore(t, layout), "a refused line takes the whole file with it")
		})
	}
}

// §6.1.4 through `cr record`: `duplicate_of` is read only on the file `cr merge`
// wrote for the round, and refused with exit code 1 naming the file, the line
// and the field on every other one — a file the agent wrote before any merge
// ran, a role's own §4.6.2 file handed straight in, the merge's output with its
// lines reordered, and the agent's file again once a merge has recorded a
// digest that is not its own. Each refusal stores nothing, and the merge's own
// output is then accepted with the duplicate it marked retired.
func TestRecordReadsDuplicateOfOnlyOnTheFileCRMergeWrote(t *testing.T) {
	layout := recordedHome(t)
	dir := t.TempDir()

	claimed := aRecord("f2", "u2")
	claimed["duplicate_of"] = "f1"
	claimed["class"] = "unchecked-error"
	agents := writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"), claimed)
	roles := writeFanOut(t, dir, "correctness", aRecord("f1", "u1"), claimed)

	refused := func(t *testing.T, file string, line int) {
		t.Helper()
		_, err := runRecord(t, recordPR, file, "--repo", recordSlug)
		var reserved *state.ReservedFieldError
		require.ErrorAs(t, err, &reserved)
		assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.1.4's refusal exits 1")
		assert.Equal(t, &state.ReservedFieldError{File: file, Line: line, Field: "duplicate_of"}, reserved)
		assert.Empty(t, recordStore(t, layout))
	}

	t.Run("a file the agent wrote, before any merge ran", func(t *testing.T) { refused(t, agents, 2) })
	t.Run("a role's file handed straight to cr record", func(t *testing.T) { refused(t, roles, 2) })

	out := mergedOut(t)
	inputs := t.TempDir()
	correctness := writeFanOut(t, inputs, "correctness",
		aRoleRecord("f1", "correctness", "unchecked-error", "u1"))
	convention := writeFanOut(t, inputs, "convention",
		aRoleRecord("f2", "convention", "unchecked-error", "u1"))
	_, err := runMergeCLI(t, out, correctness, convention)
	require.NoError(t, err)
	body, err := os.ReadFile(out)
	require.NoError(t, err)
	// The same records in the other order: every line is the merge's, and
	// the file is not.
	lines := bytes.Split(bytes.TrimRight(body, "\n"), []byte("\n"))
	require.Len(t, lines, 2)
	reordered := append(append(bytes.Clone(lines[1]), '\n'), append(bytes.Clone(lines[0]), '\n')...)
	edited := filepath.Join(t.TempDir(), "merged.ndjson")
	require.NoError(t, os.WriteFile(edited, reordered, 0o600))
	t.Run("the merge's output reordered", func(t *testing.T) { refused(t, edited, duplicateLine(t, reordered)) })
	t.Run("the agent's file once a merge recorded another digest", func(t *testing.T) { refused(t, agents, 2) })

	_, err = runRecord(t, recordPR, out, "--repo", recordSlug)
	require.NoError(t, err, "§6.5.1: the file cr merge wrote carries duplicate_of into cr record")
	states := map[string]finding.State{}
	for _, record := range recordStore(t, layout) {
		states[record.ID] = record.State
	}
	// §6.4.2 ranks equal grades and severities by corpus order, which puts
	// convention's f2 ahead of correctness's f1.
	assert.Equal(t, map[string]finding.State{"f1": finding.StateDuplicate, "f2": finding.StateDraft}, states)
}

// duplicateLine is the one-based line of body holding the one record that
// carries `duplicate_of`.
func duplicateLine(t *testing.T, body []byte) int {
	t.Helper()
	found := 0
	for at, line := range bytes.Split(bytes.TrimRight(body, "\n"), []byte("\n")) {
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(line, &fields))
		if _, marked := fields["duplicate_of"]; marked {
			require.Zero(t, found, "the fixture's merge marks exactly one duplicate")
			found = at + 1
		}
	}
	require.NotZero(t, found, "the fixture's merge marks a duplicate")
	return found
}
