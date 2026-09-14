package brief

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
)

// A same-head brief carries meta.json's mapping stamp through, as it carries
// post_unresolved: `cr map record` wrote it, and a brief that dropped it would
// hold a round §4.6.5 had already unblocked.
func TestASameHeadBriefKeepsTheMappingStamp(t *testing.T) {
	dir, head, base := repository(t)
	src := sources(t, dir, answering(head, base, oneThread))
	_, err := Run(src)
	require.NoError(t, err)

	meta, err := src.Layout.ReadMeta(testOwner, testRepo, testPR)
	require.NoError(t, err)
	meta.MappingRound, meta.MappingHead = meta.Round, meta.Head
	held, err := src.Layout.LockPR(testOwner, testRepo, testPR)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())

	_, err = Run(src)
	require.NoError(t, err)

	after, err := src.Layout.ReadMeta(testOwner, testRepo, testPR)
	require.NoError(t, err)
	assert.True(t, after.MappingRecorded(), "the stamp survives a brief that opened no round")
}

// The claims stamp survives a same-head brief, and moves with the claims
// §9.3.4 carries into the round a moved head opens; a closing round that
// recorded no claims opens a round that has recorded none either.
func TestTheClaimsStampFollowsTheCarriedClaims(t *testing.T) {
	for name, stamped := range map[string]bool{"recorded": true, "never recorded": false} {
		t.Run(name, func(t *testing.T) {
			dir, first, base := repository(t)
			src := sources(t, dir, answering(first, base, oneThread))
			_, err := Run(src)
			require.NoError(t, err)
			if stamped {
				held, err := src.Layout.LockPR(testOwner, testRepo, testPR)
				require.NoError(t, err)
				require.NoError(t, held.StampClaims(1, first))
				require.NoError(t, held.Unlock())
			}

			_, err = Run(src)
			require.NoError(t, err)
			same, err := src.Layout.ReadMeta(testOwner, testRepo, testPR)
			require.NoError(t, err)
			assert.Equal(t, stamped, same.ClaimsRecorded(), "a same-head brief")

			second := advanceAdding(t, dir)
			src.GH = gh.WithRunner(answering(second, base, oneThread))
			_, err = Run(src)
			require.NoError(t, err)
			opened, err := src.Layout.ReadMeta(testOwner, testRepo, testPR)
			require.NoError(t, err)
			require.Equal(t, 2, opened.Round)
			assert.Equal(t, stamped, opened.ClaimsRecorded(), "the round a moved head opened")
		})
	}
}
