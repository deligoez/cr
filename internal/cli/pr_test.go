package cli

import (
	"strings"
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

// Record ids are scoped to a PR (spec/0.1.0.md §11), so a command naming one
// must take the pull request ahead of it and must reject an argument that is
// not a pull request number. The walk covers the whole tree, so a command
// registered later cannot quietly drop the scope.
func TestRecordIDCommandsTakeThePullRequest(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, sub := range cmd.Commands() {
			walk(sub)
		}

		if i := strings.Index(cmd.Use, "<record-id>"); i >= 0 {
			pr := strings.Index(cmd.Use, prPlaceholder)
			require.GreaterOrEqual(t, pr, 0,
				"%q names a record id without the PR that scopes it", cmd.Use)
			assert.Less(t, pr, i, "%q must take the PR before the record id", cmd.Use)
		}

		if !strings.Contains(cmd.Use, prPlaceholder) {
			return
		}
		require.NotNil(t, cmd.Args, "%q takes a PR but validates no arguments", cmd.Use)
		assert.Error(t, cmd.Args(cmd, []string{"not-a-pull-request"}),
			"%q accepts an argument that is not a pull request", cmd.Use)
	}

	walk(newRootCmd())
}
