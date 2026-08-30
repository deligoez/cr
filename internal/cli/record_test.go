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
