package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/suggestion"
)

// addFence puts a suggestion block the reviewer wrote at the end of f1's body.
func addFence(t *testing.T, drafted, replacement string) {
	t.Helper()
	body, err := os.ReadFile(drafted)
	require.NoError(t, err)
	fenced := strings.TrimRight(string(body), "\n") + "\n\n```suggestion\n" + replacement + "\n```\n"
	require.NoError(t, os.WriteFile(drafted, []byte(fenced), 0o600))
}

// removeFence deletes every suggestion block from a draft, fences included, the
// way a reviewer drops a replacement they will not offer.
func removeFence(t *testing.T, drafted string) {
	t.Helper()
	body, err := os.ReadFile(drafted)
	require.NoError(t, err)
	kept := make([]string, 0)
	inside, removed := false, 0
	for line := range strings.SplitSeq(string(body), "\n") {
		switch {
		case !inside && strings.TrimSpace(line) == "```suggestion":
			inside = true
			removed++
		case inside && strings.TrimSpace(line) == "```":
			inside = false
		case !inside:
			kept = append(kept, line)
		}
	}
	require.Equal(t, 1, removed, "the draft rendered the stored suggestion as one fence")
	require.NoError(t, os.WriteFile(drafted, []byte(strings.Join(kept, "\n")), 0o600))
}

// assertSuggestionRefused runs the dry run and checks §8.2.4's refusal of f1's
// suggestion.
func assertSuggestionRefused(t *testing.T, because string) {
	t.Helper()
	printed, err := runPost(t, fixturePR, "--repo", fixtureSlug)
	var refused *suggestion.RangeError
	require.ErrorAs(t, err, &refused, because)
	assert.Equal(t, "f1", refused.Record, "§8.2.4: the refusal names the record id")
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§8.2.4 codes it 1")
	assert.Empty(t, printed)
}

// §8.2.1 and §8.2.2 through `cr post`, over the suggestion the comment will
// carry rather than the one the record stores.
//
// The follow-up to QA D-S09-1's fix: draft.SentSuggestion gave §8.2.3's warning
// the fence the reviewer wrote in draft.md, and positions.go still range-checked
// the stored field alone. A fence added by hand on a LEFT anchor — which §8.2.1
// never admits, and which §8.4.1's position check lets through because the diff
// carries that base line — was sent; one added on a marker moved out of the diff
// was refused as a position rather than as the suggestion it is; and a stored
// suggestion the reviewer had deleted from the body still blocked the round.
func TestPostRangeChecksTheSuggestionItWillSend(t *testing.T) {
	t.Run("a fence added on a LEFT anchor", func(t *testing.T) {
		_, drafted := longFileRound(t, plainQuestion(git.Left, 3))
		addFence(t, drafted, "func Load() error {")

		assertSuggestionRefused(t, "§8.2.1 admits a range on the RIGHT side alone")
	})

	t.Run("a fence added on a marker moved outside the diff", func(t *testing.T) {
		_, drafted := longFileRound(t, plainQuestion(git.Right, 4))
		body, err := os.ReadFile(drafted)
		require.NoError(t, err)
		moved := markerEdit(t, string(body), "f1", `start_line="4" line="4"`, `start_line="25" line="25"`)
		require.NoError(t, os.WriteFile(drafted, []byte(moved), 0o600))
		addFence(t, drafted, "// note 23, reworded")

		assertSuggestionRefused(t, "§8.2.1 refuses the replacement before §8.4.1 refuses the position")
	})

	t.Run("a fence added inside the hunk", func(t *testing.T) {
		_, drafted := longFileRound(t, plainQuestion(git.Right, 4))
		addFence(t, drafted, "\treturn nil")

		printed, err := runPost(t, fixturePR, "--repo", fixtureSlug)
		require.NoError(t, err, "the control: a fence §8.2 admits posts")
		assert.Contains(t, onlyCommentBody(t, printed), "```suggestion\n\treturn nil\n```")
	})

	t.Run("a stored suggestion on a LEFT anchor left in the body", func(t *testing.T) {
		stored := plainQuestion(git.Left, 3)
		stored.Suggestion = "func Load() error {"
		longFileRound(t, stored)

		assertSuggestionRefused(t, "the fence cr rendered is the one that would be sent")
	})

	t.Run("a stored suggestion on a LEFT anchor removed from the body", func(t *testing.T) {
		stored := plainQuestion(git.Left, 3)
		stored.Suggestion = "func Load() error {"
		_, drafted := longFileRound(t, stored)
		removeFence(t, drafted)

		printed, err := runPost(t, fixturePR, "--repo", fixtureSlug)
		require.NoError(t, err, "§8.2 has nothing to refuse once no suggestion is sent")
		assert.NotContains(t, onlyCommentBody(t, printed), "```suggestion")
	})
}

// onlyCommentBody is the body of the one comment the dry run's payload carries,
// which the fixture's one record becomes.
func onlyCommentBody(t *testing.T, printed string) string {
	t.Helper()
	var report dryRun
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	require.NotNil(t, report.Payload)
	require.Len(t, report.Payload.Comments, 1)
	return report.Payload.Comments[0].Body
}
