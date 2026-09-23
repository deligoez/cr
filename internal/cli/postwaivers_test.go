package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// §7.2 and §7.4.1 through `cr post --confirm` with no `cr draft` between the
// reviewer's edit and the send: each discard verb writes the waiver a redraft
// would have written, in the scope its disposition names, and the record it
// discards is not posted.
//
// Each verb runs in a home of its own, with the discarded record anchored in a
// file of its own, so the waiver found can only be the one that verb wrote.
func TestPostConfirmWaivesTheDraftsDiscardsWithoutARedraft(t *testing.T) {
	for _, verb := range []struct {
		name        string
		edit        func(t *testing.T, file string) string
		disposition finding.Disposition
		scope       string
	}{
		{
			name:        "block deleted",
			edit:        func(t *testing.T, file string) string { return deleteBlock(t, file, "f2") },
			disposition: finding.DispositionNotHere,
			scope:       "pull request",
		},
		{
			name: "marked wrong",
			edit: func(t *testing.T, file string) string {
				return markerEdit(t, file, "f2", `disposition=""`, `disposition="wrong"`)
			},
			disposition: finding.DispositionWrong,
			scope:       "repository",
		},
	} {
		t.Run(verb.name, func(t *testing.T) {
			kept, discarded := aCitedRecord("f1"), aCitedRecord("f2")
			discarded.Anchor.Path = "internal/api/discarded.go"
			layout := draftedHome(t, kept, discarded)
			redraft(t)
			require.NoError(t, os.WriteFile(
				layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft),
				[]byte(verb.edit(t, readDraft(t, layout))), 0o600))
			shim := ghShimming(t, builtPayload(t))
			require.Empty(t, waiverScopeOf(t, layout, discarded.Anchor.Path),
				"§8.5.1's dry run writes no waiver")

			_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
			require.NoError(t, err)

			require.Len(t, shim.writes(t), 1)
			assert.Equal(t, verb.scope, waiverScopeOf(t, layout, discarded.Anchor.Path),
				"§7.4.1: the discard's waiver is in the scope its disposition names")
			assert.Empty(t, waiverScopeOf(t, layout, kept.Anchor.Path),
				"the record that was posted is waived nowhere")
			stored, err := state.ReadStamped[finding.Finding](
				layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
			require.NoError(t, err)
			require.Len(t, stored, 2)
			assert.Equal(t, finding.StatePosted, stored[0].State)
			assert.Equal(t, finding.StateDiscarded, stored[1].State)
			assert.Equal(t, verb.disposition, stored[1].Disposition)
		})
	}
}

// §6.1's `thread_id` row, written by cr: once `cr post --confirm` has read the
// returned threads back, every posted record in findings.ndjson carries the id
// of the thread its comment became, the same id posted.json keys by it.
func TestPostConfirmStoresEachPostedRecordsThreadID(t *testing.T) {
	first, second := aCitedRecord("f1"), aCitedRecord("f2")
	second.Anchor.Path = "internal/api/second.go"
	layout := draftedHome(t, first, second)
	redraft(t)
	ghShimming(t, builtPayload(t))

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	stored, err := state.ReadStamped[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
	require.NoError(t, err)
	threads := make(map[string]string, len(stored))
	for i := range stored {
		assert.Equal(t, finding.StatePosted, stored[i].State, stored[i].ID)
		threads[stored[i].ID] = stored[i].ThreadID
	}
	assert.Equal(t, map[string]string{"f1": threadIDFor(0), "f2": threadIDFor(1)}, threads,
		"§6.1: each posted record carries the thread its comment became")
}

// §7.2's `kind` row through `cr post --confirm`: a question the reviewer
// hardened into a finding is stored as the finding the author received, so
// §9.5.5 refuses `answered` on it. Measured before the fix on
// deligoez/cr-qa#25: f1701 posted as a finding stayed a question in
// findings.ndjson, and `cr verify … answered` then exited 0.
func TestAPostedRecordHoldsTheKindItWasPostedAs(t *testing.T) {
	asked := aCitedRecord("f1")
	asked.Kind = finding.KindQuestion
	asked.Summary = "Does the caller ever see the error Decode returns?"
	layout := draftedHome(t, asked)
	redraft(t)
	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f1", `kind="question"`, `kind="finding"`))
	ghShimming(t, builtPayload(t))

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	stored, err := state.ReadStamped[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, finding.StatePosted, stored[0].State)
	assert.Equal(t, finding.KindFinding, stored[0].Kind, "the register the author received")
}
