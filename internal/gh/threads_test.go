package gh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// fixture reads one recorded GraphQL answer.
func fixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(body)
}

// recorder answers each successive gh invocation with the next recorded
// payload and keeps the arguments it was called with.
//
// No test in this package reaches the network. A test that called the real API
// would need a token, would rate-limit in CI, and would assert against
// whatever the pull request looks like today rather than against a payload it
// can reason about. Every fixture under testdata was captured once, by hand,
// from a public repository, and is replayed from then on.
type recorder struct {
	t       *testing.T
	answers []string
	calls   [][]string
}

// replay returns a recorder answering with the named fixtures in order. An
// invocation it has no answer for fails the test, so a test states the exact
// sequence of requests it expects rather than letting a stray one pass.
func replay(t *testing.T, answers ...string) *recorder {
	t.Helper()
	return &recorder{t: t, answers: answers}
}

func (r *recorder) run(args ...string) (string, error) {
	r.t.Helper()
	require.Lessf(r.t, len(r.calls), len(r.answers),
		"gh was run %d times against %d recorded answers: %v",
		len(r.calls)+1, len(r.answers), args)
	r.calls = append(r.calls, args)
	return fixture(r.t, r.answers[len(r.calls)-1]), nil
}

// field returns the value gh was given for one GraphQL variable.
func field(args []string, name string) (string, bool) {
	for _, arg := range args {
		if value, found := strings.CutPrefix(arg, name+"="); found {
			return value, true
		}
	}
	return "", false
}

// §3.5.1 ingests every existing review thread, resolved ones included, with
// author, body, anchor, resolution state, and replies. A resolved thread is
// the one a reviewer is most tempted to drop, and it is the one §3.5.4 needs:
// a concern somebody already raised and closed is exactly what must not be
// raised a second time.
func TestIngestionKeepsResolvedAndUnresolvedThreadsAlike(t *testing.T) {
	answers := replay(t, "threads.json")

	threads, err := WithRunner(answers.run).Threads("cli", "cli", 11451)
	require.NoError(t, err)
	require.Len(t, threads, 2)

	resolved := threads[0]
	assert.Equal(t, "PRRT_kwDODKw3uc5XATDu", resolved.ID)
	assert.True(t, resolved.Resolved)
	assert.False(t, resolved.Outdated)
	assert.Equal(t, Anchor{
		Path:              "pkg/cmd/release/shared/fetch.go",
		Side:              git.Right,
		StartLine:         168,
		Line:              168,
		OriginalStartLine: 168,
		OriginalLine:      168,
	}, resolved.Anchor)
	assert.Equal(t, "copilot-pull-request-reviewer", resolved.Comment.Author)
	assert.Equal(t, "Bot", resolved.Comment.AuthorTypename)
	assert.Equal(t, "PRRC_kwDODKw3uc6G7GwX", resolved.Comment.ID)
	assert.Equal(t, "2025-08-08T17:29:32Z", resolved.Comment.CreatedAt)
	assert.Contains(t, resolved.Comment.Body, "masks all JSON decoding errors")
	assert.Contains(t, resolved.Comment.URL, "#discussion_r2263641111")
	assert.Empty(t, resolved.Replies)

	open := threads[1]
	assert.Equal(t, "PRRT_kwDODKw3uc5XAsRq", open.ID)
	assert.False(t, open.Resolved)
	assert.Equal(t, "babakks", open.Comment.Author)
	assert.Equal(t, "User", open.Comment.AuthorTypename)
	require.Len(t, open.Replies, 1)
	assert.Equal(t, "PRRC_kwDODKw3uc6G8AGk", open.Replies[0].ID)
	assert.Equal(t, "ejahnGithub", open.Replies[0].Author)
	assert.Contains(t, open.Replies[0].Body, "partial semver match")

	require.Len(t, answers.calls, 1)
	number, sent := field(answers.calls[0], "number")
	assert.True(t, sent)
	assert.Equal(t, "11451", number)
}

