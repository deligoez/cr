package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/review"
)

// An `--axis` naming no axis of §1.5 is refused as the malformed invocation it
// is, with §11.2's code 2 and the closed set named, before any state is read.
func TestAnAxisOutsideSection15IsAUsageError(t *testing.T) {
	crHome(t)

	err := runCLI(t, "review", "7", "--repo", "acme/api", "--axis", "style")

	var unknown *unknownAxisFlagError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, ExitUsage, exitCodeFor(err))
	assert.Contains(t, err.Error(), "intent, correctness, convention, test")
}

// A terminal prints every prompt whole under the role and unit it is for, and
// then the lenses that did not run.
func TestATerminalReviewPrintsEveryPromptWholeUnderItsRoleAndUnit(t *testing.T) {
	result := &reviewResult{Fanout: &review.Fanout{
		Round: 2, Head: "abc123",
		Prompts: []review.Prompt{
			{Role: "correctness", Axis: "correctness", Unit: "u1", Text: "# Correctness on u1\n\nthe whole prompt\n"},
			{Role: "convention", Axis: "convention", Unit: "u1", Text: "# Convention on u1\n"},
		},
		Honesty: []string{"lens test/symbols unavailable, per §4.5.4: no index"},
	}}

	var printed bytes.Buffer
	require.NoError(t, (&writer{out: &printed, mode: ModeText}).emit(result))

	text := printed.String()
	assert.Contains(t, text, "review round 2 at head abc123: 2 prompt(s)")
	assert.Contains(t, text, "prompt correctness on u1\n\n# Correctness on u1\n\nthe whole prompt\n")
	assert.Contains(t, text, "prompt convention on u1\n\n# Convention on u1\n")
	assert.Contains(t, text, "lens test/symbols unavailable, per §4.5.4: no index")
}
