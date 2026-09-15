package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// writeRoleFile writes a correctness role named id into dir, which is one of
// §2.2's two on-disk role layers.
func writeRoleFile(t *testing.T, dir, id string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".json"), []byte(
		`{"id":"`+id+`","title":"`+id+`","axis":"correctness","instructions":"Check it.","focus":[]}`), 0o600))
}

// reBrief stands in for a `cr brief` of the same round that records active as
// the round's active roles, which is all a re-brief at an unmoved head changes
// that `cr review` reads.
func reBrief(t *testing.T, layout state.Layout, active ...string) {
	t.Helper()
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.ActiveRoles = active
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())
}

// promptsOf keeps the prompts of one role.
func promptsOf(fan *review.Fanout, roleID string) []review.Prompt {
	kept := make([]review.Prompt, 0, len(fan.Prompts))
	for i := range fan.Prompts {
		if fan.Prompts[i].Role == roleID {
			kept = append(kept, fan.Prompts[i])
		}
	}
	return kept
}

// Release QA S04: a re-brief that changed the round's active roles re-assigned
// the id blocks `cr review` hands out, so the files the roles wrote from the
// earlier emission collided with the new one's and `cr merge` refused them.
//
// The round's active roles change three ways here, each one a re-emission of
// the same round: a global role joins the set, a per-repository override of a
// built-in role moves that role to the front of §2.5.5's corpus, and a role
// leaves the set. Every prompt keeps the run the first emission gave it, and the
// records written from the first emission merge beside the joining role's.
//
// Every emission here is `--all`: the fixture's round holds a cell for every
// role on u1, and §4.6.1's default narrowing would leave those prompts out of
// each run, which is not what this test is about.
func TestAReBriefThatChangesTheActiveRolesKeepsEveryIDBlockAnEarlierEmissionGave(t *testing.T) {
	statusHome(t)
	layout, err := state.Default()
	require.NoError(t, err)

	first := fanoutOf(t, "--all")
	require.Len(t, first.Prompts, 6, "three active roles over two units")
	firstRuns := runsOf(t, &first)

	writeRoleFile(t, layout.RolesDir(), "money-safety")
	writeRoleFile(t, layout.RepoRolesDir(fixtureOwner, fixtureProject), "correctness")
	reBrief(t, layout, "money-safety", "convention", "correctness", "intent-coverage")
	joined := fanoutOf(t, "--all")
	require.Len(t, joined.Prompts, 8, "four active roles over two units")
	joinedRuns := runsOf(t, &joined)
	for key, run := range firstRuns {
		assert.Equalf(t, run, joinedRuns[key], "%s keeps the run the first emission gave it", key)
	}
	assertNoTwoRunsOverlap(t, joinedRuns)

	reBrief(t, layout, "money-safety", "correctness", "intent-coverage")
	left := fanoutOf(t, "--all")
	require.Len(t, left.Prompts, 6, "convention left the active set")
	for key, run := range runsOf(t, &left) {
		assert.Equalf(t, joinedRuns[key], run, "%s keeps its run when another role leaves", key)
	}

	files := writeAtFirstIDs(t, first.Prompts)
	files = append(files, writeAtFirstIDs(t, promptsOf(&joined, "money-safety"))...)
	_, merged := mergeFanOut(t, files)
	assert.Equal(t, 8, merged)
}

// Release QA S04: an empty `--axis` was read as no `--axis` at all and emitted every
// prompt of the round. An empty value names no axis of §1.5, so it is refused
// like any other with §11.2's code 2 and the closed set named, however the
// empty value is spelled.
func TestAnEmptyAxisIsAUsageErrorNamingTheAxes(t *testing.T) {
	statusHome(t)

	for _, args := range [][]string{{"--axis", ""}, {"--axis="}} {
		printed, err := runCLIPrinting(t, append([]string{"review", fixturePR, "--repo", fixtureSlug}, args...)...)

		var unknown *unknownAxisFlagError
		require.ErrorAsf(t, err, &unknown, "%q", args)
		assert.Equal(t, ExitUsage, exitCodeFor(err))
		assert.Equal(t, `--axis "" is not an axis id; v0.3 has exactly intent, correctness, convention, test, per §1.5`,
			err.Error())
		assert.Empty(t, printed, "no prompt is emitted")
	}
}
