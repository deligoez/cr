package git

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A commit the repository holds is required without complaint, one it does not
// hold is a MissingCommitError naming the repository and the commit, and a
// directory in no repository stays git's own CommandError: only the second is a
// fetch.
func TestRequireCommitTellsAnAbsentCommitFromAFailingGit(t *testing.T) {
	repo, head := trackedFixture(t)
	absent := "0123456789abcdef0123456789abcdef01234567"

	require.NoError(t, RequireCommit(repo, head))

	err := RequireCommit(repo, absent)
	var missing *MissingCommitError
	require.True(t, errors.As(err, &missing), "got %v", err)
	assert.Equal(t, MissingCommitError{Dir: repo, Commit: absent}, *missing)
	assert.Equal(t, "the repository at "+repo+" does not hold commit "+absent, err.Error())

	err = RequireCommit(t.TempDir(), head)
	var failed *CommandError
	require.True(t, errors.As(err, &failed), "got %v", err)
	assert.False(t, errors.As(err, &missing), "a git that could not run is not an absent commit")
}
