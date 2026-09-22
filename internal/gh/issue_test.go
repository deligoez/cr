package gh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// answering is a Runner that records the argv it was given and answers every
// call with body.
func answering(body string, called *[]string) Runner {
	return func(args ...string) (string, error) {
		*called = args
		return body, nil
	}
}

// §3.1.8 reads an issue's title and body through the REST issue endpoint, a GET
// the read boundary admits.
func TestAnIssueIsReadThroughTheRESTEndpoint(t *testing.T) {
	var called []string
	issue, err := WithRunner(answering(
		`{"title":"Retry on 5xx","body":"Bounded at three.","number":12}`, &called)).Issue("acme", "shop", 12)

	require.NoError(t, err)
	assert.Equal(t, Issue{Title: "Retry on 5xx", Body: "Bounded at three."}, issue)
	assert.Equal(t, []string{"api", "repos/acme/shop/issues/12"}, called)
	read, why := readOnly(called)
	assert.True(t, read, why)
}

// GitHub answers the issue endpoint for a pull request too, and marks it only
// with a `pull_request` key. Measured 2026-09-22: `gh issue view 1` on a
// repository with no issues returned pull request #1 without a word, so the
// read refuses on the key rather than trusting the number.
func TestAPullRequestNumberIsRefusedAsAnIssue(t *testing.T) {
	var called []string
	_, err := WithRunner(answering(
		`{"title":"feat: retry","body":"Closes #3.","pull_request":{"url":"https://api.github.com/repos/acme/shop/pulls/1"}}`,
		&called)).Issue("acme", "shop", 1)

	var refused *NotAnIssueError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, 1, refused.Number)
}

// An issue whose `pull_request` is null is an issue: the key's absence and a
// null value are the same answer.
func TestANullPullRequestKeyIsAnIssue(t *testing.T) {
	var called []string
	issue, err := WithRunner(answering(`{"title":"t","body":"b","pull_request":null}`, &called)).
		Issue("acme", "shop", 2)

	require.NoError(t, err)
	assert.Equal(t, "t", issue.Title)
}

// §3.2's second source for a `github` tracker is the pull request's closing
// references, read in the same query as its identity, with the repository on
// every one so a cross-repository reference names its own.
func TestAPullRequestCarriesTheIssuesItCloses(t *testing.T) {
	var called []string
	pr, err := WithRunner(answering(`{"data":{"repository":{"pullRequest":{
		"number":7,"title":"t","body":"b","headRefName":"f","headRefOid":"aaa","baseRefName":"main","baseRefOid":"bbb",
		"state":"OPEN","closingIssuesReferences":{"nodes":[
			{"number":12,"repository":{"name":"shop","owner":{"login":"acme"}}},
			{"number":5,"repository":{"name":"lib","owner":{"login":"other"}}}]}}}}}`, &called)).
		PullRequest("acme", "shop", 7)

	require.NoError(t, err)
	assert.Equal(t, []IssueRef{{Owner: "acme", Repo: "shop", Number: 12}, {Owner: "other", Repo: "lib", Number: 5}},
		pr.ClosingIssues)
}

// An answer carrying no closing references is a pull request closing none,
// and the list is empty rather than null (§12.3).
func TestAPullRequestClosingNothingCarriesAnEmptyList(t *testing.T) {
	var called []string
	pr, err := WithRunner(answering(`{"data":{"repository":{"pullRequest":{
		"number":7,"headRefOid":"aaa","baseRefOid":"bbb"}}}}`, &called)).PullRequest("acme", "shop", 7)

	require.NoError(t, err)
	assert.NotNil(t, pr.ClosingIssues)
	assert.Empty(t, pr.ClosingIssues)
}
