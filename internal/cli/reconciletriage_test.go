package cli

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// triagedRound puts a fresh state root behind CR_HOME holding four records
// drafted into one round, with the draft triaged by each of §7.2's verbs that
// §7.3.1 draws an outcome from: f1 left as it is, f2 softened to a question, f3
// deleted, and f4 marked wrong. The two records that reach the payload are
// anchored in the two files the stand-in diff carries.
func triagedRound(t *testing.T) state.Layout {
	t.Helper()
	records := make([]*finding.Finding, 0, 4)
	for _, path := range []string{
		"internal/api/handler.go", "internal/api/second.go",
		"internal/api/deleted.go", "internal/api/wrong.go",
	} {
		record := aCitedRecord("f" + strconv.Itoa(len(records)+1))
		record.Anchor.Path = path
		records = append(records, record)
	}
	// §8.1.5 posts a question only when its body asks one.
	records[1].Summary = "Does the caller ever see the error Decode returns?"
	layout := draftedHome(t, records...)
	redraft(t)
	edited := markerEdit(t, readDraft(t, layout), "f2", `kind="finding"`, `kind="question"`)
	edited = deleteBlock(t, edited, "f3")
	edited = markerEdit(t, edited, "f4", `disposition=""`, `disposition="wrong"`)
	writeDraft(t, layout, edited)
	return layout
}

// outcomeEvents are the repository's §7.3.1 outcome events, in ledger order,
// each with its moment cleared: two runs made at different times write
// different timestamps and every other field is what the comparison is about.
func outcomeEvents(t *testing.T, layout state.Layout) []finding.TriageEvent {
	t.Helper()
	events, err := finding.TriageEvents(layout, draftOwner, draftRepo)
	require.NoError(t, err)
	outcomes := make([]finding.TriageEvent, 0, len(events))
	for i := range events {
		if events[i].Action == finding.ActionRaised {
			continue
		}
		event := events[i]
		event.At = time.Time{}
		outcomes = append(outcomes, event)
	}
	return outcomes
}

// §7.3.1 against §8.4.4: a round `cr post --reconcile` adopts after a send whose
// outcome cr never learned holds the same outcome events as the identical round
// a `cr post --confirm` settled on its own, so the ledger carries one outcome
// against every raise whichever path settled the round.
//
// The two rounds are built in two state roots from the same records and the
// same draft edits, one per verb. The softening is the case that matters: it
// is not a state findings.ndjson holds, and a question in the payload may be
// §6.3.2's forcing rather than the reviewer's, so the adoption can only write it
// from what the send named before its call.
func TestReconcileWritesTheOutcomeEventsAConfirmedSendWrites(t *testing.T) {
	confirmed := triagedRound(t)
	accepting := ghShimming(t, builtPayload(t))
	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)
	require.Len(t, accepting.writes(t), 1)
	sent := outcomeEvents(t, confirmed)
	named := make([]string, 0, len(sent))
	for i := range sent {
		named = append(named, sent[i].Record+":"+string(sent[i].Action))
	}
	require.Equal(t, []string{
		"f1:kept", "f2:softened", "f3:discarded-not-here", "f4:discarded-wrong",
	}, named, "§7.3.1: the confirmed send settles every queued record once")

	adopted := triagedRound(t)
	shim := reconcilingShim(t, builtPayload(t))
	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.Error(t, err, "§8.4.4: an outcome cr could not establish is not a success")
	require.Empty(t, outcomeEvents(t, adopted), "an unknown outcome settles nothing")

	printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)
	report := reconcileReport(t, printed)
	require.Equal(t, adoptedReviewURL, report.Adopted)
	require.Equal(t, []string{"f1", "f2"}, report.Records)
	assert.Len(t, shim.writes(t), 1, "§8.4.4: the reconciliation reads and never sends")

	assert.Equal(t, sent, outcomeEvents(t, adopted),
		"§7.3.1: the adoption writes the outcome events the confirmed send writes")
}
