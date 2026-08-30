package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §3.7 makes `cr brief` the writer of the per-PR files the rest of cr reads as
// authoritative, so the three states a reader can find the pull request in are
// three different answers: no state directory at all, a directory no round has
// been opened in, and a briefed pull request.
//
// The middle one is the case worth the test. Layout.EnsurePR writes the whole
// §2.3 table with the identity alone and every NDJSON file in it empty, so a
// command reading meta.json directly would find a well-formed document naming
// round 0 and would then answer §4.1.6 and §4.5.6 against a unit set no round
// ever recorded. Both refusals name `cr brief`, which is the only way forward.
func TestReadingPerPRStateBeforeABriefIsRefusedNamingBrief(t *testing.T) {
	l := lockedPR(t)

	t.Run("no state directory", func(t *testing.T) {
		recorded, err := l.Briefed("acme", "web", 7)
		var missing *NotBriefedError
		require.ErrorAs(t, err, &missing)
		assert.Zero(t, recorded)
		assert.Equal(t, 0, missing.Round)
		assert.Error(t, missing.Unwrap(), "an absent directory carries the read that failed")
		assert.Contains(t, missing.Error(), "cr brief 7 --repo acme/web",
			"§12.4: the refusal names the next actionable step")
	})

	t.Run("a directory no round has been opened in", func(t *testing.T) {
		require.NoError(t, l.EnsurePR("acme", "web", 42))

		recorded, err := l.Briefed("acme", "web", 42)
		var missing *NotBriefedError
		require.ErrorAs(t, err, &missing)
		assert.Zero(t, recorded)
		assert.Equal(t, 0, missing.Round, "§9.3.3 numbers rounds from 1, so 0 is no round")
		assert.NoError(t, missing.Unwrap(),
			"meta.json read cleanly, so there is no read failure to report")
		assert.Contains(t, missing.Error(), "cr brief 42 --repo acme/web")
	})

	t.Run("a briefed pull request", func(t *testing.T) {
		held, err := l.LockPR("acme", "web", 42)
		require.NoError(t, err)
		require.NoError(t, held.WriteMeta(&Meta{
			Owner: "acme", Repo: "web", PR: 42, Round: 1, Head: "0f1e2d3",
		}))
		require.NoError(t, held.Unlock())

		recorded, err := l.Briefed("acme", "web", 42)
		require.NoError(t, err)
		assert.Equal(t, 1, recorded.Round)
		assert.Equal(t, "0f1e2d3", recorded.Head)
	})
}
