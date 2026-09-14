package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// QA D-S11-1: a meta.json that is there and does not decode is cr's own state
// it cannot use, which §11.2 codes 3 with state.UnusableHint, from every command
// that reads it — never "has no round" with exit 4 and a hint to run `cr brief`,
// which reads the same file and refuses it. Only an absent meta.json is a pull
// request no brief has opened.
//
// meta.json is truncated to half its bytes, as the QA run did, and `cr status`,
// `cr record` and `cr brief` are each driven against it. The control removes
// the file instead and still gets §3.7's refusal, code 4.
func TestAMetaJSONThatDoesNotDecodeIsUnusableStateInEveryCommand(t *testing.T) {
	commands := map[string]func(t *testing.T) []string{
		"status": func(*testing.T) []string { return []string{"status", fixturePR, "--repo", fixtureSlug} },
		"record": func(t *testing.T) []string {
			return []string{"record", fixturePR, writeRecordFile(t, "merged.ndjson", confirming("f1", 4)),
				"--repo", fixtureSlug}
		},
		// --intent-file keeps §3.1's tracker command from running at all.
		"brief": func(t *testing.T) []string {
			issue := filepath.Join(t.TempDir(), "issue.txt")
			require.NoError(t, os.WriteFile(issue, []byte("Retry the upload.\n"), 0o600))
			return []string{"brief", fixturePR, "--repo", fixtureSlug, "--issue", fixtureIssue, "--intent-file", issue}
		},
	}
	for name, args := range commands {
		t.Run(name+" truncated", func(t *testing.T) {
			layout := detectedHome(t)
			metaPath := layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileMeta)
			body, err := os.ReadFile(metaPath)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(metaPath, body[:len(body)/2], 0o600))

			err = runCLI(t, args(t)...)
			require.Error(t, err)
			var file *state.FileError
			require.True(t, errors.As(err, &file), "the refusal is a file of cr's own state: %v", err)
			var unbriefed *state.NotBriefedError
			assert.False(t, errors.As(err, &unbriefed), "a meta.json that is there is not an unbriefed pull request")
			assert.Equal(t, "cannot read "+metaPath+": unexpected end of JSON input", err.Error())
			assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2: cr's own state it cannot use is 3")
			assert.Equal(t, state.UnusableHint, hintFor(err))
		})
	}
	for _, name := range []string{"status", "record"} {
		t.Run(name+" absent", func(t *testing.T) {
			layout := detectedHome(t)
			require.NoError(t, os.Remove(layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileMeta)))

			err := runCLI(t, commands[name](t)...)
			var unbriefed *state.NotBriefedError
			require.True(t, errors.As(err, &unbriefed), "an absent meta.json is no round: %v", err)
			assert.Equal(t, ExitState, exitCodeFor(err))
			assert.Equal(t, "run `cr brief <pr>` to open a round on the pull request", hintFor(err))
		})
	}
}
