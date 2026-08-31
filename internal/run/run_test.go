package run

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §5.2.4's field list, and §2.3.3's `round` beside the `head` it travels with.
// It is spelled out here rather than derived from the package's own table,
// because a test that asked the table what the table says would agree with any
// drift at all.
func TestARunRecordCarriesExactlyTheFieldsSection524Names(t *testing.T) {
	full := &Record{
		ID:          "r3",
		Stamp:       state.Stamp{Head: "0a1b2c3", Round: 2},
		Filter:      "retries twice",
		ExitCode:    1,
		TimedOut:    true,
		DurationMS:  1420,
		TestsRun:    count(4),
		TestsFailed: count(1),
		OutputTail:  "Tests:  1 failed, 3 passed (5 assertions)\n",
		Passed:      false,
		Probe:       "p1",
	}
	line, err := json.Marshal(full)
	require.NoError(t, err)

	var written map[string]any
	require.NoError(t, json.Unmarshal(line, &written))
	assert.Equal(t, map[string]any{
		"id":           "r3",
		"head":         "0a1b2c3",
		"round":        float64(2),
		"filter":       "retries twice",
		"exit_code":    float64(1),
		"timed_out":    true,
		"duration_ms":  float64(1420),
		"tests_run":    float64(4),
		"tests_failed": float64(1),
		"output_tail":  "Tests:  1 failed, 3 passed (5 assertions)\n",
		"passed":       false,
		"probe":        "p1",
	}, written)
}

// The other half of the same fence: who writes each of those fields. §5.2.4's
// record is produced entirely by cr, so every row is measured except the two
// §2.3.3 stamps — and the absence of a third author is what says no agent
// submits a run record, where §6.1's table would carry Required and Optional
// rows for the fields an agent supplies.
func TestEveryFieldOfARunRecordIsWrittenByCR(t *testing.T) {
	require.True(t, checkRecordFields(), "the package's own check must agree")

	byStamp := make([]string, 0, 2)
	for _, row := range fields {
		switch row.Author {
		case stamped:
			byStamp = append(byStamp, row.Name)
		case measured:
		default:
			t.Fatalf("%s has an author that is neither cr nor the §2.3.3 stamp", row.Name)
		}
	}
	assert.Equal(t, []string{"head", "round"}, byStamp,
		"§2.3.3's pair reaches the record through state.Stamp, and nothing else does")

	var record any = &Record{}
	_, stampable := record.(state.Stamped)
	assert.True(t, stampable, "state.WriteStamped is the one writer of head and round")
}

// count is the address of one derived test count, which is what §5.2.4's
// "when derivable" needs a literal to be able to express.
func count(n int) *int { return &n }
