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

