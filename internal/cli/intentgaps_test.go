package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/state"
)

// storedGapFields is intent-gaps.ndjson as the JSON keys each line actually
// carries, rather than as decoded structs.
//
// The keys are read off the wire because §4.1.3 names the fields an entry
// carries, and a decoded Gap reports a field the file omitted and a field it
// wrote empty as the same zero value — which is the difference between a claim
// nobody has judged and an encoder that dropped the key.
func storedGapFields(t *testing.T, l state.Layout) []map[string]json.RawMessage {
	t.Helper()
	body, err := l.ReadPR(mapOwner, mapRepo, mapPR, state.FileIntentGaps)
	require.NoError(t, err)
	lines := make([]map[string]json.RawMessage, 0)
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(line, &fields))
		lines = append(lines, fields)
	}
	return lines
}

// `cr map record` writes §4.1.3's unimplemented-claim entry for every claim of
// the round the mapping maps to no unit, carrying all four fields.
//
// head and round are asserted against the round's own values rather than
// against anything the agent wrote, because §2.3.3 gives the pair one author:
// the mapping file handed in below carries neither, and state.ReplaceStamped is
// what puts them on the entry. An entry stamped by the agent would let a claim
// gap be filed against a head it was never derived from, and §9.3.5 scopes
// every later reader by exactly that pair.
//
// `set_aside_note` is asserted as a key that is present and empty. §4.1.8 fills
// it and §10.2.3 reads it, and a reader that had to tell a claim nobody judged
// from one somebody set aside by the key being absent would be reading the
// encoder rather than the decision.
//
// The second recording is what shows the file is derived rather than
// accumulated: c2 becomes mapped, and its entry has to go.
func TestMapRecordWritesAnEntryForEveryClaimMappedToNoUnit(t *testing.T) {
	layout := briefedForMapping(t)

	require.NoError(t, recordMapping(t, `{"claim":"`+mapIssue+`#c1","unit":"u1"}`))

	stored := storedGapFields(t, layout)
	require.Len(t, stored, 2, "§4.1.3: c2 and c3 are mapped to no unit")
	assert.Equal(t, `"`+mapIssue+`#c2"`, string(stored[0]["claim"]))
	assert.Equal(t, `"`+mapHead+`"`, string(stored[0]["head"]),
		"§2.3.3's head is written by cr, and the mapping file carried none")
	assert.Equal(t, "2", string(stored[0]["round"]),
		"§2.3.3's round is the round the mapping was recorded in")
	assert.Equal(t, `""`, string(stored[0]["set_aside_note"]),
		"§4.1.3 carries the field; §4.1.8 is what fills it")
	assert.Equal(t, `"`+mapIssue+`#c3"`, string(stored[1]["claim"]))

	require.NoError(t, recordMapping(t,
		`{"claim":"`+mapIssue+`#c1","unit":"u1"}`,
		`{"claim":"`+mapIssue+`#c2","unit":"u2"}`))

	after, err := state.ReadRecords[mapping.Gap](layout, mapOwner, mapRepo, mapPR, state.FileIntentGaps)
	require.NoError(t, err)
	require.Len(t, after, 1, "§4.1.7 derives the file, so a claim that became mapped drops out")
	assert.Equal(t, mapIssue+"#c3", after[0].Claim)
	assert.Equal(t, 2, after[0].Round)
}

// The entries a round derives replace that round's and leave every other round
// in the file byte for byte, per §9.3.5.
func TestTheDerivedEntriesAreScopedToTheRound(t *testing.T) {
	layout := briefedForMapping(t)
	held, err := layout.LockPR(mapOwner, mapRepo, mapPR)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileIntentGaps,
		[]byte(`{"claim":"`+mapIssue+`#c1","head":"0f1e2d3","round":1,"set_aside_note":"`+
			mapIssue+`#n1"}`+"\n")))
	require.NoError(t, held.Unlock())

	require.NoError(t, recordMapping(t, `{"claim":"`+mapIssue+`#c1","unit":"u1"}`))

	stored, err := state.ReadRecords[mapping.Gap](layout, mapOwner, mapRepo, mapPR, state.FileIntentGaps)
	require.NoError(t, err)
	rendered := make([]string, 0, len(stored))
	for i := range stored {
		rendered = append(rendered,
			stored[i].Claim+"@"+strconv.Itoa(stored[i].Round)+"/"+stored[i].SetAsideNote)
	}
	assert.Equal(t, []string{
		mapIssue + "#c1@1/" + mapIssue + "#n1",
		mapIssue + "#c2@2/",
		mapIssue + "#c3@2/",
	}, rendered, "§9.3.5: round 1's entry is history and is left exactly as it was")
}

// setAside stamps §4.1.8's note onto the round's entry for one claim, by
// writing the file `cr claims set-aside` will write once it exists.
//
// The command is still a stub — claims-set-aside-command owns it — so the
// stamp is put where the command will put it rather than faked somewhere the
// derivation does not read. Every other entry of the file is rewritten exactly
// as it stood, because §4.1.7's carry-forward is read out of this file and a
// helper that dropped a neighbour would be testing its own damage.
func setAside(t *testing.T, l state.Layout, claim, note string) {
	t.Helper()
	stored, err := state.ReadRecords[mapping.Gap](l, mapOwner, mapRepo, mapPR, state.FileIntentGaps)
	require.NoError(t, err)
	var body bytes.Buffer
	for i := range stored {
		if stored[i].Claim == claim {
			stored[i].SetAsideNote = note
		}
		line, err := json.Marshal(stored[i])
		require.NoError(t, err)
		body.Write(line)
		body.WriteByte('\n')
	}
	held, err := l.LockPR(mapOwner, mapRepo, mapPR)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileIntentGaps, body.Bytes()))
	require.NoError(t, held.Unlock())
}

// recordedMapping runs `cr map record` and returns the document it printed,
// which is where §4.1.7's dropped-stamp report has to appear.
func recordedMapping(t *testing.T, lines ...string) mapRecordResult {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mapping.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"map", "record", strconv.Itoa(mapPR), path, "--repo", mapSlug})
	require.NoError(t, cmd.Execute())
	var printed mapRecordResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &printed))
	return printed
}

// notesOf renders intent-gaps.ndjson as `claim/note`, which is the whole of
// what a carry-forward can be right or wrong about.
func notesOf(t *testing.T, l state.Layout) []string {
	t.Helper()
	stored, err := state.ReadRecords[mapping.Gap](l, mapOwner, mapRepo, mapPR, state.FileIntentGaps)
	require.NoError(t, err)
	rendered := make([]string, 0, len(stored))
	for i := range stored {
		rendered = append(rendered, stored[i].Claim+"/"+stored[i].SetAsideNote)
	}
	return rendered
}

// A re-recorded mapping keeps the §4.1.8 set-aside of a claim that is still
// unmapped, and drops it — saying so — for a claim that has since become
// mapped.
//
// This is round 9's `derived-state-clobbers-decision` and round 12's
// `derived-file-erases-recorded-decision` end to end, through the command
// §4.1.6 permits to be run again rather than through the derivation alone. Both
// findings are about a literal re-derivation handing every claim a fresh entry
// with no `set_aside_note`: the reviewer's judgement goes, §10.2.3's block
// silently returns, and nothing in the run says it happened.
//
// The middle recording is the same mapping as the first, which is the case that
// matters most and the one a derivation keyed on "did anything change" would
// pass by accident: nothing about c2 changed, so nothing about c2's decision
// may.
//
// The last recording asserts both halves of the drop together. The entry has to
// go — c2 is mapped, so §4.1.3 raises nothing for it — and the run has to say
// which decision that cost, because after the write there is nowhere left in
// the file to read it from.
func TestReRecordingKeepsASetAsideAndReportsTheOneItDrops(t *testing.T) {
	layout := briefedForMapping(t)
	first := recordedMapping(t, `{"claim":"`+mapIssue+`#c1","unit":"u1"}`)
	require.Empty(t, first.DroppedSetAsides, "nothing was set aside yet")

	setAside(t, layout, mapIssue+"#c2", mapIssue+"#n7")

	unchanged := recordedMapping(t, `{"claim":"`+mapIssue+`#c1","unit":"u1"}`)

	assert.Equal(t, []string{mapIssue + "#c2/" + mapIssue + "#n7", mapIssue + "#c3/"},
		notesOf(t, layout),
		"§4.1.6 permits the mapping to be re-recorded, so the note survives the re-derivation")
	assert.Empty(t, unchanged.DroppedSetAsides,
		"a stamp that was kept is not a stamp that was dropped")

	mapped := recordedMapping(t,
		`{"claim":"`+mapIssue+`#c1","unit":"u1"}`,
		`{"claim":"`+mapIssue+`#c2","unit":"u2"}`)

	assert.Equal(t, []string{mapIssue + "#c3/"}, notesOf(t, layout),
		"§4.1.7 derives the file, so a claim that became mapped raises no entry to hold a stamp")
	assert.Equal(t,
		[]mapping.DroppedSetAside{{Claim: mapIssue + "#c2", SetAsideNote: mapIssue + "#n7"}},
		mapped.DroppedSetAsides,
		"round 9: the drop is correct, and a correct drop nothing reports is indistinguishable "+
			"from the clobber round 12 names")
}

// The dropped-stamp report is an empty array rather than an absent key when a
// run dropped nothing, per §12.3.
//
// It is read off the wire because that is the difference the reader sees: a
// missing key reads as a command that does not report drops at all, and a
// caller checking for the field would take the silence for the older behaviour
// rather than for a run that revoked nothing.
func TestARunThatDroppedNoStampReportsAnEmptyArray(t *testing.T) {
	briefedForMapping(t)
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	path := filepath.Join(t.TempDir(), "mapping.ndjson")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"claim":"`+mapIssue+`#c1","unit":"u1"}`+"\n"), 0o600))
	cmd.SetArgs([]string{"map", "record", strconv.Itoa(mapPR), path, "--repo", mapSlug})
	require.NoError(t, cmd.Execute())

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(out.Bytes(), &fields))
	require.Contains(t, fields, "dropped_set_asides")
	assert.Equal(t, "[]", string(fields["dropped_set_asides"]))
}
