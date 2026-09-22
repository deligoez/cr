package gh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §9.6's write is behind §8.5's gate like every other, and the gate is not
// re-implemented in settle.go — the resolve reaches Confirmation.Write and is
// refused there.
//
// The marker file is the assertion rather than the error: a refusal that
// arrived after gh ran would be a report about a write that had already
// happened, which is the failure §2.1.2's single door exists to prevent.
func TestTheSettlingWriteDoesNotRunWithoutConfirmation(t *testing.T) {
	ran := filepath.Join(t.TempDir(), "gh-was-started")
	stubGh(t, "touch "+ran+"\necho '{}'")

	err := Confirm(false).ResolveThread("PRRT_kwDO")

	var refused *WriteRefusedError
	require.ErrorAs(t, err, &refused)
	assert.NoFileExists(t, ran, "gh was started before the boundary refused")
}

// §9.6.1: a resolution is recorded only when GitHub reports one.
//
// The mutation returning without resolving is the case worth a test. A caller
// reading a 200 as success would record a thread resolved that is still open
// on the pull request, and nothing later would notice: cr's own state would
// agree with itself and disagree with GitHub.
func TestAResolveIsOnlyARresolutionWhenGitHubSaysSo(t *testing.T) {
	t.Run("GitHub resolved it", func(t *testing.T) {
		stubGh(t, `echo '{"data":{"resolveReviewThread":{"thread":{"id":"T1","isResolved":true}}}}'`)

		require.NoError(t, Confirm(true).ResolveThread("T1"))
	})

	t.Run("GitHub answered without resolving it", func(t *testing.T) {
		stubGh(t, `echo '{"data":{"resolveReviewThread":{"thread":{"id":"T1","isResolved":false}}}}'`)

		err := Confirm(true).ResolveThread("T1")

		var unresolved *NotResolvedError
		require.ErrorAs(t, err, &unresolved)
		assert.Equal(t, "T1", unresolved.Thread)
	})

	t.Run("the answer is not readable", func(t *testing.T) {
		stubGh(t, `echo 'not json'`)

		require.Error(t, Confirm(true).ResolveThread("T1"))
	})
}

// The resolve is a GraphQL mutation and travels as one, with the thread id in
// the variable the mutation declares rather than interpolated into the query.
//
// Interpolating it would work and is the thing not to do: the id comes from
// GitHub's own answer, but a query built by concatenation is a query whose
// shape depends on its data, and cr sends the same mutation every time.
//
// The variable is asserted as the one argument `-f thread=<id>`, which is the
// shape `gh api graphql` turns into `$thread`. v0.5.0's `variables={...}`
// passed this test's previous assertion and reached GitHub as a variable named
// `variables`, leaving `$thread` null, so what is pinned now is the argument gh
// reads and not an encoding that merely contains the id.
func TestTheResolveSendsItsThreadIdAsTheThreadVariable(t *testing.T) {
	sent := filepath.Join(t.TempDir(), "argv")
	stubGh(t, `for argument in "$@"; do printf '%s\n' "$argument"; done > `+sent+`
echo '{"data":{"resolveReviewThread":{"thread":{"id":"T1","isResolved":true}}}}'`)

	require.NoError(t, Confirm(true).ResolveThread("PRRT_kwDOabc"))

	argv, err := os.ReadFile(sent)
	require.NoError(t, err)
	arguments := strings.Split(strings.TrimSuffix(string(argv), "\n"), "\n")
	require.GreaterOrEqual(t, len(arguments), 2)
	assert.Equal(t, []string{"api", "graphql"}, arguments[:2])
	assert.Contains(t, arguments, "thread=PRRT_kwDOabc",
		"gh fills $thread from a field named thread and from nothing else")
	for _, argument := range arguments {
		assert.False(t, strings.HasPrefix(argument, "variables="),
			"gh sends a field named variables as a variable named variables")
	}
	assert.True(t, strings.Contains(string(argv), "resolveReviewThread"),
		"the mutation cr sends is the one in settle.go")
	assert.NotContains(t, string(argv), `threadId:"PRRT_kwDOabc"`,
		"the id is a variable, not interpolated into the query")
}
