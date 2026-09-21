package review

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/render"
)

// Measurement 4 part B found two constraints a role is judged by and was never
// told, and lost one run and one unit's work to them. Both tests here hold the
// telling, not the enforcement: the enforcement already worked, which is exactly
// why the silence was expensive.

// §5.7's `filter` and `paths` were the only rows the proposal sentence named
// without explaining, and `x36101` filled `paths` with the source file its
// hypothesis was about — `go test ./internal/probe/resolve.go`, which compiles
// one file as a package of its own, ran no test and settled nothing.
//
// The assertion is on the two claims a role has to come away with: what the
// fields scope, and what to do when unsure. A prompt that named them again
// without saying which way round they go would pass a mere Contains on the
// names, so the direction is asserted as prose.
func TestEveryPromptSaysWhatFilterAndPathsScope(t *testing.T) {
	r := handRound()
	prompts := Emit(r)
	require.NotEmpty(t, prompts)
	for _, prompt := range prompts {
		_, proposals, found := strings.Cut(prompt.Text, "\n## Proposed experiments (§5.7)\n\n")
		require.Truef(t, found, "%s on %s", prompt.Role, prompt.Unit)
		assert.Contains(t, proposals,
			"`filter` and `paths` scope the run, never the code the experiment is about.",
			"%s on %s", prompt.Role, prompt.Unit)
		assert.Contains(t, proposals, "`filter` is the name of a test for the runner to select, "+
			"not a runner argument string", "%s on %s", prompt.Role, prompt.Unit)
		assert.Contains(t, proposals, "the package or directory whose tests are to run, never the "+
			"file the patch changes", "%s on %s", prompt.Role, prompt.Unit)
		assert.Contains(t, proposals, "Leave both out to run the whole suite",
			"the safe answer is named, so an unsure role has one", "%s on %s", prompt.Role, prompt.Unit)
	}
}

// §8.1.3's sequence is refused where a record enters, and nothing told the role
// it existed. M4 part B: a role reviewing cr's own marker machinery quoted the
// sequence to name it, the record was refused, and the proposal naming that
// record was refused after it.
//
// The contract file is where the other body rule (§8.1.5's "?") lives, so the
// sentence belongs beside it rather than repeated in every prompt — and every
// prompt already names the file. The sequence is asserted as render.Reserved,
// so a change to the bytes cr refuses moves the prompt with it rather than
// leaving a stale literal behind.
func TestTheContractNamesTheSequenceARecordMayNotCarry(t *testing.T) {
	text := Contract(1)
	assert.Contains(t, text, render.Reserved,
		"the contract names the bytes the refusal looks for, not a paraphrase of them")
	assert.Contains(t, text, "No field of a record may carry the sequence")
	assert.Contains(t, text, "so a record carrying it is rejected with exit code 1 where the record enters")
	assert.Contains(t, text, "name the sequence in words rather than quoting it",
		"a role reviewing cr's own marker machinery is given a way through")
}
