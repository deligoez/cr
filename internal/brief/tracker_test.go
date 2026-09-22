package brief

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/intent"
)

// githubTracker is a configuration naming §3.1.8's `github` tracker.
func githubTracker(t *testing.T) config.Config {
	t.Helper()
	resolved, err := config.Resolve(config.Sources{Flags: map[string]any{intent.TrackerSetting: intent.TrackerGitHub}})
	require.NoError(t, err)
	return resolved
}

// githubRunner answers §3.1.8's two reads: the issue endpoint for each issue
// named in issues, and a pull request whose body names `#99` in prose and
// whose closing references are closing. Every call is recorded.
func githubRunner(closing string, issues map[string]string, called *[]string) gh.Runner {
	return func(args ...string) (string, error) {
		*called = append(*called, strings.Join(args, " "))
		if len(args) > 1 && strings.HasPrefix(args[1], "repos/") {
			return issues[args[1]], nil
		}
		return `{"data":{"repository":{"pullRequest":{"number":7,"title":"CR-7 retry","body":"see #99",` +
			`"headRefOid":"aaa","baseRefOid":"bbb","closingIssuesReferences":{"nodes":[` + closing + `]}}}}}`, nil
	}
}

// ref is one closing reference as GitHub answers it.
func ref(owner, repo string, number string) string {
	return `{"number":` + number + `,"repository":{"name":"` + repo + `","owner":{"login":"` + owner + `"}}}`
}

// §3.2 for a `github` tracker: the key is the one issue GitHub links the pull
// request to as closing, qualified by its repository, and the text is that
// issue's title and body read through cr's gh door — not a pattern over the
// title (`CR-7` is there) or the body (`#99` is there), and not intent.cmd.
func TestAGitHubTrackerTakesTheKeyFromTheClosingLink(t *testing.T) {
	var called []string
	src := &Sources{
		GH: gh.WithRunner(githubRunner(ref("acme", "shop", "12"),
			map[string]string{"repos/acme/shop/issues/12": `{"title":"Retry on 5xx","body":"Bounded at three."}`},
			&called)),
		Config: githubTracker(t), Owner: "acme", Repo: "shop", PR: 7,
		Intent: intent.Source{Cmd: []string{"jira-that-must-not-run", "{key}"}},
	}
	pr, err := src.GH.PullRequest("acme", "shop", 7)
	require.NoError(t, err)

	resolved, err := resolveIntent(src, &pr)

	require.NoError(t, err)
	assert.Equal(t, intent.Key{Value: "acme.shop#12", Origin: intent.KeyFromClosingReference}, resolved.Key)
	assert.Equal(t, "Retry on 5xx\n\nBounded at three.", resolved.Text)
	assert.Equal(t, intent.GitHubKeyPattern, resolved.Pattern)
	assert.Contains(t, called, "api repos/acme/shop/issues/12")
}

// `--issue` wins over the link, in any spelling, and a bare number is an issue
// of the pull request's own repository.
func TestAnIssueFlagOverridesTheClosingLink(t *testing.T) {
	var called []string
	src := &Sources{
		GH: gh.WithRunner(githubRunner(ref("acme", "shop", "12"),
			map[string]string{"repos/acme/shop/issues/40": `{"title":"Other","body":"x"}`}, &called)),
		Config: githubTracker(t), Owner: "acme", Repo: "shop", PR: 7, IssueFlag: "#40",
	}
	pr, err := src.GH.PullRequest("acme", "shop", 7)
	require.NoError(t, err)

	resolved, err := resolveIntent(src, &pr)

	require.NoError(t, err)
	assert.Equal(t, intent.Key{Value: "acme.shop#40", Origin: intent.KeyFromFlag}, resolved.Key)
}

// Two closing references are the reviewer's choice, and cr refuses rather than
// choosing; none is the empty intent, which §4.5.3 then discloses.
func TestTheClosingLinkIsTakenOnlyWhenThereIsExactlyOne(t *testing.T) {
	var called []string
	two := &Sources{
		GH:     gh.WithRunner(githubRunner(ref("acme", "shop", "12")+","+ref("acme", "shop", "13"), nil, &called)),
		Config: githubTracker(t), Owner: "acme", Repo: "shop", PR: 7,
	}
	pr, err := two.GH.PullRequest("acme", "shop", 7)
	require.NoError(t, err)
	_, err = resolveIntent(two, &pr)
	var ambiguous *intent.AmbiguousIssueError
	require.ErrorAs(t, err, &ambiguous)

	none := &Sources{
		GH:     gh.WithRunner(githubRunner("", nil, &called)),
		Config: githubTracker(t), Owner: "acme", Repo: "shop", PR: 7,
	}
	pr, err = none.GH.PullRequest("acme", "shop", 7)
	require.NoError(t, err)
	resolved, err := resolveIntent(none, &pr)
	require.NoError(t, err)
	_, unavailable := resolved.Unavailability()
	assert.True(t, unavailable, "`#99` in the body is prose, not a reference")
	for _, call := range called {
		assert.NotContains(t, call, "issues/99", "a mention in prose is never read as the issue")
	}
}
