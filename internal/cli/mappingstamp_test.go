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

// unmappedStatusRound is statusHome's round with meta.json's mapping stamp
// taken off, which is a round whose intent axis is active and whose mapping no
// `cr map record` has stored.
func unmappedStatusRound(t *testing.T) state.Layout {
	t.Helper()
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.MappingRound, meta.MappingHead = 0, ""
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())
	return layout
}

// Round 9's mapping-existence-unobservable through the commands: an empty
// mapping — every unit mapped to zero claims — recorded with `cr map record`
// stamps the round and head on meta.json, and `cr review` then emits the
// remaining axes.
func TestAnEmptyMappingRecordedStampsMetaAndUnblocksTheRemainingAxes(t *testing.T) {
	layout := unmappedStatusRound(t)
	empty := filepath.Join(t.TempDir(), "mapping.ndjson")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))

	_, err := runCLIPrinting(t, "map", "record", fixturePR, empty, "--repo", fixtureSlug)
	require.NoError(t, err)

	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	assert.Equal(t, meta.Round, meta.MappingRound, "the stamp names the round the mapping was stored for")
	assert.Equal(t, meta.Head, meta.MappingHead, "and the head")

	_, err = runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err, "§4.6.5: a mapping exists for the round and head, empty as it is")
}

// Round 12's empty-vs-absent-mapping, the other half: with no mapping recorded
// the remaining axes still exit 4, although the round's mapping.ndjson and
// intent-gaps.ndjson hold what an empty mapping would have left.
func TestAnAbsentMappingStillExitsFour(t *testing.T) {
	layout := unmappedStatusRound(t)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileMapping, nil))
	require.NoError(t, held.Unlock())

	_, err = runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)

	var required *review.MappingRequiredError
	require.ErrorAs(t, err, &required)
	assert.Equal(t, ExitState, exitCodeFor(err))
}
