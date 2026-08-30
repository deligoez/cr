package state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundsRecorded writes one line per round into name, so a round-scoped write
// has history to leave alone. The lines go in as bytes rather than through a
// writer: the round is the whole of what earlierRounds reads, and a fixture
// built through WriteStamped could only ever hold one round at a time.
func roundsRecorded(t *testing.T, k *Lock, name string, rounds ...int) {
	t.Helper()
	var body []byte
	for _, round := range rounds {
		body = fmt.Appendf(body, "{\"id\":\"r%d\",\"head\":\"0f1e2d3\",\"round\":%d}\n", round, round)
	}
	require.NoError(t, k.Write(name, body))
}

// §3.3.1 replaces claims.ndjson and §9.3.5 scopes the replacement: the current
// round's records go, every other round's stay.
//
// The fixture holds two earlier rounds rather than one, so a writer that kept
// only the line nearest the replacement would still fail, and the round being
// replaced sits between them, so keeping "everything before" is not enough
// either — earlierRounds has to take out one round and leave both neighbours.
func TestReplaceStampedRewritesOneRoundAndLeavesTheOthers(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	roundsRecorded(t, held, FileClaims, 1, 2, 3)

	at := Stamp{Head: "9a8b7c6", Round: 2}
	require.NoError(t, ReplaceStamped(held, FileClaims, at, []*stampedRecord{{ID: "c9"}}))
	require.NoError(t, held.Unlock())

	got, err := ReadRecords[stampedRecord](l, "acme", "web", 42, FileClaims)
	require.NoError(t, err)
	assert.Equal(t, []stampedRecord{
		{Stamp: Stamp{Head: "0f1e2d3", Round: 1}, ID: "r1"},
		{Stamp: Stamp{Head: "0f1e2d3", Round: 3}, ID: "r3"},
		{Stamp: at, ID: "c9"},
	}, got, "§9.3.5: the round is replaced, and earlier rounds are left intact")
}
