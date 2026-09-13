package state

import (
	"errors"
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
	l := unlockedPR(t)

	t.Run("no state directory", func(t *testing.T) {
		recorded, err := l.Briefed("acme", "web", 7, unread(t))
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

		recorded, err := l.Briefed("acme", "web", 42, unread(t))
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

		recorded, err := l.Briefed("acme", "web", 42, at("0f1e2d3"))
		require.NoError(t, err)
		assert.Equal(t, 1, recorded.Round)
		assert.Equal(t, "0f1e2d3", recorded.Head)
		assert.False(t, recorded.Stale(), "the current head is the one the round recorded")
	})
}

// at answers §9.3.1's current head with head.
func at(head string) CurrentHead {
	return func() (string, error) { return head, nil }
}

// unread fails the test if it is called. A pull request no round has been
// opened on has no recorded head to compare against, so §9.3.1 has nothing to
// ask GitHub and Briefed must refuse before it asks — which is what keeps an
// unbriefed pull request from costing a network read.
func unread(t *testing.T) CurrentHead {
	t.Helper()
	return func() (string, error) {
		t.Error("§9.3.1 compares against a recorded head, and this pull request has none")
		return "", nil
	}
}

// §9.3.1's comparison comes back from the door, and §9.3.2's refusal falls out
// of it: the same round reads as current against its own head and as stale
// against another, and the refusal names both heads.
//
// Both halves are asserted on one round rather than on two fixtures, because
// what is being tested is that the two answers are the same comparison. A
// refusal that could disagree with the disclosure beside it would be exactly
// the defect §9.3 is written to stop — §11.1's stale-round report and §9.3.2's
// refusal are one fact told twice.
func TestARoundComparesAgainstTheCurrentHeadAndRefusesWhenItMoved(t *testing.T) {
	l := unlockedPR(t)
	require.NoError(t, l.EnsurePR("acme", "web", 42))
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&Meta{
		Owner: "acme", Repo: "web", PR: 42, Round: 2, Head: "0f1e2d3",
	}))
	require.NoError(t, held.Unlock())

	t.Run("the head has not moved", func(t *testing.T) {
		round, err := l.Briefed("acme", "web", 42, at("0f1e2d3"))
		require.NoError(t, err)
		assert.False(t, round.Stale())
		assert.NoError(t, round.RefuseStale(), "§9.3.2 refuses nothing while the heads agree")
		assert.Equal(t,
			"§9.3.1: round 2 was opened at head 0f1e2d3, which is still the pull request's current head",
			round.Disclosure())
	})

	t.Run("the head moved", func(t *testing.T) {
		round, err := l.Briefed("acme", "web", 42, at("9a8b7c6"))
		require.NoError(t, err)
		assert.True(t, round.Stale())
		assert.Equal(t,
			"§9.3.1: round 2 was opened at head 0f1e2d3 and the pull request's current head is 9a8b7c6",
			round.Disclosure(),
			"§9.3.1 reports both heads when they differ")

		refused := round.RefuseStale()
		var stale *StaleRoundError
		require.ErrorAs(t, refused, &stale)
		assert.Equal(t, round, stale.At)
		assert.Contains(t, refused.Error(), "0f1e2d3")
		assert.Contains(t, refused.Error(), "9a8b7c6")
		assert.Contains(t, refused.Error(), "cr brief 42 --repo acme/web",
			"§12.4: the refusal names the next actionable step, which §9.3.2 says is `cr brief`")
	})

	// §9.3.1 admits no command that skips the comparison, so a caller with
	// no way to learn the current head is refused rather than served a
	// round it would read as current.
	t.Run("no source for the current head", func(t *testing.T) {
		round, err := l.Briefed("acme", "web", 42, nil)
		var unread *NoCurrentHeadError
		require.ErrorAs(t, err, &unread)
		assert.Zero(t, round)
		assert.Equal(t, 2, unread.Round)
		assert.Contains(t, unread.Error(), "acme/web#42")
	})

	// A head cr asked for and could not get is the reader's own failure,
	// reported as it is rather than folded into a comparison.
	t.Run("the current head could not be read", func(t *testing.T) {
		wanted := errors.New("gh: no such pull request")
		round, err := l.Briefed("acme", "web", 42, func() (string, error) { return "", wanted })
		require.ErrorIs(t, err, wanted)
		assert.Zero(t, round)
	})
}
