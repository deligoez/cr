package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// reindentFence rewrites the one suggestion block of the fixture's draft.md the
// way a reviewer does: its first line moves from a tab to four spaces, and a
// second line is added below it.
func reindentFence(t *testing.T, path string) {
	t.Helper()
	const (
		rendered = "```suggestion\n\treturn fmt.Errorf(\"one\")\n```"
		edited   = "```suggestion\n    return fmt.Errorf(\"one\")\n    // and a line the reviewer added\n```"
	)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(body), rendered), "the draft renders the stored suggestion once")
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(body), rendered, edited, 1)), 0o600))
}

// editedWarning is §8.2.3's warning for the re-indented fence: the edited first
// line beside head line 4 of detectedHome's lib.go, which it replaces.
const editedWarning = "record f7: the suggestion's first line and the line it replaces are indented differently, " +
	"and §8.2.3 has cr infer nothing about indentation. Leave the block in place to post it " +
	"as written, or edit it.\n  suggestion: \"    return fmt.Errorf(\\\"one\\\")\"\n  replaces:   \"\\tpanic(\\\"one\\\")\""

// §8.2.3 over a suggestion the reviewer edited in draft.md: the stored
// suggestion is indented like the line it replaces and draws no warning, the
// reviewer re-indents the fence, and both `cr draft` and the `cr post` dry run
// warn about the edited first line while the payload still carries the edited
// block as written.
//
// The stored suggestion agrees with its line, so a warning computed from the
// record rather than from the body that will be posted stays silent on both
// commands — which is QA's D-S09-1.
func TestAnEditedSuggestionIsWarnedAboutInTheDraftAndThePost(t *testing.T) {
	suggestingRound(t, suggesting("f7", 4, "\treturn fmt.Errorf(\"one\")"))
	layout, err := state.Default()
	require.NoError(t, err)
	path := layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft)

	before, err := runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var unedited struct {
		Warnings []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(before), &unedited), before)
	require.Equal(t, []string{}, unedited.Warnings, "the stored suggestion is indented like its line")

	reindentFence(t, path)

	printed, err := runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err, "§8.2.3 warns; it does not refuse")
	var drafted struct {
		Preserved []string `json:"preserved"`
		Warnings  []string `json:"warnings"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &drafted), printed)
	assert.Equal(t, []string{"f7"}, drafted.Preserved, "the edited body is kept")
	assert.Equal(t, []string{editedWarning}, drafted.Warnings, "cr draft warns about the edited fence")

	posted, err := runPost(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err, "§8.2.3: the post still builds its payload")
	var report struct {
		Warnings []string `json:"warnings"`
		Posted   bool     `json:"posted"`
		Payload  struct {
			Comments []struct {
				Body string `json:"body"`
			} `json:"comments"`
		} `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(posted), &report), posted)
	assert.Equal(t, []string{editedWarning}, report.Warnings, "the cr post dry run warns about the edited fence")
	assert.False(t, report.Posted)
	require.Len(t, report.Payload.Comments, 1)
	assert.Equal(t, "```suggestion\n    return fmt.Errorf(\"one\")\n    // and a line the reviewer added\n```",
		fenceOf(t, report.Payload.Comments[0].Body), "the payload carries the edited suggestion as written")
}

// fenceOf is the one suggestion block of a comment body, from its opening fence
// through its closing one, so the block is compared whole.
func fenceOf(t *testing.T, body string) string {
	t.Helper()
	const open = "```suggestion\n"
	require.Equal(t, 1, strings.Count(body, open), body)
	start := strings.Index(body, open)
	end := strings.Index(body[start+len(open):], "\n```")
	require.GreaterOrEqual(t, end, 0, body)
	return body[start : start+len(open)+end+len("\n```")]
}
