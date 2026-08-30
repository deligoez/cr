package note

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// The pull request every test here answers against.
const (
	answerOwner = "acme"
	answerRepo  = "web"
	answerPR    = 42
)

// briefed puts one pull request's §2.3 state on disk with issueKey recorded in
// its metadata, which is what `cr brief` leaves behind and what an answer reads
// the key out of. An empty issueKey is the §3.2 fallback: a pull request none
// of that section's four sources yielded a key for.
func briefed(t *testing.T, issueKey string) state.Layout {
	t.Helper()
	l := storeRoot(t)
	require.NoError(t, l.EnsurePR(answerOwner, answerRepo, answerPR))

	held, err := l.LockPR(answerOwner, answerRepo, answerPR)
	require.NoError(t, err)
	recorded := state.Meta{Owner: answerOwner, Repo: answerRepo, PR: answerPR, IssueKey: issueKey}
	require.NoError(t, held.WriteMeta(&recorded))
	require.NoError(t, held.Unlock())
	return l
}

// §3.6.2: the answer is captured against the issue key, which the command is
// never told — it is addressed by pull request, because §11 scopes a record id
// to one, and the key is read out of that pull request's metadata.
//
// The note is read back off disk rather than the return value trusted, because
// the file is what §3.6.4 loads on every later round and every later pull
// request resolving to the same key. It is numbered in the same sequence as a
// plain note and carries the record it answers alongside the pull request that
// scopes that record.
func TestAnAnswerIsFiledUnderThePullRequestsIssueKey(t *testing.T) {
	l := briefed(t, "CR-7")
	at := time.Date(2026, 8, 30, 9, 15, 0, 0, time.UTC)

	plain, err := Append(l, "CR-7", "the deadline moved", SourceChat, answerPR, at)
	require.NoError(t, err)
	assert.Empty(t, plain.Record, "a §3.6.1 note answers no record")

	recorded, err := Answer(
		l, answerOwner, answerRepo, answerPR, "f3", "the retry is deliberate", SourceThread, at,
	)
	require.NoError(t, err)

	assert.Equal(t, "CR-7#n2", recorded.ID, "an answer is numbered in the same sequence as a note")
	assert.Equal(t, "f3", recorded.Record)
	assert.Equal(t, answerPR, recorded.PR, "§11 scopes the record id to this pull request")
	assert.Equal(t, SourceThread, recorded.Source)
	assert.Equal(t, at, recorded.RecordedAt)

	assert.Equal(t, []Note{plain, recorded}, stored(t, l, "CR-7"),
		"one store holds both, in the order they were appended")
}

// Nothing that could not become a §3.6.2 answer reaches the store, and each
// refusal names what the user has to do next.
//
// The record id is checked for §6.1's spelling and is deliberately not resolved
// against findings.ndjson, so a well-spelled id nobody allocated is accepted:
// §9.3.5 scopes those records to the current round while exempting this store
// from the scoping, and an existence check would refuse a true answer arriving
// after the next round opened.
func TestAnAnswerIsRefusedWhenItHasNowhereToGo(t *testing.T) {
	at := time.Now()

	t.Run("a record id §6.1 does not spell", func(t *testing.T) {
		l := briefed(t, "CR-7")
		for _, id := range []string{"", "3", "f0", "f03", "F3", "u3", "CR-7#n1"} {
			recorded, err := Answer(l, answerOwner, answerRepo, answerPR, id, "answered", SourceChat, at)
			var invalid *InvalidRecordIDError
			require.ErrorAs(t, err, &invalid, "%q", id)
			assert.Zero(t, recorded)
		}
		assert.NoFileExists(t, l.ContextFile("CR-7"))
	})

	t.Run("a pull request cr has never briefed", func(t *testing.T) {
		l := storeRoot(t)
		recorded, err := Answer(l, answerOwner, answerRepo, answerPR, "f3", "answered", SourceChat, at)
		var missing *NoStateError
		require.ErrorAs(t, err, &missing)
		assert.Zero(t, recorded)
		assert.Contains(t, missing.Error(), "cr brief 42", "the refusal names the way forward")
	})

	t.Run("a pull request that resolved to no issue key", func(t *testing.T) {
		l := briefed(t, "")
		recorded, err := Answer(l, answerOwner, answerRepo, answerPR, "f3", "answered", SourceChat, at)
		var unkeyed *NoIssueKeyError
		require.ErrorAs(t, err, &unkeyed)
		assert.Zero(t, recorded)
		assert.Contains(t, unkeyed.Error(), "--issue", "§3.2's first source is how the user names one")
	})

	t.Run("an answer §3.6.1 would refuse as a note", func(t *testing.T) {
		l := briefed(t, "CR-7")
		for name, source := range map[string]Source{
			"an unlisted source": Source("gossip"),
			"an absent source":   "",
		} {
			recorded, err := Answer(l, answerOwner, answerRepo, answerPR, "f3", "answered", source, at)
			require.Error(t, err, name)
			assert.Zero(t, recorded, name)
		}
		recorded, err := Answer(l, answerOwner, answerRepo, answerPR, "f3", " \t\n", SourceChat, at)
		require.Error(t, err, "an empty answer is no answer")
		assert.Zero(t, recorded)
		assert.NoFileExists(t, l.ContextFile("CR-7"))
	})

	t.Run("a well-spelled id nobody allocated", func(t *testing.T) {
		l := briefed(t, "CR-7")
		recorded, err := Answer(l, answerOwner, answerRepo, answerPR, "f9", "answered", SourceChat, at)
		require.NoError(t, err, "§3.6.2 keeps the fact rather than resolving the reference")
		assert.Equal(t, "f9", recorded.Record)
	})
}
