package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// ghWrapping installs a `gh` in front of the one already on PATH that runs
// script before handing every invocation on to it unchanged.
//
// It is how a test reaches the moment between §8.3's call and what cr writes
// after it: the inner shim still records the call and answers it, and the
// script acts on cr's state while the call is in flight.
func ghWrapping(t *testing.T, inner *ghShimTranscript, script string) {
	t.Helper()
	dir := t.TempDir()
	wrapped := filepath.Join(filepath.Dir(inner.path), "gh")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte(
		"#!/bin/sh\n"+script+"\nexec '"+wrapped+"' \"$@\"\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// §8.4.4's "posting twice is a worse failure than posting late", over the one
// outcome that is neither a rejection nor an unknown: GitHub created the review
// and a local write after the call failed.
//
// The shim makes the pull request's state directory read-only as the POST
// arrives, so the call succeeds and every §2.3 write into that directory after
// it fails. Before the refusal existed the records stayed `queued`, nothing was
// marked unresolved, and the second `--confirm` sent the review again. The
// count is the number of calls the shim saw carrying a request body, compared
// as a length rather than searched for in text. The run ends with the step the
// refusal names, so the round the refusal holds is shown to be recoverable.
func TestAFailedWriteAfterTheCallDoesNotLetTheRoundPostTwice(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	built := builtPayload(t)
	shim := ghShimming(t, built)
	prDir := layout.PRDir(draftOwner, draftRepo, draftPRNum)
	if os.Geteuid() == 0 {
		t.Skip("root writes through a read-only directory")
	}
	t.Cleanup(func() { _ = os.Chmod(prDir, 0o700) })
	ghWrapping(t, shim, "case \"$*\" in\n  *'--method POST'*) chmod 500 '"+prDir+"' ;;\nesac")

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.Error(t, err, "the writes after the call failed, and the run says so")
	require.Len(t, shim.writes(t), 1, "the first run created the review")

	// The disk is back, and the reviewer runs the same command again.
	require.NoError(t, os.Chmod(prDir, 0o700))
	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")

	assert.Len(t, shim.writes(t), 1, "§8.4.4: the second run sent no second review")
	var unresolved *UnresolvedPostError
	require.ErrorAs(t, err, &unresolved,
		"§8.3.3 wrote the payload before the call, so a round cr never settled is refused")
	assert.Equal(t, ExitState, exitCodeFor(err), "§11.2 codes where the round stands 4")
	assert.Contains(t, hintFor(err), "--reconcile", "§8.4.4's recovery is the next step")

	stored, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	assert.True(t, stored.PostUnresolved,
		"the refusal rests on what was written before the call, not after it")

	// §8.4.4's way forward: the review the first run created carries the
	// payload hash, so --reconcile adopts it and settles the round.
	page, err := json.Marshal(map[string]any{"data": map[string]any{
		"repository": map[string]any{"pullRequest": map[string]any{
			"reviews": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
				"nodes": []gh.Review{{
					ID: "PRR_shim", URL: "https://example.invalid/review", Body: built.Body,
					Commit: gh.ReviewCommit{OID: draftHead},
				}},
			},
		}},
	}})
	require.NoError(t, err)
	reviews := filepath.Join(t.TempDir(), "reviews.json")
	require.NoError(t, os.WriteFile(reviews, page, 0o600))
	ghWrapping(t, shim, "case \"$*\" in\n  *'reviews(first'*) exec cat '"+reviews+"' ;;\nesac")

	_, err = runPost(t, draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)
	stored, err = layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	assert.False(t, stored.PostUnresolved, "the adoption settles the round")
	records, err := state.ReadStamped[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
	require.NoError(t, err)
	require.Len(t, records, 2)
	for i := range records {
		assert.Equalf(t, finding.StatePosted, records[i].State,
			"the records sent before the failed write are adopted, and %s is not", records[i].ID)
	}
	assert.Len(t, shim.writes(t), 1, "reconciling sends nothing")
}

// §8.4.2's side of the same refusal: a call GitHub rejected marks nothing, so
// the round is still postable and a corrected second run reaches GitHub.
//
// Two calls carrying a request body is the assertion. A refusal keyed on the
// payload having been written, and not on the call's outcome, would stop the
// second run here and turn a rejection into a round nobody can post.
func TestARejectedCallLeavesTheRoundPostable(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	shim := ghShimming(t, builtPayload(t))
	rejection := filepath.Join(t.TempDir(), "rejection.json")
	require.NoError(t, os.WriteFile(rejection, []byte(
		`{"message":"Validation Failed","errors":[{"resource":"PullRequestReviewComment",`+
			`"field":"line","code":"custom","message":"line must be part of the diff"}],`+
			`"status":"422"}`+"\n"), 0o600))
	ghWrapping(t, shim, "case \"$*\" in\n  *'--method POST'*)\n"+
		"    printf '%s\\n' \"$*\" >> '"+shim.path+"'; cat '"+rejection+"'; exit 1 ;;\nesac")

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.Error(t, err)
	var unresolved *UnresolvedPostError
	require.False(t, errors.As(err, &unresolved), "a rejection is not an unresolved posting")
	stored, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	assert.False(t, stored.PostUnresolved, "§8.4.2: a rejected call leaves nothing to reconcile")

	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	assert.False(t, errors.As(err, &unresolved), "the second run is not refused as unresolved")
	assert.Len(t, shim.writes(t), 2, "the round was still postable, so the second run sent")
}
