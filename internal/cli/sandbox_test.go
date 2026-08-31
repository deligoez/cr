package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The sandbox a rendering is asked to print. The head is a full revision and
// the path a real §2.2 one, because both are values the reader is expected to
// paste somewhere else.
const (
	renderedSandboxPath = "/home/dev/.cr/state/acme/web/pr-42/sandbox"
	renderedSandboxHead = "be7e2c75aeb661ba3c96d7a7634f3f8e84bb7b91"
)

// renderedSandbox returns `cr sandbox create`'s payload as one mode prints it.
func renderedSandbox(t *testing.T, mode Mode) string {
	t.Helper()
	var printed bytes.Buffer
	out := &writer{out: &printed, mode: mode}
	require.NoError(t, out.emit(&sandboxCreateResult{
		Path: renderedSandboxPath, Head: renderedSandboxHead,
	}))
	return printed.String()
}

// Both renderings name the worktree and the revision in it.
//
// The two are asserted separately because they fail apart. The JSON document is
// what an agent parses, so it is decoded and each value looked up by its key;
// the terminal rendering is what a person reads, so it is searched for the
// values themselves. A payload that carried both while printing neither would
// pass an assertion on the document alone — and the head in particular is the
// value §5.1.6 checks the sandbox against before every later run, so a reader
// told only that something was created has been told nothing they can check.
func TestTheSandboxCreationNamesTheWorktreeAndItsHead(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var printed map[string]any
		require.NoError(t, json.Unmarshal([]byte(renderedSandbox(t, ModeJSON)), &printed))

		assert.Equal(t, renderedSandboxPath, printed["path"])
		assert.Equal(t, renderedSandboxHead, printed["head"])
	})

	t.Run("terminal", func(t *testing.T) {
		printed := renderedSandbox(t, ModeText)

		assert.Contains(t, printed, renderedSandboxPath)
		assert.Contains(t, printed, renderedSandboxHead)
	})
}
