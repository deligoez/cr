package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// `cr brief` names both heads when the remote's pull ref disagrees with gh's,
// and says nothing when they agree. Measured 2026-09-22 on deligoez/cr-qa: a
// brief run straight after a push stayed in the old round because GitHub's API
// still reported the old head, and nothing in its output said so.
func TestBriefNamesBothHeadsWhenTheRemoteIsAheadOfGitHub(t *testing.T) {
	restore := remotePullHead
	t.Cleanup(func() { remotePullHead = restore })

	const pushed = "1111111111111111111111111111111111111111"
	var asked []string
	remotePullHead = func(_, owner, repo string, pr int) string {
		asked = append(asked, owner+"/"+repo)
		require.Equal(t, fixturePRNumber, pr)
		return pushed
	}

	emptyRoundHome(t)
	issue := writeOutside(t, "issue.txt", fixtureIssue+": load the configuration.\n")
	honesty := briefHonesty(t, issue)

	require.NotEmpty(t, asked)
	for _, slug := range asked {
		assert.Equal(t, fixtureSlug, slug, "the remote is asked for the pull request under review")
	}
	var lag []string
	for _, said := range honesty {
		if strings.Contains(said, "refs/pull/") {
			lag = append(lag, said)
		}
	}
	require.Len(t, lag, 1, "one disclosure names the disagreement: %q", honesty)
	assert.Contains(t, lag[0], pushed, "the remote's head is named")
	assert.Contains(t, lag[0], "Brief again once both name the commit that was pushed")
}

// The disclosure appears only when the remote was read and disagrees.
func TestHeadLagSaysNothingWhenTheHeadsAgreeOrTheRemoteWasNotRead(t *testing.T) {
	const head = "abcdef0123456789abcdef0123456789abcdef01"
	assert.Empty(t, headLag("acme", "web", 3, head, head))
	assert.Empty(t, headLag("acme", "web", 3, head, strings.ToUpper(head)))
	assert.Empty(t, headLag("acme", "web", 3, head, ""), "an unread remote is no disagreement")

	said := headLag("acme", "web", 3, head, "1111111111111111111111111111111111111111")
	require.Len(t, said, 1)
	assert.Contains(t, said[0], "acme/web#3: GitHub's API reports head "+head)
	assert.Contains(t, said[0], "refs/pull/3/head is 1111111111111111111111111111111111111111")
}
