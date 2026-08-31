package run

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recap shape the shipped laravel-pest profile reads, reduced to what
// §5.2.1's arithmetic turns on: a runner names one status per count and prints
// no status at all for a count of zero, so several matches make up one run's
// executed count and an all-passing run's failed count comes from no match.
const (
	countPattern  = `(?:Tests:|,)\s+(\d+)\s+(?:failed|passed)\b`
	failedPattern = `(?:Tests:|,)\s+(\d+)\s+failed\b`
)

// §5.2.1: each pattern is matched repeatedly and its captures are summed, and
// the two counts are undetermined together on everything the sum cannot be
// taken over.
//
// "Last match wins" is what this rules out. The spec review removed that shape
// deliberately: a recap that spends one match per status reports `2 failed, 4
// passed` as two matches, and a reader that kept only the last of them would
// call a six-test run a four-test one — or, reading the failed pattern the
// same way, would lose the earlier of two failure lines. Every case below with
// more than one match fails under it.
func TestEveryMatchOfEachPatternIsSummedRatherThanTheLastOneWinning(t *testing.T) {
	cases := map[string]struct {
		output   string
		executed string
		failed   string
	}{
		// A pattern that never matches leaves both undetermined, and
		// §5.3.4 rung 5 turns that into `inconclusive` rather than
		// into evidence.
		"no match of either pattern": {
			output:   "No tests found.\n",
			executed: "undetermined",
			failed:   "undetermined",
		},
		"one match": {
			output:   "Tests:  4 passed (9 assertions)\n",
			executed: "4",
			failed:   "0",
		},
		// The case the summing rule exists for.
		"several matches that must sum": {
			output:   "Tests:  2 failed, 1 passed, 3 passed (11 assertions)\n",
			executed: "6",
			failed:   "2",
		},
		// Two failure lines, so the failed pattern sums too rather
		// than reporting whichever came last.
		"several failed matches": {
			output:   "Tests:  2 failed, 4 passed\nTests:  3 failed, 1 passed\n",
			executed: "10",
			failed:   "5",
		},
		// §5.2.1's asymmetry, and the run §5.2.5 must be able to call
		// passed: nothing in the output says the word failed.
		"an all-passing run whose failed pattern is absent": {
			output:   "Tests:  4 passed (9 assertions)\nDuration: 0.11s\n",
			executed: "4",
			failed:   "0",
		},
		// A count pattern matching text that is not a count leaves
		// both undetermined rather than counting what it could read.
		"an unparseable capture in the count pattern": {
			output:   "Tests:  4 passed\nTests:  99999999999999999999 passed\n",
			executed: "undetermined",
			failed:   "undetermined",
		},
		// Undetermined *both*, per §5.2.1: an executed count that
		// stood alone beside an unreadable failed count would be a
		// half-measured run, and §5.4.3 rung 4 reads either one being
		// undetermined as inconclusive.
		"an unparseable capture in the failed pattern": {
			output:   "Tests:  99999999999999999999 failed, 4 passed\n",
			executed: "undetermined",
			failed:   "undetermined",
		},
		// A sum larger than a count can be is refused rather than
		// wrapped into a smaller number than the run produced.
		"a sum too large to be a count": {
			output:   "Tests:  2000000000 passed\nTests:  2000000000 passed\n",
			executed: "undetermined",
			failed:   "undetermined",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			counter, err := NewCounter(countPattern, failedPattern)
			require.NoError(t, err)
			written, err := counter.Write([]byte(tc.output))
			require.NoError(t, err)
			assert.Equal(t, len(tc.output), written,
				"the whole of what the caller wrote is accepted")

			executed, failed := counter.Counts()
			assert.Equal(t, tc.executed, shown(executed), "executed count")
			assert.Equal(t, tc.failed, shown(failed), "failed count")
		})
	}
}

// A capture that reads as a negative number is not a base-10 non-negative
// integer, so it leaves both counts undetermined rather than being subtracted
// from the total — which is what a signed parse would have done, and what
// would let a runner's own output talk a failed count down to zero.
func TestANegativeCaptureIsNotACount(t *testing.T) {
	counter, err := NewCounter(`ran (-?\d+)`, `failed (-?\d+)`)
	require.NoError(t, err)
	_, err = counter.Write([]byte("ran 4\nfailed 2\nfailed -2\n"))
	require.NoError(t, err)

	executed, failed := counter.Counts()
	assert.Nil(t, executed, "§5.2.1: a group that does not parse leaves both undetermined")
	assert.Nil(t, failed)
}

// §5.2.1 matches over the runner's output as a whole, not over one write of
// it, so a recap split across writes — which is how it arrives from a pipe —
// still sums. The counter is an io.Writer for exactly this reason: it is teed
// off the merged stream as it is produced.
func TestOutputArrivingInPiecesIsCountedAsOneStream(t *testing.T) {
	counter, err := NewCounter(countPattern, failedPattern)
	require.NoError(t, err)
	for _, piece := range []string{"Tests:  2 fail", "ed, 4 pas", "sed\n"} {
		_, err := counter.Write([]byte(piece))
		require.NoError(t, err)
	}

	executed, failed := counter.Counts()
	assert.Equal(t, "6", shown(executed))
	assert.Equal(t, "2", shown(failed))
}

// §2.4 makes `tests.count_pattern` optional and §5.2.1 records the counts only
// when it is configured, so a profile without one determines nothing however
// countable its output looks — and retains none of that output either, since
// there is nothing to match it against.
func TestAProfileWithNoCountPatternDeterminesNothingAndKeepsNothing(t *testing.T) {
	for name, patterns := range map[string][2]string{
		"neither pattern":       {"", ""},
		"only a failed pattern": {"", failedPattern},
		// §2.4 requires the pair together and profile.Parse
		// refuses the half-configured file, so this is the answer
		// for a pattern that reached here some other way: an
		// unmatched failed pattern is zero, and zero failures out
		// of a count nothing constrains is a run §5.2.5 would call
		// passed.
		"only a count pattern": {countPattern, ""},
	} {
		t.Run(name, func(t *testing.T) {
			counter, err := NewCounter(patterns[0], patterns[1])
			require.NoError(t, err)
			_, err = counter.Write([]byte("Tests:  4 passed\n"))
			require.NoError(t, err)

			executed, failed := counter.Counts()
			assert.Nil(t, executed)
			assert.Nil(t, failed)
			assert.Zero(t, counter.held.Len(),
				"no pattern to match means no reason to hold the suite's output")
		})
	}
}

// A pattern that does not compile is reported rather than silently disabling
// the counts, and the message names which of the two fields to fix. §2.4 makes
// this unreachable through a loaded profile, which validates both patterns;
// what it must not become is a run that quietly counts nothing.
func TestAnUncompilablePatternIsReportedAndNamesItsField(t *testing.T) {
	_, err := NewCounter(`(\d+`, failedPattern)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tests.count_pattern")

	_, err = NewCounter(countPattern, `(\d+`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tests.failed_pattern")
}

// shown renders one of §5.2.4's optional counts, so a test can say what it
// expects without a pointer comparison and an absent count reads as the answer
// it is.
func shown(n *int) string {
	if n == nil {
		return "undetermined"
	}
	return strconv.Itoa(*n)
}
