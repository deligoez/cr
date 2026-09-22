package intent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §3.2's key for a `github` tracker carries its repository, because an issue
// number is unique in one repository and §2.2's context store is keyed by the
// key alone: `#12` would put every repository's twelfth issue in one store.
// Every spelling a reviewer types for `--issue` normalises to that one key.
func TestAGitHubIssueFlagNormalisesToARepositoryQualifiedKey(t *testing.T) {
	for flag, want := range map[string]string{
		"12":          "acme.shop#12",
		"#12":         "acme.shop#12",
		"other/lib#5": "other.lib#5",
		"https://github.com/other/lib.go/issues/5":  "other.lib.go#5",
		"https://github.com/other/lib.go/issues/5/": "other.lib.go#5",
		"acme.shop#12": "acme.shop#12",
	} {
		t.Run(flag, func(t *testing.T) {
			key, err := GitHubIssueFlag(flag, "acme", "shop")
			require.NoError(t, err)
			assert.Equal(t, want, key)
			assert.True(t, CheckKey(key, GitHubKeyPattern) == nil, "the key matches its own pattern whole")
		})
	}

	padded, err := GitHubIssueFlag("\t12\n", "acme", "shop")
	require.NoError(t, err, "a flag value carrying whitespace is read as the number it holds")
	assert.Equal(t, "acme.shop#12", padded)

	for _, wrong := range []string{"", "0", "#", "CR-12", "https://gitlab.com/a/b/issues/1", "a/b"} {
		_, err := GitHubIssueFlag(wrong, "acme", "shop")
		var refused *IssueFlagError
		require.ErrorAs(t, err, &refused, "%q names no GitHub issue", wrong)
	}
}

// The key splits back into its parts at the first `.`: an owner holds no dot
// and a repository may, so the split is unambiguous by construction.
func TestAGitHubKeySplitsAtTheFirstDot(t *testing.T) {
	owner, repo, number, ok := SplitGitHubKey(GitHubKey("deligoez", "cr.qa", 7))
	require.True(t, ok)
	assert.Equal(t, "deligoez", owner)
	assert.Equal(t, "cr.qa", repo)
	assert.Equal(t, 7, number)

	for _, notAKey := range []string{"CR-7", "deligoez#7", ".cr#7", "deligoez.cr#0", "deli.goez.cr#x"} {
		_, _, _, ok := SplitGitHubKey(notAKey)
		assert.False(t, ok, notAKey)
	}
}

// §3.2's second source for a `github` tracker is GitHub's closing link, and cr
// takes it only when there is exactly one: none is the empty intent, and more
// than one is the reviewer's choice to make (P2).
func TestTheClosingLinkYieldsAKeyOnlyWhenThereIsOne(t *testing.T) {
	none, err := LinkedKey(nil)
	require.NoError(t, err)
	assert.Equal(t, KeyAbsent, none.Origin)

	one, err := LinkedKey([]string{"acme.shop#12"})
	require.NoError(t, err)
	assert.Equal(t, Key{Value: "acme.shop#12", Origin: KeyFromClosingReference}, one)

	_, err = LinkedKey([]string{"acme.shop#12", "acme.shop#13"})
	var ambiguous *AmbiguousIssueError
	require.ErrorAs(t, err, &ambiguous)
	assert.Equal(t, []string{"acme.shop#12", "acme.shop#13"}, ambiguous.Keys)
}

// Every reader of §3.2's pattern gets the GitHub key shape for a `github`
// tracker, whatever `intent.key_pattern` says: that setting describes a
// command tracker's keys, and the Jira-shaped default would refuse every key
// a `github` tracker forms.
func TestAGitHubTrackerHoldsKeysToItsOwnPattern(t *testing.T) {
	assert.Equal(t, GitHubKeyPattern, KeyPattern(TrackerGitHub, `[A-Z][A-Z0-9]+-[0-9]+`))
	assert.Equal(t, `[A-Z][A-Z0-9]+-[0-9]+`, KeyPattern(TrackerCommand, `[A-Z][A-Z0-9]+-[0-9]+`))
	assert.True(t, PatternAdmits(GitHubKeyPattern, "acme.shop#12"))
}

// A `github` tracker's unavailability names what would give it a key, and not
// a pattern over the branch it never searched.
func TestAGitHubTrackerWithNoLinkSaysWhatWouldGiveItAKey(t *testing.T) {
	reason, unavailable := Intent{Pattern: GitHubKeyPattern}.Unavailability()

	require.True(t, unavailable)
	assert.Contains(t, reason.Reason, "--issue")
	assert.Contains(t, reason.Reason, "Closes #12")
	assert.NotContains(t, reason.Reason, "branch name, the title, or the body;")
}

// §3.1.8: a Fetch reads the text for the key, and §3.1.4's file still wins.
func TestAFetchReadsTheTextAndTheFileStillBypassesIt(t *testing.T) {
	fetched := ""
	source := Source{Fetch: func(key string) (string, error) {
		fetched = key
		return "Retry on 5xx\n\nBounded at three.", nil
	}}

	reading, err := Read(source, "acme.shop#12")
	require.NoError(t, err)
	assert.Equal(t, "acme.shop#12", fetched)
	assert.Equal(t, "Retry on 5xx\n\nBounded at three.", reading.Text)

	source.File = "does-not-exist.txt"
	_, err = Read(source, "acme.shop#13")
	require.Error(t, err, "the file named is read, and it does not exist")
	assert.Equal(t, "acme.shop#12", fetched, "a file bypasses the fetch as it bypasses the command")
}
