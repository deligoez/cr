package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/render"
)

// recordedGH installs a `gh` that answers nothing and remembers what it was
// asked, so a run can be held to what it did and did not send.
//
// It answers with an error rather than with a payload, which is deliberate: a
// run that needed GitHub would fail loudly here instead of passing on a canned
// answer, and the assertion below would then be measuring a stub rather than
// the command.
func recordedGH(t *testing.T) *[]string {
	t.Helper()
	calls := &[]string{}
	restore := ghClient
	ghClient = func() gh.Client {
		return gh.WithRunner(func(args ...string) (string, error) {
			asked := strings.Join(args, " ")
			*calls = append(*calls, asked)
			return "", fmt.Errorf("no gh answer was prepared for %q", asked)
		})
	}
	t.Cleanup(func() { ghClient = restore })
	return calls
}

// dryRun is the document §8.5.1's run prints.
type dryRun struct {
	Round   int          `json:"round"`
	Payload *post.Review `json:"payload"`
	Posted  bool         `json:"posted"`
}

// §8.5.1: `cr post` without `--confirm` validates, prints the full payload,
// reports `"posted": false`, and exits 0 without sending anything.
//
// The payload is asserted to be the payload and not a summary of it. The
// printed body is the one §8.4.3 embeds the hash in, and the hash it embeds is
// recomputed here from the printed comments — so a document that had dropped a
// comment, reordered them, or rendered a body other than the one that would be
// sent could not agree with its own hash. That is the whole of what a reviewer
// about to type `--confirm` is deciding on.
//
// The no-write half is measured at the seam rather than asserted about the
// code: every invocation the run made is remembered, and none of them is the
// review-creation call, which §8.3.3 makes the one invocation carrying a
// request body.
func TestADryRunPrintsThePayloadAndSendsNothing(t *testing.T) {
	draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	calls := recordedGH(t)

	printed, err := runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err, "§8.5.1: a dry run exits 0")

	var report dryRun
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	assert.False(t, report.Posted, "§12.6's field, which §8.5.1 requires of a run that sent nothing")
	assert.Contains(t, printed, `"posted": false`,
		"§8.5.1 asks for the field by name, so the document carries it spelled out")

	require.NotNil(t, report.Payload, "§8.5.1: the full payload")
	assert.Equal(t, draftRound, report.Round)
	require.Len(t, report.Payload.Comments, 2, "one comment per queued record")
	assert.Equal(t, "COMMENT", report.Payload.Event, "§8.3.2's event")

	embedded, found := render.PayloadHashIn(report.Payload.Body)
	require.True(t, found, "§8.4.3 embeds the payload hash in the review's own body")
	computed, err := report.Payload.Hash()
	require.NoError(t, err)
	assert.Equal(t, computed, embedded,
		"the printed payload is the payload the hash was taken over, comment for comment")

	for _, call := range *calls {
		assert.NotContains(t, call, "--input",
			"§8.3.3's review-creation call is the one invocation carrying a request body")
		assert.NotContains(t, call, "POST", "a dry run performs no network write")
	}
}

// §8.5.1 in a terminal: the payload a reviewer is deciding about is printed
// whole, and the run says it sent nothing.
//
// The text shape is asserted beside the document because §12.6 names both, and
// because the reviewer who gives `--confirm` is reading this one. A run that
// carried the payload faithfully in JSON and printed a summary to the terminal
// would satisfy the test above and show the deciding human nothing.
func TestTheDryRunPrintsThePayloadToATerminalToo(t *testing.T) {
	draftedHome(t, aCitedRecord("f1"))
	redraft(t)

	// `--no-color` because the comment count is accented in a terminal, and
	// what is asserted is the payload rather than the escape codes around
	// it. §12.1 leaves the shape alone either way.
	printed := throughATerminal(t, "post", draftPR, "--repo", draftSlug, "--no-color")

	assert.Contains(t, printed, "not posted: --confirm was not given, and nothing was sent to GitHub")
	assert.Contains(t, printed, render.Reserved+"payload-hash ",
		"§8.4.3's body reaches the reviewer, and this is the only command that prints it")
	assert.Contains(t, printed, "internal/api/handler.go:44 RIGHT",
		"every comment is printed with the position it would land at")
}

// §11.2 and §8.5.1 together: validation runs before the gate, so a payload that
// fails it exits 1 whichever way the flag was given.
//
// The record is refused by §6.3 and §8.1.5 acting in order — it is stored
// `cited` with no citation, so the recomputation reaches `argued`, §6.3.1
// forces it to a question, and a question body holding no "?" is one §8.1.5
// will not post. Nothing about that depends on the flag, and this is what says
// so: the same state answers the same way with `--confirm` and without it.
//
// The negative half matters as much. A build that refused `--confirm` before
// reading anything would pass an assertion about exit codes while meaning
// something else entirely, so the confirmed run is checked not to have been
// stopped by the flag's own refusal.
func TestAnInvalidPayloadExitsOneUnderBothFlagSettings(t *testing.T) {
	draftedHome(t, aStoredRecord("f1", finding.StateDraft))
	redraft(t)

	for _, flags := range [][]string{{}, {"--confirm"}} {
		name := "without --confirm"
		if len(flags) > 0 {
			name = "with --confirm"
		}
		t.Run(name, func(t *testing.T) {
			_, err := runPost(t, append([]string{draftPR, "--repo", draftSlug}, flags...)...)

			require.Error(t, err)
			assert.Equal(t, ExitValidation, exitCodeFor(err),
				"§11.2 codes an invalid payload 1, and the gate does not move that")
			var unbuilt *notImplementedError
			assert.NotErrorAs(t, err, &unbuilt,
				"the validation ran: what refused is the payload and not the unbuilt write")
		})
	}
}
