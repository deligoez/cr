package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// failingPost makes the review-creation call of the shim on PATH answer the
// way gh reports a 502: a document on standard output, the status on standard
// error, and exit 1. It is §8.4.4's unknown outcome.
func failingPost(t *testing.T, shim *ghShimTranscript) {
	t.Helper()
	answer := t.TempDir()
	stdout := filepath.Join(answer, "stdout")
	require.NoError(t, os.WriteFile(stdout, []byte(`{"message":"Server Error"}`+"\n"), 0o600))
	ghWrapping(t, shim, "case \"$*\" in\n  *'--method POST'*)\n"+
		"    printf '%s\\n' \"$*\" >> '"+shim.path+"'; cat '"+stdout+"'; echo 'gh: Server Error (HTTP 502)' >&2; exit 1 ;;\nesac")
}

// postHonesty reads the honesty channel of one JSON document cr printed.
func postHonesty(t *testing.T, printed string) []string {
	t.Helper()
	var report struct {
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report.Honesty
}

// unresolvedSentence is the disclosure a round carrying post_unresolved is
// reported with, for the pull request named.
func unresolvedSentence(round int, pr, slug string) string {
	return "§8.4.4: round " + strconv.Itoa(round) + " carries post_unresolved: the outcome of its " +
		"review-creation call is unknown, so its review may already be on the pull request, and no send " +
		"happens until `cr post " + pr + " --reconcile --repo " + slug + "` adopts that review " +
		"or clears the flag for a retry"
}

// While a round carries post_unresolved, `cr status` says so with the command
// that settles it, right after the head comparison.
//
// Measured on release QA before the fix (D-S09-2): after a 502 on the send,
// `cr status 1` exited 0 and mentioned the unresolved post nowhere, JSON or
// terminal.
func TestStatusReportsAnUnresolvedPost(t *testing.T) {
	statusHome(t)
	want := unresolvedSentence(1, fixturePR, fixtureSlug)
	status, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	assert.NotContains(t, postHonesty(t, status), want, "the control: a settled round says nothing of a send")

	rewriteStatusMeta(t, func(m *state.Meta) { m.PostUnresolved = true })
	status, err = runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	honesty := postHonesty(t, status)
	require.GreaterOrEqual(t, len(honesty), 2)
	assert.Equal(t, want, honesty[1])
	assert.Contains(t, throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--quiet"), want,
		"the terminal prints it, --quiet or not")
}

// While a round carries post_unresolved, a `cr post` dry run says so before
// anything else, and before the payload it still prints, so nobody reads that
// payload as one a send could go out with.
//
// Measured on release QA before the fix (D-S09-2): after a 502 on the send,
// `cr post 1` exited 0 and printed the full payload with `posted: false` and no
// note that a review of it may already exist.
func TestTheDryRunReportsAnUnresolvedPostFirst(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	shim := ghShimming(t, builtPayload(t))
	want := unresolvedSentence(draftRound, draftPR, draftSlug)

	printed, err := runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	assert.Equal(t, []string{}, postHonesty(t, printed), "the control: a settled round has nothing to say")

	failingPost(t, shim)
	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.Error(t, err)
	meta, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.True(t, meta.PostUnresolved, "the fixture is only worth anything with the flag set")

	printed, err = runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err, "§8.5.1: the dry run still validates, prints and exits 0")
	assert.Equal(t, []string{want}, postHonesty(t, printed))

	shown := throughATerminal(t, "post", draftPR, "--repo", draftSlug, "--no-color")
	first, _, found := strings.Cut(shown, "\n")
	require.True(t, found)
	assert.Equal(t, want, strings.TrimSuffix(first, "\r"), "the terminal says it before anything else")

	assert.Len(t, shim.writes(t), 1, "none of the dry runs sent anything")
}

// `cr post --reconcile --confirm` is refused as a usage error naming both
// flags, before anything is read or sent.
//
// Measured on release QA before the fix (D-S09-3): the pair exited 0, ran the
// reconciliation alone, and dropped --confirm without a word.
func TestReconcileWithConfirmIsRefusedAsUsage(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"))
	redraft(t)
	shim := ghShimming(t, builtPayload(t))
	before := stateFiles(t, layout)

	for _, args := range [][]string{{"--reconcile", "--confirm"}, {"--confirm", "--reconcile"}} {
		_, err := runPost(t, append([]string{draftPR, "--repo", draftSlug}, args...)...)

		var refused *ReconcileWithConfirmError
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, ExitUsage, exitCodeFor(err))
		assert.Equal(t, "--reconcile and --confirm cannot be given together: --reconcile settles an unknown "+
			"posting outcome by reading the pull request's reviews and sends nothing, and --confirm "+
			"is the permission for a send", err.Error())
		assert.Equal(t, "run `cr post <pr> --reconcile` alone to settle the round, then "+
			"`cr post <pr> --confirm` if its review is still to be sent", hintFor(err))
	}
	assert.Empty(t, shim.calls(t), "the refusal asks gh nothing")
	assert.Equal(t, before, stateFiles(t, layout), "and writes nothing")
}

// A comment spanning several lines is named by its whole range where §8.4.2's
// rejection reports it through `cr post --confirm`.
//
// Measured on release QA before the fix (D-S09-4): the rejection line for a
// comment on 33-35 read `f1701 src/Order.php:35: …`.
func TestARejectedCommentIsNamedByItsWholeRange(t *testing.T) {
	draftedHome(t, aCitedRecord("f1"))
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
	assert.Equal(t,
		"§8.4.2: GitHub rejected the review, so nothing was posted: Validation Failed (HTTP 422)\n"+
			"  f1 internal/api/handler.go:42-44: line: line must be part of the diff",
		err.Error())
}
