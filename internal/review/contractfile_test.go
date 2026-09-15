package review

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §4.6.2: a run of `cr review` writes the record schema to the round's
// contract.md, and every prompt it emits names that file.
func TestARunWritesTheContractFileEveryPromptNames(t *testing.T) {
	src := briefed(t)
	fan, err := Run(src)
	require.NoError(t, err)
	require.NotEmpty(t, fan.Prompts)

	path := src.Layout.RoundFile(runOwner, runRepo, runPR, 1, state.FileContract)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, Contract(1), string(body))
	for _, prompt := range fan.Prompts {
		assert.Contains(t, prompt.Text, "read it before writing a record:\n\n    "+path+"\n\n",
			"%s on %s", prompt.Role, prompt.Unit)
	}
}
