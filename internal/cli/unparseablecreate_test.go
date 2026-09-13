package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/post"
)

// §8.4.4's "a response cr cannot parse", over a call gh reports as successful:
// the review-creation call exits 0, and its answer is not JSON, or is JSON
// naming no review. Neither says the review exists, so neither is a post. The
// round is left unresolved, the run exits 4 naming `cr post --reconcile`, and
// nothing a created review owes is written — no record is posted, no thread id,
// waiver or outcome event is stored, and the next `--confirm` sends nothing.
//
// `cr post --reconcile` then finds the review by §8.4.3's embedded hash and
// settles the round as the confirmed send would have: the records are posted,
// each carries the thread its comment became, the draft's discard is waived and
// the outcome events are written. One call carried a request body in all.
//
// The last row answers the same shim with the review the call created, so the
// shim's own answering is shown to reach cr as a success when the answer names
// the review.
func TestASuccessfulCallWhoseAnswerNamesNoReviewIsAnUnknownOutcome(t *testing.T) {
	const reconcileHint = "run `cr post <pr> --reconcile`, which adopts the review the call created or " +
		"clears post_unresolved for a retry; a second `cr post --confirm` is refused until then"
	threads := map[string]string{"f1": threadIDFor(0), "f2": threadIDFor(1)}
	for _, row := range []struct {
		name    string
		answer  string
		unknown bool
	}{
		{name: "a body that is not JSON", answer: "<html><body>OK</body></html>", unknown: true},
		{name: "a JSON body carrying no node_id", answer: `{"id":1,"state":"COMMENTED"}`, unknown: true},
		{name: "a JSON body naming the created review", answer: `{"id":1,"node_id":"` + adoptedReviewID + `"}`},
	} {
		t.Run(row.name, func(t *testing.T) {
			first, second, discarded := aCitedRecord("f1"), aCitedRecord("f2"), aCitedRecord("f3")
			second.Anchor.Path = "internal/api/second.go"
			discarded.Anchor.Path = "internal/api/discarded.go"
			layout := draftedHome(t, first, second, discarded)
			redraft(t)
			writeDraft(t, layout,
				markerEdit(t, readDraft(t, layout), "f3", `disposition=""`, `disposition="wrong"`))
			shim := reconcilingShim(t, builtPayload(t))
			answer := filepath.Join(t.TempDir(), "answer")
			require.NoError(t, os.WriteFile(answer, []byte(row.answer+"\n"), 0o600))
			ghWrapping(t, shim, "case \"$*\" in\n  *'--method POST'*)\n"+
				"    cat > /dev/null; printf '%s\\n' \"$*\" >> '"+shim.path+"'; cat '"+answer+"'; exit 0 ;;\nesac")

			_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")

			require.Len(t, shim.writes(t), 1, "§8.3.1: the round reached gh as one call")
			if !row.unknown {
				require.NoError(t, err)
				stored := roundRecordsByID(t, layout)
				for id, thread := range threads {
					assert.Equal(t, finding.StatePosted, stored[id].State, id)
					assert.Equal(t, thread, stored[id].ThreadID, id)
				}
				return
			}
			require.Error(t, err)
			var unknown *UnknownOutcomeError
			require.ErrorAs(t, err, &unknown, "§8.4.4: the answer does not say whether the review exists")
			assert.Equal(t, ExitState, exitCodeFor(err))
			assert.Equal(t, reconcileHint, hintFor(err))
			meta, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
			require.NoError(t, err)
			assert.True(t, meta.PostUnresolved, "§8.4.4 sets post_unresolved")
			for id, record := range roundRecordsByID(t, layout) {
				assert.Equal(t, finding.StateQueued, record.State, "§8.4.4 marks nothing posted: %s", id)
				assert.Empty(t, record.ThreadID, id)
			}
			_, named := readPosted(t, layout)[postedThreads]
			assert.False(t, named, "§8.3.3: no thread id is stored for a review cr cannot name")
			assert.Empty(t, waiverScopeOf(t, layout, discarded.Anchor.Path),
				"§7.4: a call that may not have created the review leaves no waiver behind")
			assert.Empty(t, outcomeEvents(t, layout), "§7.3.1: an unknown outcome settles nothing")

			_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
			var refused *UnresolvedPostError
			require.ErrorAs(t, err, &refused, "§8.4.4 refuses a second send while the outcome is unknown")
			require.Len(t, shim.writes(t), 1, "§8.4.4: nothing is sent twice")

			printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
			require.NoError(t, err)

			report := reconcileReport(t, printed)
			assert.Equal(t, adoptedReviewURL, report.Adopted)
			assert.Equal(t, []string{"f1", "f2"}, report.Records)
			assert.Len(t, shim.writes(t), 1, "§8.4.4: the reconciliation reads and never sends")
			meta, err = layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
			require.NoError(t, err)
			assert.False(t, meta.PostUnresolved)
			stored := roundRecordsByID(t, layout)
			for id, thread := range threads {
				assert.Equal(t, finding.StatePosted, stored[id].State, id)
				assert.Equal(t, thread, stored[id].ThreadID, "§6.1: %s carries the thread its comment became", id)
			}
			assert.Equal(t, finding.StateDiscarded, stored["f3"].State)
			var inPosted map[string]string
			require.NoError(t, json.Unmarshal(readPosted(t, layout)[postedThreads], &inPosted))
			assert.Equal(t, threads, inPosted)
			assert.Equal(t, "repository", waiverScopeOf(t, layout, discarded.Anchor.Path))
			settled := make([]string, 0)
			for _, event := range outcomeEvents(t, layout) {
				settled = append(settled, event.Record+":"+string(event.Action))
			}
			assert.Equal(t, []string{"f1:kept", "f2:kept", "f3:discarded-wrong"}, settled)
		})
	}
}

// §8.3.3's read-back given no review: a thread whose opening comment GitHub
// names no review for would match an empty review id if the two were compared,
// so an empty id claims no thread, while the same thread named for the review
// is claimed.
func TestAnEmptyReviewIDClaimsNoThread(t *testing.T) {
	review := &post.Review{Comments: []post.Comment{
		{Record: "f1", Path: "internal/api/handler.go", Line: 3, Side: git.Right, Body: "Is the error dropped?"},
	}}
	thread := gh.Thread{
		ID:      "PRRT_one",
		Anchor:  gh.Anchor{Path: "internal/api/handler.go", Side: git.Right, OriginalLine: 3, OriginalStartLine: 3},
		Comment: gh.Comment{Body: "Is the error dropped?"},
	}
	assert.Empty(t, threadsByRecord([]gh.Thread{thread}, review, ""))
	thread.Comment.Review = "PRR_named"
	assert.Equal(t, map[string]string{"f1": "PRRT_one"},
		threadsByRecord([]gh.Thread{thread}, review, "PRR_named"))
}
