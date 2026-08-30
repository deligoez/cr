package gh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §3.7.1's identity comes back whole, and the two revisions are the object ids
// rather than the branch names.
//
// The head is the field that has to be an oid. §3.4.1 diffs against the merge
// base at the current head and §9.3.1 compares that head against the recorded
// one, so a branch name here would make the round's staleness depend on where
// somebody else's branch happens to point.
func TestReadingAPullRequestAnswersTheIdentitySection371Prints(t *testing.T) {
	recorded := replay(t, "pull-request.json")

	pr, err := WithRunner(recorded.run).PullRequest("acme", "api", 7)
	require.NoError(t, err)

	assert.Equal(t, 7, pr.Number)
	assert.Equal(t, "CR-31 retry the upload on a 5xx response", pr.Title)
	assert.Contains(t, pr.Body, "Closes CR-31.")
	assert.Equal(t, "feature/CR-31-retry-upload", pr.HeadRefName)
	assert.Equal(t, "5e6d7c8b9a807162534455667788990a1b2c3d4e", pr.Head)
	assert.Equal(t, "main", pr.BaseRefName)
	assert.Equal(t, "1f2e3d4c5b6a798071625344556677889900aabb", pr.Base)

	require.Len(t, recorded.calls, 1)
	owner, ok := field(recorded.calls[0], "owner")
	require.True(t, ok)
	assert.Equal(t, "acme", owner)
	number, ok := field(recorded.calls[0], "number")
	require.True(t, ok)
	assert.Equal(t, "7", number)
}

// The read is a read: the boundary of §2.1.2 runs it, so the invocation the
// query is built as has to pass readOnly rather than merely look harmless.
func TestReadingAPullRequestPassesTheWriteBoundary(t *testing.T) {
	read, why := readOnly([]string{
		"api", "graphql",
		"-f", "query=" + pullRequestQuery,
		"-f", "owner=acme",
		"-f", "repo=api",
		"-F", "number=7",
	})
	assert.True(t, read, why)
}

// An answer naming no pull request, and one whose revisions GitHub declares
// non-null and did not supply, are both refused rather than carried.
//
// The second is the expensive one. An empty head would be written into
// meta.json as the round's head, every §9.3.1 comparison afterwards would run
// against it, and the round would look oriented while pointing at nothing.
func TestAnAnswerWithoutTheTwoRevisionsIsRefused(t *testing.T) {
	t.Run("no pull request", func(t *testing.T) {
		answered := func(...string) (string, error) {
			return `{"data":{"repository":{"pullRequest":null}}}`, nil
		}
		_, err := WithRunner(answered).PullRequest("acme", "api", 7)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "acme/api#7")
	})

	t.Run("no head oid", func(t *testing.T) {
		answered := func(...string) (string, error) {
			return `{"data":{"repository":{"pullRequest":{"number":7,` +
				`"baseRefOid":"1f2e3d4c"}}}}`, nil
		}
		_, err := WithRunner(answered).PullRequest("acme", "api", 7)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "§3.4.1")
	})
}
