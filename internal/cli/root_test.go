package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootPrintsHelpWithNoArguments(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{})

	require.NoError(t, cmd.Execute())
	help := out.String()
	assert.Contains(t, help, "Usage:")
	assert.Contains(t, help, "--json")
}

func TestRootVersion(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--version"})

	require.NoError(t, cmd.Execute())
	assert.Contains(t, out.String(), "cr version")
}

// Exit codes are a published contract; a renumbering must fail loudly.
func TestExitCodesArePinned(t *testing.T) {
	assert.Equal(t, 0, ExitOK)
	assert.Equal(t, 1, ExitValidation)
	assert.Equal(t, 2, ExitUsage)
	assert.Equal(t, 3, ExitFile)
	assert.Equal(t, 4, ExitState)
}
