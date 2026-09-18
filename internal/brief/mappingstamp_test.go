package brief

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
