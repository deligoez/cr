package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/review"
)

// §4.6.1 through the command: `--units` narrows the prompts to the units it
// names and `--shard` to its part of them, while an id that is no unit of the
// current round is refused with §11.2's code 1, naming the round's units and
// the step that lists them.
func TestReviewNarrowsThePromptsByUnitsAndShard(t *testing.T) {
	statusHome(t)

	printed, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug, "--units", "u9")
	var unknown *review.UnknownUnitError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, review.UnknownUnitError{Unit: "u9", Round: 1, Units: []string{"u1", "u2"}}, *unknown)
	assert.Equal(t, `--units names "u9", which is not a unit of round 1; §4.6.1 narrows the prompts to units `+
		"of the current round, which are u1, u2", err.Error())
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, "name only units of the current round, which `cr brief <pr>` lists, or use --shard <k>/<n>",
		hintFor(err))
	assert.Empty(t, printed, "a refused run emits nothing")

	assert.Equal(t, []string{"u2", "u2", "u2"}, promptUnits(fanoutOf(t, "--units", "u2").Prompts))
	assert.Equal(t, []string{"u1", "u1", "u1", "u2", "u2", "u2"},
		promptUnits(fanoutOf(t, "--units", "u1,u2", "--all").Prompts), "a comma-separated list names both")
	assert.Equal(t, []string{"u2", "u2", "u2"}, promptUnits(fanoutOf(t, "--shard", "2/2").Prompts))
	assert.Equal(t, []string{"u1", "u1", "u1"}, promptUnits(fanoutOf(t, "--shard", "1/2", "--all").Prompts))
	assert.Empty(t, fanoutOf(t, "--shard", "3/3").Prompts, "a shard past the units emits nothing")
}

// §4.6.1 aborts with §11.2's code 2 on a `--shard` that is not `<k>/<n>` with
// 1 <= k <= n, and on `--units` given together with `--shard`. Neither is a
// fault in the round's state: the command line is what is wrong.
func TestReviewRefusesAMalformedShardAndShardWithUnits(t *testing.T) {
	statusHome(t)

	for _, value := range []string{"", "2", "1/0", "0/2", "3/2", "x/2", "1/y", "-1/2", "1/2/3"} {
		printed, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug, "--shard", value)
		var malformed *shardFlagError
		require.ErrorAsf(t, err, &malformed, "--shard %q", value)
		assert.Equal(t, `--shard "`+value+`" is not <k>/<n> with whole numbers 1 <= k <= n, per §4.6.1`, err.Error())
		assert.Equal(t, ExitUsage, exitCodeFor(err))
		assert.Empty(t, printed)
	}

	printed, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug,
		"--units", "u1", "--shard", "1/2")
	var both *unitsWithShardError
	require.ErrorAs(t, err, &both)
	assert.Equal(t, "--units and --shard both narrow the units, and §4.6.1 accepts one of them at a time",
		err.Error())
	assert.Equal(t, ExitUsage, exitCodeFor(err))
	assert.Empty(t, printed)
}

// §4.6.5's refusal composes with §4.6.1's narrowing: the intent pass of a round
// with no claims recorded is refused with code 4 however the prompts were
// narrowed, and `--units` naming no unit of the round is refused first, because
// it is the invocation that is wrong.
func TestTheIntentPassRefusalHoldsUnderNarrowing(t *testing.T) {
	_, _, u := rerecordHome(t)

	for _, args := range [][]string{
		{"--axis", axis.Intent},
		{"--axis", axis.Intent, "--units", u},
		{"--axis", axis.Intent, "--shard", "1/1"},
		{"--axis", axis.Intent, "--all"},
	} {
		_, err := runCLIPrinting(t, append([]string{"review", fixturePR, "--repo", fixtureSlug}, args...)...)
		var required *review.ClaimsRequiredError
		require.ErrorAsf(t, err, &required, "%v", args)
		assert.Equal(t, ExitState, exitCodeFor(err))
	}

	_, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug,
		"--axis", axis.Intent, "--units", "u404")
	var unknown *review.UnknownUnitError
	require.ErrorAs(t, err, &unknown, "the invocation is settled before the round's state is judged")
}

// §4.6.3's marks reach a terminal: a cell the round holds is marked recorded,
// and one §4.6.1's note condition holds for is marked stale by note, which is
// also the cell whose prompt the next run emits again.
func TestATerminalReviewMarksRecordedAndStaleCells(t *testing.T) {
	statusHome(t)
	// The round holds u1's cells and nothing has been emitted for them, so
	// this run is what gives them an emission a later note can postdate.
	_, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug, "--all")
	require.NoError(t, err)

	lines := terminalLines(t, throughATerminal(t, "review", fixturePR, "--repo", fixtureSlug))
	assert.Contains(t, lines, "  u1 convention (recorded)")
	assert.Contains(t, lines, "  u2 convention")

	_, err = runCLIPrinting(t, "note", fixtureIssue, "the shipping table moved.",
		"--source", "chat", "--pr", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	// `--units u2` leaves u1's emissions alone, so the marks below are read
	// before any run of this test carries the note to u1's prompts.
	lines = terminalLines(t, throughATerminal(t, "review", fixturePR, "--repo", fixtureSlug, "--units", "u2"))
	assert.Contains(t, lines, "  u1 convention (recorded) (stale by note)")
	assert.Equal(t, []string{"u1", "u1", "u1", "u2", "u2", "u2"}, promptUnits(fanoutOf(t).Prompts),
		"§4.6.1: the note postdates u1's prompts, so they are emitted again")
	assert.Equal(t, []string{"u2", "u2", "u2"}, promptUnits(fanoutOf(t).Prompts),
		"and the run that carried it leaves them held again")
}

// terminalLines splits what a command printed at a terminal into lines.
func terminalLines(t *testing.T, printed string) []string {
	t.Helper()
	return strings.Split(strings.ReplaceAll(printed, "\r", ""), "\n")
}
