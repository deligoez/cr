package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/post"
)

// §8.4.4 against §8.4.2, told apart by what GitHub's answer states rather than
// by whether cr can read a message in it.
//
// Each response is written the way gh 2.97.0 reported it when a local server
// answered `gh api` with that response: the document on standard output, the
// status on standard error, and exit 1. A timeout and a server error each carry
// a JSON message a reader could mistake for a refusal, and neither says whether
// the review was created, so the round is left unresolved, the run exits 4
// naming `cr post --reconcile`, and the next `--confirm` sends nothing. A 422
// carrying GitHub's validation errors does say so: the round stays postable, and
// the next `--confirm` reaches GitHub again and is refused again.
//
// The count of calls carrying a request body is the assertion that matters,
// because a second send after an unknown outcome is the double post §8.4.4
// calls the worse failure.
func TestAServerErrorOrATimeoutIsAnUnknownOutcome(t *testing.T) {
	const reconcileHint = "run `cr post <pr> --reconcile`, which adopts the review the call created or " +
		"clears post_unresolved for a retry; a second `cr post --confirm` is refused until then"
	for _, row := range []struct {
		name       string
		stdout     string
		stderr     string
		unresolved bool
	}{
		{
			name: "a 504 timeout body",
			stdout: `{"message":"We couldn't respond to your request in time. Sorry about that. ` +
				`Please try resubmitting your request and contact us if the problem persists.",` +
				`"documentation_url":"https://docs.github.com/rest","status":"504"}`,
			stderr: "gh: We couldn't respond to your request in time. Sorry about that. Please try " +
				"resubmitting your request and contact us if the problem persists. (HTTP 504)",
			unresolved: true,
		},
		{
			name:       "a 502 server error body",
			stdout:     `{"message":"Server Error"}`,
			stderr:     "gh: Server Error (HTTP 502)",
			unresolved: true,
		},
		{
			name:       "a 502 body that is not JSON",
			stdout:     "<html><body><h1>502 Bad Gateway</h1></body></html>",
			stderr:     "gh: HTTP 502",
			unresolved: true,
		},
		{
			name: "a 422 validation body",
			stdout: `{"message":"Validation Failed","errors":[{"resource":"PullRequestReviewComment",` +
				`"field":"line","code":"custom","message":"line must be part of the diff"}],` +
				`"documentation_url":"https://docs.github.com/rest/pulls/reviews","status":"422"}`,
			stderr:     "gh: Validation Failed (HTTP 422)",
			unresolved: false,
		},
		{
			name: "a 422 whose document names no status",
			stdout: `{"message":"Validation Failed","errors":[{"resource":"PullRequestReviewComment",` +
				`"field":"line","code":"custom","message":"line must be part of the diff"}]}`,
			stderr:     "gh: Validation Failed (HTTP 422)",
			unresolved: false,
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
			redraft(t)
			shim := ghShimming(t, builtPayload(t))
			answer := t.TempDir()
			stdout := filepath.Join(answer, "stdout")
			stderr := filepath.Join(answer, "stderr")
			require.NoError(t, os.WriteFile(stdout, []byte(row.stdout+"\n"), 0o600))
			require.NoError(t, os.WriteFile(stderr, []byte(row.stderr+"\n"), 0o600))
			ghWrapping(t, shim, "case \"$*\" in\n  *'--method POST'*)\n"+
				"    printf '%s\\n' \"$*\" >> '"+shim.path+"'; cat '"+stdout+"'; cat '"+stderr+"' >&2; exit 1 ;;\nesac")

			_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")

			require.Error(t, err)
			assert.Equal(t, ExitState, exitCodeFor(err), "§11.2 codes a partial post and a rejection 4")
			stored, readErr := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
			require.NoError(t, readErr)
			assert.Equal(t, row.unresolved, stored.PostUnresolved)
			var unknown *UnknownOutcomeError
			var rejected *post.RejectedError
			if row.unresolved {
				require.ErrorAs(t, err, &unknown, "§8.4.4: the answer does not say whether the review exists")
				assert.False(t, errors.As(err, &rejected), "§8.4.2 is not asserted over an unknown outcome")
				assert.Equal(t, reconcileHint, hintFor(err))
			} else {
				require.ErrorAs(t, err, &rejected, "§8.4.2: GitHub said the review was not created")
				assert.False(t, errors.As(err, &unknown))
			}
			require.Len(t, shim.writes(t), 1)

			_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")

			require.Error(t, err, "the following --confirm is refused")
			assert.Equal(t, ExitState, exitCodeFor(err))
			var refusedAgain *UnresolvedPostError
			if row.unresolved {
				require.ErrorAs(t, err, &refusedAgain, "§8.4.4 refuses a second send while the outcome is unknown")
				assert.Len(t, shim.writes(t), 1, "§8.4.4: nothing is sent twice")
			} else {
				assert.False(t, errors.As(err, &refusedAgain), "a rejected round is still postable")
				require.ErrorAs(t, err, &rejected, "the retry reached GitHub and was refused again")
				assert.Len(t, shim.writes(t), 2, "§8.4.2: the rejected round was sent again")
			}
		})
	}
}
