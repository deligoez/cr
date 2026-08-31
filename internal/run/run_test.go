package run

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

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

// Round 12's unstorable-value finding, which is what `timed_out` is for.
// §5.2.3 requires a run cr killed to be recorded as timeout, and the exit
// status is no place to keep that: a killed process and a runner that decided
// to fail report the same kind of number, and §5.3.4's ladder puts the two on
// different rungs — `timeout` above `error`, and both above every reading of
// the counts. So the two records below agree on everything the runner reported
// and disagree on the one field that says which happened.
func TestAKilledRunIsDistinguishableFromARunnerThatExitedNonZero(t *testing.T) {
	killed := &Record{ID: "r1", ExitCode: 1, TimedOut: true}
	refused := &Record{ID: "r2", ExitCode: 1, TimedOut: false}

	assert.Equal(t, killed.ExitCode, refused.ExitCode,
		"the exit code alone cannot tell the two apart, which is why the field exists")

	var stored [2]map[string]any
	for i, record := range []*Record{killed, refused} {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(line, &stored[i]))
	}
	assert.Equal(t, true, stored[0]["timed_out"], "§5.2.3: the kill is recorded")
	assert.Equal(t, false, stored[1]["timed_out"],
		"a run that finished carries the field too, so its absence is never the answer")
}

// §5.2.4 stores the two counts "when derivable", and §5.2.1 makes an
// undetermined count a different fact from a zero one: a `tests.failed_pattern`
// that never matched yields zero, because Pest and its like print a failed line
// only when something failed, while a `tests.count_pattern` that never matched
// leaves both undetermined. §5.2.5 then reads the difference — an underivable
// run never passes and so can never be a baseline — so the two states have to
// survive the round trip through the file rather than collapsing into 0.
func TestAnUndeterminedCountIsAbsentFromTheRecordAndAZeroOneIsNot(t *testing.T) {
	passing := &Record{ID: "r1", TestsRun: count(4), TestsFailed: count(0)}
	unreadable := &Record{ID: "r2"}

	var stored [2]map[string]any
	for i, record := range []*Record{passing, unreadable} {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(line, &stored[i]))
	}
	assert.Equal(t, float64(4), stored[0]["tests_run"])
	assert.Equal(t, float64(0), stored[0]["tests_failed"],
		"§5.2.1: no match of the failed pattern is zero failures, and zero is a measurement")
	assert.NotContains(t, stored[1], "tests_run",
		"§5.2.4: an underivable count is not stored as a number")
	assert.NotContains(t, stored[1], "tests_failed")

	read := make([]Record, 0, 2)
	for i := range stored {
		line, err := json.Marshal(stored[i])
		require.NoError(t, err)
		var back Record
		require.NoError(t, json.Unmarshal(line, &back))
		read = append(read, back)
	}
	require.NotNil(t, read[0].TestsFailed)
	assert.Equal(t, 0, *read[0].TestsFailed)
	assert.Nil(t, read[1].TestsRun, "an absent count reads back as no count, not as zero")
	assert.Nil(t, read[1].TestsFailed)
}

// §5.2.4's id space, which §5.5 has a probe's `baseline` reference. An id is
// allocated above every id the file already holds rather than at the count of
// records, so a probe stored at an earlier head keeps pointing at the run it
// measured; and an id cr did not write is no evidence about what is taken, so
// it moves nothing.
func TestNextIDAllocatesAboveEveryRunTheFileHolds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing []Record
		want     string
	}{
		{name: "an empty file", existing: []Record{}, want: "r1"},
		{name: "one run", existing: []Record{{ID: "r1"}}, want: "r2"},
		{
			name:     "a gap left by a run of an earlier round",
			existing: []Record{{ID: "r1"}, {ID: "r7"}},
			want:     "r8",
		},
		{
			name:     "records out of order",
			existing: []Record{{ID: "r9"}, {ID: "r2"}},
			want:     "r10",
		},
		{
			name:     "spellings cr never wrote",
			existing: []Record{{ID: "r0"}, {ID: "r007"}, {ID: "r-3"}, {ID: "p4"}, {ID: ""}},
			want:     "r1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NextID(tc.existing))
		})
	}
}

// §5.2.4's `output_tail` is the last `tests.output_tail_bytes` of the runner's
// output, and the runner produces it in pieces. So the bound holds across
// however many writes the suite makes, including one write larger than the
// whole budget, and the end of the output is what survives — a suite says what
// failed at the end, not at the beginning.
func TestTheOutputTailKeepsTheLastBytesOfEverythingWritten(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		writes []string
		want   string
	}{
		{name: "output within the budget", limit: 16, writes: []string{"PASS\n"}, want: "PASS\n"},
		{
			name:   "one write over the budget",
			limit:  4,
			writes: []string{"abcdefgh"},
			want:   "efgh",
		},
		{
			name:   "several writes over the budget together",
			limit:  6,
			writes: []string{"abcd", "efgh", "ij"},
			want:   "efghij",
		},
		{name: "nothing written at all", limit: 8, writes: nil, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tail := NewTail(tc.limit)
			for _, piece := range tc.writes {
				n, err := tail.Write([]byte(piece))
				require.NoError(t, err)
				assert.Equal(t, len(piece), n,
					"truncation is the tail's business, not a short write")
			}
			assert.Equal(t, tc.want, tail.String())
		})
	}
}

// The bound is in bytes, so a cut can land inside a multi-byte rune. The
// fragment it leaves is not a character, and json.Marshal would silently write
// U+FFFD in its place — a replacement no reader could tell from output that
// really held one. So it is dropped, and what is left is valid UTF-8 within
// the byte bound.
func TestTheOutputTailDoesNotEndAHalfRuneAtTheFrontOfTheOutput(t *testing.T) {
	// Four runes of three bytes each, and a budget that cuts the second
	// of them in half.
	tail := NewTail(10)
	_, err := tail.Write([]byte("日本語テ"))
	require.NoError(t, err)

	held := tail.String()
	assert.True(t, utf8.ValidString(held), "a stored tail is valid UTF-8")
	assert.Equal(t, "本語テ", held)
	assert.LessOrEqual(t, len(held), 10, "§5.2.4's bound is in bytes and still holds")
}

// count is the address of one derived test count, which is what §5.2.4's
// "when derivable" needs a literal to be able to express.
func count(n int) *int { return &n }
