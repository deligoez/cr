package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Field-feedback 2.6: every prompt's output contract states §8.1.5's rule as a
// whole line of its own — a posted question body carries "?", composed at
// draft time — so no role learns it first from `cr post`'s refusal.
func TestEveryPromptStatesThatAPostedQuestionAsks(t *testing.T) {
	prompts := Emit(handRound())
	require.NotEmpty(t, prompts)
	for _, prompt := range prompts {
		assert.Contains(t, prompt.Text, "\nA kind=question record's posted body must contain \"?\" (§8.1.5); "+
			"that body is composed at draft time, where a question body that does not ask is rewritten into "+
			"one before `cr post` accepts it.\n", "%s on %s", prompt.Role, prompt.Unit)
	}
}
