package cli

import (
	"bytes"
	"encoding/json"
	"strconv"
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
