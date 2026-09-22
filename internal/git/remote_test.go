package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Remotes lists each remote once, by its fetch URL, and an empty list for a
// repository that declares none.
func TestRemotesListsEachRemoteOnceByItsFetchURL(t *testing.T) {
	dir := fixtureRepo(t)
	none, err := Remotes(dir)
	require.NoError(t, err)
	assert.Empty(t, none)

	fixtureGit(t, dir, "remote", "add", "origin", "git@github.com:acme/api.git")
	fixtureGit(t, dir, "remote", "set-url", "--push", "origin", "git@github.com:acme/push-only.git")
	fixtureGit(t, dir, "remote", "add", "upstream", "https://github.com/base/api")

	remotes, err := Remotes(dir)
	require.NoError(t, err)
	assert.Equal(t, []Remote{
		{Name: "origin", URL: "git@github.com:acme/api.git"},
		{Name: "upstream", URL: "https://github.com/base/api"},
	}, remotes)
}

// PullHead reads the commit a remote's refs/pull/<n>/head names, and answers
// empty for a pull request the remote holds no ref for. The remote here is a
// local repository, so the read reaches no network.
func TestPullHeadReadsTheRemotesPullRef(t *testing.T) {
	upstream := fixtureRepo(t)
	fixtureGit(t, upstream, "commit", "--quiet", "--allow-empty", "-m", "pushed")
	pushed := fixtureGit(t, upstream, "rev-parse", "HEAD")
	fixtureGit(t, upstream, "update-ref", "refs/pull/7/head", pushed)

	clone := fixtureRepo(t)
	fixtureGit(t, clone, "remote", "add", "origin", upstream)

	head, err := PullHead(clone, "origin", 7)
	require.NoError(t, err)
	assert.Equal(t, pushed, head)

	absent, err := PullHead(clone, "origin", 8)
	require.NoError(t, err)
	assert.Empty(t, absent)
}

// A listing that is not name, URL and direction is refused rather than read as
// fewer remotes.
func TestParseRemotesRefusesALineItCannotRead(t *testing.T) {
	_, err := parseRemotes("origin git@github.com:acme/api.git\n")
	require.Error(t, err)
}
