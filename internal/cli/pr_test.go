package cli

import (
	"testing"

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
