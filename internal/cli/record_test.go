package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The pull request `cr record` is exercised against, and the round it stands
// in. The round is past its first because §3.4.6 scopes a unit id to its round:
// a fixture in round 1 could not tell a unit set drawn from the round apart
// from one drawn from the whole file.
const (
	recordOwner = "acme"
	recordRepo  = "api"
	recordSlug  = recordOwner + "/" + recordRepo
	recordPR    = "13"
	recordPRNum = 13
	recordRound = 2
	recordHead  = "9a8b7c6d5e4f30211220314f5e6d7c8b9a807162"
	// staleUnit is a unit of the round before, which this round did not
	// form and no record of it may name.
	staleUnit = "u3"
)

// recordedHome puts a state root behind CR_HOME holding one pull request in
// round recordRound, with two units of that round and one of the round before.
//
// The units are written as bytes rather than through a record type because
// §3.4.6's unit record does not exist yet, and the id and the round are the
// whole of what `cr record` reads out of the file.
func recordedHome(t *testing.T) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(recordOwner, recordRepo, recordPRNum))

	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: recordOwner, Repo: recordRepo, PR: recordPRNum,
		Round: recordRound, Head: recordHead,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(strings.Join([]string{
		unitLine("u1", recordHead, recordRound),
		unitLine("u2", recordHead, recordRound),
		unitLine(staleUnit, "1f2e3d4c5b6a79880997a6b5c4d3e2f11f2e3d4c", recordRound-1),
	}, "\n")+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// unitLine is one line of units.ndjson carrying the two fields this command
// reads: §3.4.6's id, and §2.3.3's round.
func unitLine(id, head string, round int) string {
	return fmt.Sprintf(`{"id":%q,"head":%q,"round":%d}`, id, head, round)
}

// aRecord is the §6.1 fields an agent supplies, complete and valid. A test
// makes exactly one thing wrong with a copy of it, so what a rejection proves
// is that one fault and not some second thing the fixture never had.
func aRecord(id, unit string) map[string]any {
	return map[string]any{
		"id":       id,
		"kind":     "finding",
		"role":     "correctness",
		"class":    "unchecked-error",
		"severity": "high",
		"unit":     unit,
		"anchor": map[string]any{
			"path":         "internal/api/handler.go",
			"side":         "RIGHT",
			"start_line":   42,
			"line":         44,
			"content_hash": "0123456789abcdef",
		},
		"summary":  "The error Decode returns is dropped.",
		"evidence": "The call's second result is assigned to the blank identifier.",
	}
}

// writeRecordFile writes records as the NDJSON file an agent hands `cr record`,
// and returns its path. It sits in a directory of its own, outside the state
// tree: the file is the agent's own output, and cr only ever reads it.
func writeRecordFile(t *testing.T, name string, records ...map[string]any) string {
	t.Helper()
	var body bytes.Buffer
	for _, record := range records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		body.Write(line)
		body.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, body.Bytes(), 0o600))
	return path
}

// runRecord runs `cr record` with args against whatever CR_HOME points at, and
// returns what it printed and what it refused.
func runRecord(t *testing.T, args ...string) (printed string, err error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"record"}, args...))
	// Executed first and read after, for the reason runAnswer gives.
	err = cmd.Execute()
	return out.String(), err
}

// §11 and §9.1 through the command: every record of an accepted file reaches
// findings.ndjson, in `draft`, stamped with the round's head.
//
// The three fields asserted on each stored record are the three the agent could
// not have written. §6.1.4 reserves `state` and §2.3.3 reserves head and round,
// and all three are refused on the wire, so a record carrying them on disk got
// them from cr — which is what makes reading them back a test of this command
// rather than of the fixture.
func TestRecordStoresEveryRecordOfAnAcceptedFile(t *testing.T) {
	layout := recordedHome(t)
	file := writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"), aRecord("f2", "u2"))

	printed, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings,
	)
	require.NoError(t, err)
	require.Len(t, stored, 2, "§11: cr record stores the round's merged findings")
	assert.Equal(t, []string{"f1", "f2"}, []string{stored[0].ID, stored[1].ID},
		"the file's order is the file's, and cr reorders nothing")

	for _, held := range stored {
		assert.Equal(t, finding.StateDraft, held.State,
			"§9.1: cr record is the actor that brings a new record into draft")
		assert.Equal(t, recordHead, held.Head, "§2.3.3: cr writes head on every write")
		assert.Equal(t, recordRound, held.Round, "§2.3.3: and round with it")
	}

	assert.Contains(t, printed, `"state": "draft"`,
		"the stored records are handed back, carrying what cr wrote onto them")
}

// §6.1.3 through the command: a refused line takes the whole file with it, and
// the refusal names the file, the one-based line, and the field.
//
// The fault is put on the third line of four on purpose. Two well-formed
// records precede it, so a command that wrote as it validated would already
// have stored them, and one follows it, so a command that carried on would have
// stored that too. Neither may reach the file: the agent is about to correct
// the input and hand the whole of it in again, and a half-stored round would
// duplicate every line above the fault.
//
// A round is recorded first, so "nothing is written" is asserted against a file
// that already holds something. findings.ndjson is appended to (§2.3), and an
// append that failed by publishing a truncated file would pass a check that
// only counted the new records.
func TestARefusedLineLeavesFindingsExactlyAsItWas(t *testing.T) {
	layout := recordedHome(t)
	accepted := writeRecordFile(t, "first.ndjson", aRecord("f1", "u1"))
	_, err := runRecord(t, recordPR, accepted, "--repo", recordSlug)
	require.NoError(t, err)

	stored := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
	before, err := os.ReadFile(stored)
	require.NoError(t, err)
	require.NotEmpty(t, before, "the fixture round has to have reached the file")

	faulty := aRecord("f4", "u1")
	delete(faulty, "evidence")
	refused := writeRecordFile(t, "second.ndjson",
		aRecord("f2", "u1"), aRecord("f3", "u2"), faulty, aRecord("f5", "u2"))

	_, err = runRecord(t, recordPR, refused, "--repo", recordSlug)
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.1.3 rejects with exit code 1")
	assert.Contains(t, err.Error(), refused, "the refusal names the file")
	assert.Contains(t, err.Error(), "line 3", "and the one-based line the record sits on")
	assert.Contains(t, err.Error(), "evidence", "and the field at fault")

	after, err := os.ReadFile(stored)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"§6.1.3 refuses the file, so neither the lines above the fault nor the line below it are stored")
}

// §6.1.3's three refusals reach the command, each naming the field at fault.
//
// The point is not that three faults are caught. It is that the command adds no
// reading of §6.1 of its own: finding.Decode already holds every line to the
// section, and all three refusals arrive here as the RejectedRecordError it
// raises, carrying the file, the line and the field it named. A command that
// validated for itself would answer some of these and not others, or answer
// them in a shape internal/cli does not map onto §11.2's code 1.
//
// The unknown unit is the round before's rather than an invented id, so the
// refusal proves the scoping as well as the membership: §3.4.6 makes a unit id
// round-scoped, and a set drawn from the whole of units.ndjson would accept it.
func TestRecordRefusesTheThreeFaultsSection613Names(t *testing.T) {
	recordedHome(t)

	refuse := func(t *testing.T, name string, record map[string]any) string {
		t.Helper()
		file := writeRecordFile(t, name, record)
		_, err := runRecord(t, recordPR, file, "--repo", recordSlug)
		require.Error(t, err)
		assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.1.3 rejects with exit code 1")

		var rejected *finding.RejectedRecordError
		require.ErrorAs(t, err, &rejected, "the command raises no rejection of its own")
		assert.Equal(t, file, rejected.File)
		assert.Equal(t, 1, rejected.Line)
		return rejected.Field
	}

	missing := aRecord("f1", "u1")
	delete(missing, "summary")
	assert.Equal(t, "summary", refuse(t, "missing.ndjson", missing),
		"a record missing a field §6.1 requires is refused by that field's name")

	assert.Equal(t, "unit", refuse(t, "stale-unit.ndjson", aRecord("f1", staleUnit)),
		"§3.4.6 scopes a unit id to its round, so the round before formed no unit of this one")

	assert.Equal(t, "role", refuse(t, finding.FanOutFile("test"), aRecord("f1", "u1")),
		"§6.1.3 binds a record's role to the role whose §4.6.2 output file it arrived in")
}
