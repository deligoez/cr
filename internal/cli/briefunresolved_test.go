package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/post"
)

// briefingShim installs a `gh` on PATH that does what reconcilingShim does and
// also answers §3.7.1's pull request query with head and base, so one PATH
// carries an unknown-outcome send, a `cr brief` on a moved head, and a
// reconciliation.
func briefingShim(t *testing.T, review *post.Review, head, base string) *ghShimTranscript {
	t.Helper()
	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript")
	threads := filepath.Join(dir, "threads.json")
	reviews := filepath.Join(dir, "reviews.json")
	pr := filepath.Join(dir, "pr.json")

	require.NoError(t, os.WriteFile(threads, threadsPage(t, review), 0o600))
	page, err := json.Marshal(map[string]any{"data": map[string]any{
		"repository": map[string]any{"pullRequest": map[string]any{
			"reviews": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
				"nodes": []map[string]string{
					{"id": "PRR_ours", "url": adoptedReviewURL, "body": review.Body},
				},
			},
		}},
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(reviews, page, 0o600))
	require.NoError(t, os.WriteFile(pr, []byte(`{"data":{"repository":{"pullRequest":{`+
		`"number":`+draftPR+`,"title":"`+fixtureIssue+` retry the upload",`+
		`"body":"Closes `+fixtureIssue+`.","headRefName":"`+fixtureHeadBranch+`",`+
		`"headRefOid":"`+head+`","baseRefName":"main","baseRefOid":"`+base+`"}}}}`), 0o600))

	shim := filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(shim, []byte(
		"#!/bin/sh\n"+
			"printf '%s\\n' \"$*\" >> "+transcript+"\n"+
			"case \"$*\" in\n"+
			"  *reviewThreads*) exec cat "+threads+" ;;\n"+
			"  *'reviews('*) exec cat "+reviews+" ;;\n"+
			"  *'--method POST'*) exit 1 ;;\n"+
			"  *headRefOid*) exec cat "+pr+" ;;\n"+
			"  *) echo '{}' ;;\n"+
			"esac\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &ghShimTranscript{path: transcript}
}

// aCheckoutPastTheRound is a repository whose head branch holds one commit past
// main, standing in for the head the pull request moved to after draftedHome's
// round was sent. It returns that head and the base.
func aCheckoutPastTheRound(t *testing.T) (head, base string) {
	t.Helper()
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.go"), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("package lib\n\nfunc Load() {}\n")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("package lib\n\nfunc Load() {\n\tpanic(\"moved\")\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the push after the send")
	head = strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base = strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))
	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	return head, base
}

// §8.4.4 across §9.3.3's increment: a confirmed send carrying f1 and f2 meets a
// `gh` that exits 1 on the review-creation call, so round 2 carries
// post_unresolved, and the pull request's head then moves.
//
// Measured before unresolved-post-survives-new-round, over this fixture: `cr
// brief` exited 0 and opened round 3 with post_unresolved still set, f1 and f2
// were stale in round 2, and `cr post --reconcile` read round 3's posted.json,
// which does not exist, and exited 3 — leaving the flag set, a second `cr post
// --confirm` refused, and the review the shim lists never adopted. §9.1 lists no
// move out of `stale`, so no reconciliation of round 2 could have adopted it
// either.
//
// `cr brief` now refuses the increment before it writes anything, as `cr post
// --confirm` and `cr draft` refuse the same round: exit 4 and the hint naming
// the reconciliation, which §9.3.2 exempts from the moved-head refusal. That
// reconciliation adopts the review, and the next brief opens round 3 with the
// flag cleared and f1 and f2 left posted rather than stale. One review-creation
// call reaches `gh` across the whole run.
func TestABriefOnAMovedHeadIsRefusedWhileAPostIsUnresolved(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	payload := builtPayload(t)
	head, base := aCheckoutPastTheRound(t)
	shim := briefingShim(t, payload, head, base)
	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.ErrorAs(t, err, new(*UnknownOutcomeError))
	require.Len(t, shim.writes(t), 1)

	movedHead(t, head)
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte("Retry on 5xx.\n"), 0o600))
	briefing := []string{"brief", draftPR, "--repo", draftSlug, "--issue", fixtureIssue, "--intent-file", issue}
	before := stateFiles(t, layout)

	_, err = runCLIPrinting(t, briefing...)

	var refused *UnresolvedPostError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, UnresolvedPostError{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum, Round: draftRound,
	}, *refused)
	assert.Equal(t,
		"the head moved from "+draftHead+" to "+head+", and §9.3.4 would move round 2's "+
			"queued records to stale while a send carrying them has an outcome cr never learned: "+
			refused.Error(),
		err.Error())
	assert.Equal(t, ExitState, exitCodeFor(err))
	assert.Equal(t,
		"run `cr post <pr> --reconcile` to adopt the review the earlier call created, "+
			"or to clear post_unresolved for a retry",
		hintFor(err))
	assert.Equal(t, before, stateFiles(t, layout),
		"the refusal writes nothing: no round is opened and no record is staled")

	printed, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err, "§9.3.2 exempts the reconciliation from the moved head")
	report := reconcileReport(t, printed)
	assert.Equal(t, adoptedReviewURL, report.Adopted)
	assert.Equal(t, []string{"f1", "f2"}, report.Records)
	assert.False(t, report.Unresolved)

	_, err = runCLIPrinting(t, briefing...)
	require.NoError(t, err, "a settled posting no longer refuses the increment")
	opened, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	assert.Equal(t, draftRound+1, opened.Round)
	assert.Equal(t, head, opened.Head)
	assert.False(t, opened.PostUnresolved)
	stored := roundRecordsByID(t, layout)
	assert.Equal(t, finding.StatePosted, stored["f1"].State)
	assert.Equal(t, finding.StatePosted, stored["f2"].State)
	assert.Len(t, shim.writes(t), 1, "§8.4.4: nothing across the run sends a second review")
}
