package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/state"
)

// earlierHead is the head a seeded probe was recorded at: the one the round
// that posted the record stood at before the author pushed.
const earlierHead = "0ld0000"

// seedProbes writes probes.ndjson for one pull request, one line per record, as
// cr's own writer would have left it.
func seedProbes(t *testing.T, path string, records ...probe.Record) {
	t.Helper()
	lines := make([]string, 0, len(records))
	for i := range records {
		line, err := json.Marshal(&records[i])
		require.NoError(t, err)
		lines = append(lines, string(line))
	}
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
}

// seededMutation is p1 as a round before this one recorded it: §5.3.1's
// mutation of the fixture, at an earlier head.
func seededMutation() probe.Record {
	return probe.Record{
		ID: "p1", Kind: probe.Mutation, Stamp: state.Stamp{Head: earlierHead, Round: 1},
		Input: fixtureDiff, Result: "test-failed", Baseline: "r1", Target: "app.go:3",
		OutputTail: "Tests:  1 failed\n",
	}
}

// §5.5.4: `cr probe run --rerun <probe-id>` performs the stored mutation at
// the current round's head, as if its patch had been given with `--patch`, and
// writes a new record whose `rerun_of` names the probe it re-ran.
//
// The stored probe is a round-1 record at an earlier head, which is the case
// the flag exists for: a posted record's probe re-run after the author pushed.
// The runner's log is what says the stored patch, and not something else, was
// what the suite ran against.
func TestAReRunPerformsTheStoredMutationAtTheRoundsHead(t *testing.T) {
	prepared, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
	seedProbes(t, prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileProbes),
		seededMutation())

	reported := probeDocument(t, throughAPipe(t,
		"probe", "run", fixturePR, "--repo", fixtureSlug, "--rerun", "p1"))

	assert.Equal(t, "p2", reported["probe"])
	assert.Equal(t, "p1", reported["rerun_of"])
	assert.Equal(t, "mutation", reported["kind"])
	assert.Equal(t, "no-test-failed", reported["result"])

	observed, err := os.ReadFile(log)
	require.NoError(t, err, "the profile's test command never ran")
	assert.Equal(t, fixtureSource+"---\n"+fixtureMutated+"---\n", string(observed),
		"§5.5.4: the baseline and then the stored patch, at the round's head")

	probes := storedRecords(t, prepared, state.FileProbes)
	require.Len(t, probes, 2, "a re-run is a new record and the one it re-ran is kept")
	assert.Equal(t, earlierHead, probes[0]["head"], "the stored probe is left as it was")
	assert.NotContains(t, probes[0], "rerun_of")
	rerun := probes[1]
	assert.Equal(t, "p2", rerun["id"])
	assert.Equal(t, "p1", rerun["rerun_of"])
	assert.Equal(t, float64(2), rerun["round"], "the re-run is the current round's")
	assert.NotEqual(t, earlierHead, rerun["head"], "the re-run is stamped with the round's head")
	assert.Equal(t, fixtureDiff, rerun["input"])
	assert.Equal(t, "app.go:3", rerun["target"], "§5.3.2 derives the target from the patch again")
}

