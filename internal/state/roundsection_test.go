package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §10.3 has `cr merge`, `cr draft` and `cr post` each accumulate their own
// counts into one summary.json, so a writer owns some of its fields and must
// leave the rest exactly as it found them.
//
// The field written second is what makes that provable: a writer that decoded
// the document into a struct of its own fields would re-encode it without the
// first, and the round's history would lose whatever the writer before it put
// there. Every write here is checked against the whole document rather than
// against its own key.
func TestARoundSectionLeavesEveryOtherFieldAlone(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)

	require.NoError(t, UpdateRoundSection(held, 1, FileSummary, "waived", 3))
	require.NoError(t, UpdateRoundSection(held, 1, FileSummary, "drafted", []string{"f1", "f2"}))
	require.NoError(t, held.Unlock())

	body, err := l.ReadRound("acme", "web", 42, 1, FileSummary)
	require.NoError(t, err)
	assert.JSONEq(t, `{"waived":3,"drafted":["f1","f2"]}`, string(body))
}

// A section written twice is replaced rather than accumulated, so regenerating
// a draft cannot inflate a count §10.3 says the round records once.
func TestARoundSectionIsReplacedNotAccumulated(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)

	require.NoError(t, UpdateRoundSection(held, 1, FileSummary, "waived", 3))
	require.NoError(t, UpdateRoundSection(held, 1, FileSummary, "waived", 5))
	require.NoError(t, held.Unlock())

	body, err := l.ReadRound("acme", "web", 42, 1, FileSummary)
	require.NoError(t, err)
	assert.JSONEq(t, `{"waived":5}`, string(body))
}

// A document holding `null` is a document with nothing in it rather than a
// document that cannot be written to.
//
// It is not hypothetical: `null` is valid JSON, EnsureRound leaves an artefact
// it finds exactly as it is, and a decode of it yields a nil map — which a
// writer assigning into it would panic on rather than report.
func TestARoundSectionSurvivesANullDocument(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.WriteRound(1, FileSummary, []byte("null\n")))

	require.NoError(t, UpdateRoundSection(held, 1, FileSummary, "waived", 3))
	require.NoError(t, held.Unlock())

	body, err := l.ReadRound("acme", "web", 42, 1, FileSummary)
	require.NoError(t, err)
	assert.JSONEq(t, `{"waived":3}`, string(body))
}

// A value that cannot be encoded is reported naming the section and the file,
// and nothing is written: the caller learns which count it failed to record
// rather than finding the artefact half-updated.
func TestAnUnencodableRoundSectionIsReported(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, UpdateRoundSection(held, 1, FileSummary, "waived", 3))

	err = UpdateRoundSection(held, 1, FileSummary, "forced_to_question", make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forced_to_question")
	assert.Contains(t, err.Error(), FileSummary)
	require.NoError(t, held.Unlock())

	body, readErr := l.ReadRound("acme", "web", 42, 1, FileSummary)
	require.NoError(t, readErr)
	var summary map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &summary))
	assert.NotContains(t, summary, "forced_to_question")
	assert.Equal(t, "3", string(summary["waived"]), "the section already there is untouched")
}

// A file §2.3 gives a round no such artefact is refused, so a section cannot be
// aimed at a document the table does not name.
func TestARoundSectionRefusesAnArtefactSection23DoesNotName(t *testing.T) {
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)

	err = UpdateRoundSection(held, 1, "counts.json", "waived", 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "counts.json")
	require.NoError(t, held.Unlock())
}
