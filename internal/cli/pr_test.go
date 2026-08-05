package cli

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePR(t *testing.T) {
	t.Run("accepts a positive pull request number", func(t *testing.T) {
		n, err := parsePR("42")
		require.NoError(t, err)
		assert.Equal(t, 42, n)
	})

	for _, arg := range []string{"", "0", "-1", "4.2", "#42", " 42", "forty-two"} {
		t.Run("rejects "+arg, func(t *testing.T) {
			_, err := parsePR(arg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "pass the pull request number")
		})
	}
}

func TestPRArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "answer <pr> <record-id> <text>"}

	assert.NoError(t, prArgs(1)(cmd, []string{"42"}))
	assert.NoError(t, prArgs(3)(cmd, []string{"42", "r-1", "text"}))

	assert.Error(t, prArgs(1)(cmd, nil), "a missing pull request is a usage error")
	assert.Error(t, prArgs(3)(cmd, []string{"42", "r-1"}), "a missing record id is a usage error")
	assert.Error(t, prArgs(3)(cmd, []string{"42", "r-1", "text", "extra"}))
	assert.Error(t, prArgs(2)(cmd, []string{"r-1", "42"}), "the pull request comes first")
}
