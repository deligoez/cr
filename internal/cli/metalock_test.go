package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// briefLandsBetween stands in for a `cr brief` that took the §2.3.1 lock after
// a meta.json writer read its copy of the round and before that writer took
// the lock: it publishes meta.json with the fields brief decides moved on, and
// hands back what reached the file.
//
// The writers take the caller's copy of the round as an argument, so the
// interleaving needs no pause: the copy read before the brief is the copy a
// writer that read too early would publish. flock gives -race no happens-before
// edge to see, so what is asserted is what reached the file.
func briefLandsBetween(t *testing.T, layout state.Layout, moved func(*state.Meta)) state.Meta {
	t.Helper()
	briefed, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	moved(&briefed)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&briefed))
	require.NoError(t, held.Unlock())
	return briefed
}

// A brief that re-resolved the issue key, the profile and the active roles
// between `cr post`'s read of the round and its §8.4.4 write keeps all three,
// and the post keeps post_unresolved.
func TestPostUnresolvedKeepsWhatABriefWroteAfterTheRoundWasRead(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	read, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)

	briefed := briefLandsBetween(t, layout, func(m *state.Meta) {
		m.IssueKey, m.ProfileID = "CR-2", "briefed-profile"
		m.ActiveRoles = append(m.ActiveRoles, "briefed-role")
	})
	require.NoError(t, setPostUnresolved(layout, &read, true))

	stored, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	briefed.PostUnresolved = true
	assert.Equal(t, briefed, stored,
		"§2.3.1: the brief's fields and the post's flag both reach meta.json")
	assert.True(t, read.PostUnresolved, "the caller's copy reports what is on disk")
}

// A brief that opened the next round between `cr map record`'s read of the
// round and its stamp keeps its round and head, and the stamp names the round
// the mapping was stored for — which MappingRecorded then reads as no mapping
// for the round the brief opened.
func TestTheMappingStampKeepsWhatABriefWroteAfterTheRoundWasRead(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	read, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)

	briefed := briefLandsBetween(t, layout, func(m *state.Meta) {
		m.Round, m.Head, m.IssueKey = m.Round+1, "briefed-head", "CR-2"
	})
	_, err = storeMapping(layout, &read, nil, nil)
	require.NoError(t, err)

	stored, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	briefed.MappingRound, briefed.MappingHead = read.Round, read.Head
	assert.Equal(t, briefed, stored,
		"§2.3.1: the brief's round, head and key and the mapping's stamp all reach meta.json")
	assert.False(t, stored.MappingRecorded(), "§4.6.5: the opened round has no mapping yet")
}
