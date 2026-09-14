package review

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// heldRecord is a stored record holding id, stamped with round.
func heldRecord(id string, round int) finding.Finding {
	return finding.Finding{ID: id, Stamp: state.Stamp{Round: round, Head: "abc123"}}
}

// A block is a hundred ids, placed by the prompt's cell in the round's whole
// grid of active roles times units and started past the highest id an earlier
// round stored. Within its block a prompt starts past the ids this round's
// stored records hold there, and a prompt whose block's last id is held names
// no run at all and says so, rather than handing out an id past its block.
func TestAPromptsRunOfIDsIsItsBlockPastTheIDsStoredInIt(t *testing.T) {
	r := handRound()
	r.Round = 3
	r.Held = []finding.Finding{
		heldRecord("f40", 2), heldRecord("f7", 1),
		heldRecord("f100", 3), heldRecord("f150", 3), heldRecord("f143", 3),
		heldRecord("f440", 3), heldRecord("fx", 2),
	}

	prompts := Emit(r)

	require.Len(t, prompts, 4)
	got := make([][2]string, 0, len(prompts))
	for _, prompt := range prompts {
		got = append(got, [2]string{prompt.FirstID, prompt.LastID})
	}
	assert.Equal(t, [][2]string{{"f101", "f140"}, {"f151", "f240"}, {"f241", "f340"}, {"", ""}}, got,
		"intent-coverage/u1, intent-coverage/u2, correctness/u1, correctness/u2")
	assert.Contains(t, strings.Split(prompts[3].Text, "\n"),
		"This prompt's block of ids ends at f440, which a record stored for this pull request already "+
			"holds, so no id past the stored ones is left for a record written here; cr merge refuses an "+
			"id another record holds, with exit code 1 (§6.1).")
}
