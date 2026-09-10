package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// §8.1.3 through the command: a record whose body carries a `<!-- cr:`
// sequence is rejected with exit code 1 naming the record, and the refusal
// writes nothing.
//
// Nothing written is the half a unit test cannot show. `cr draft` settles the
// round's states and writes the draft under one lock, so a refusal reached
// after the states were stamped would leave findings.ndjson claiming a draft
// that does not exist.
func TestDraftRejectsABodyCarryingTheReservedSequence(t *testing.T) {
	carrying := aStoredRecord("f2", finding.StateDraft)
	carrying.Summary = "The error is dropped. " + render.Reserved + "label -->"
	layout := draftedHome(t, aStoredRecord("f1", finding.StateDraft), carrying)

	_, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§8.1.3 rejects with exit code 1")
	assert.Contains(t, err.Error(), "f2", "the refusal names the record")

	_, statErr := os.Stat(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "no draft is written")

	stored, err := state.ReadRecords[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err)
	for i := range stored {
		assert.Equal(t, finding.StateDraft, stored[i].State,
			"%s stays in draft, since no block was rendered for it", stored[i].ID)
	}
}
