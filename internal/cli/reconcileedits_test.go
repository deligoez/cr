package cli

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// editedRound puts a fresh state root behind CR_HOME holding two records drafted
// into one round at head, with the draft edited by §7.2's two rows that move a
// stored field: f1's severity lowered and its anchor moved one line down. f2 is
// stored as a finding graded `cited` with no citation behind it, so §7.2.2's
// recomputation at post time lowers it to `argued` and §6.3.1 forces it to a
// question, neither of which `cr draft` does.
func editedRound(t *testing.T, head string) state.Layout {
	t.Helper()
	edited, regraded := aCitedRecord("f1"), aStoredRecord("f2", finding.StateDraft)
	regraded.Anchor.Path = "internal/api/second.go"
	// §8.1.5 posts a question only when its body asks one.
	regraded.Summary = "Does the caller ever see the error Decode returns?"
	layout := draftedAgainst(t, head, edited, regraded)
	redraft(t)
	draft := markerEdit(t, readDraft(t, layout), "f1", `severity="high"`, `severity="low"`)
	// The whole pair is replaced, for the reason markeredits_test.go gives.
	draft = markerEdit(t, draft, "f1", `start_line="42" line="44"`, `start_line="43" line="45"`)
	writeDraft(t, layout, draft)
	return layout
}

// §8.4.4 against §7.2 and §7.2.2: a round `cr post --reconcile` adopts after a
// send whose outcome cr never learned stores every adopted record as the
// identical round a successful `cr post --confirm` stores it — the severity and
// anchor the draft moved, the grade the post-time recomputation reached and the
// register the forcing left —
// and writes outcome events carrying those same values.
//
// The send applies those edits in memory before it builds the payload and
// stores them only once the call has succeeded, so an adoption reading
// findings.ndjson alone would store the values the round held before the send.
func TestReconcileStoresTheRecordsAConfirmedSendStores(t *testing.T) {
	lines := make([]string, 0, 60)
	for n := 1; n <= 60; n++ {
		lines = append(lines, fmt.Sprintf("// line %d", n))
	}
	dir, head := anchoredCheckout(t, "internal/api/handler.go", lines...)
	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })

	confirmed := editedRound(t, head)
	accepting := ghShimming(t, builtPayload(t))
	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)
	require.Len(t, accepting.writes(t), 1)
	sentRecords, sentEvents := roundRecordsByID(t, confirmed), outcomeEvents(t, confirmed)
	require.Len(t, sentRecords, 2)
	hash, err := finding.AnchorContentHash([]string{"// line 43", "// line 44", "// line 45"})
	require.NoError(t, err)
	require.Equal(t, finding.SeverityLow, sentRecords["f1"].Severity, "§7.2: the severity row is stored")
	require.Equal(t, 45, sentRecords["f1"].Anchor.Line, "§7.2: the location row is stored")
	require.Equal(t, hash, sentRecords["f1"].Anchor.ContentHash)
	require.Equal(t, finding.GradeArgued, sentRecords["f2"].Grade, "§7.2.2: the recomputed grade is stored")
	require.Equal(t, finding.KindQuestion, sentRecords["f2"].Kind, "§6.3.1: the forcing is stored")

	adopted := editedRound(t, head)
	shim := reconcilingShim(t, builtPayload(t))
	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.Error(t, err, "§8.4.4: an outcome cr could not establish is not a success")
	printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)
	require.Equal(t, []string{"f1", "f2"}, reconcileReport(t, printed).Records)
	assert.Len(t, shim.writes(t), 1, "§8.4.4: the reconciliation reads and never sends")

	assert.Equal(t, sentRecords, roundRecordsByID(t, adopted),
		"§8.4.4: the adoption stores each record as the confirmed send stores it")
	assert.Equal(t, sentEvents, outcomeEvents(t, adopted),
		"§7.3.1: the adoption's outcome events carry the values the confirmed send's carry")
}
