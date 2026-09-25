package gh

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.6.3.5 reads a repository's review comments newest first through the REST
// listing, a GET the read boundary admits, and tags the author the way §3.5.2
// tags a thread's: `bot` for GitHub's `Bot` type, `human` for everything else,
// a gone account included.
func TestReviewCommentsAreReadNewestFirstThroughTheReadBoundary(t *testing.T) {
	var called []string
	comments, last, err := WithRunner(answering(`[
		{"id":2,"html_url":"https://github.com/acme/shop/pull/7#discussion_r2",
		 "pull_request_url":"https://api.github.com/repos/acme/shop/pulls/7","created_at":"2025-02-02T00:00:00Z",
		 "path":"a.go","line":null,"original_line":9,"side":"RIGHT","body":"why?","in_reply_to_id":1,
		 "user":{"login":"Copilot","type":"Bot"}},
		{"id":1,"html_url":"https://github.com/acme/shop/pull/7#discussion_r1",
		 "pull_request_url":"https://api.github.com/repos/acme/shop/pulls/7","created_at":"2025-02-01T00:00:00Z",
		 "path":"a.go","line":4,"side":"RIGHT","body":"rename","user":null}]`, &called)).
		ReviewCommentsPage("acme", "shop", 3)

	require.NoError(t, err)
	assert.Equal(t, []string{"api", "repos/acme/shop/pulls/comments?sort=created&direction=desc&per_page=100&page=3"},
		called)
	read, why := readOnly(called)
	assert.True(t, read, why)
	assert.True(t, last, "a page shorter than the page size is the last")
	assert.Equal(t, []HistoryComment{
		{ID: 2, PR: 7, URL: "https://github.com/acme/shop/pull/7#discussion_r2", Author: "Copilot",
			AuthorType: AuthorBot, CreatedAt: "2025-02-02T00:00:00Z", Path: "a.go", Line: 9, Side: "RIGHT",
			Body: "why?", InReplyTo: 1},
		{ID: 1, PR: 7, URL: "https://github.com/acme/shop/pull/7#discussion_r1", AuthorType: AuthorHuman,
			CreatedAt: "2025-02-01T00:00:00Z", Path: "a.go", Line: 4, Side: "RIGHT", Body: "rename"},
	}, comments)
}

// A full page is not the last, so the walk asks for the next one.
func TestAFullPageOfReviewCommentsIsNotTheLast(t *testing.T) {
	node := `{"id":1,"pull_request_url":"https://api.github.com/repos/acme/shop/pulls/7","created_at":"2025-02-01T00:00:00Z"}`
	var called []string
	_, last, err := WithRunner(answering("["+strings.Repeat(node+",", 99)+node+"]", &called)).
		ReviewCommentsPage("acme", "shop", 1)

	require.NoError(t, err)
	assert.False(t, last)
}

// A comment naming no pull request can be neither grouped nor compared against
// its author, so the read refuses it rather than inventing a number.
func TestAReviewCommentNamingNoPullRequestIsRefused(t *testing.T) {
	var called []string
	_, _, err := WithRunner(answering(`[{"id":1,"pull_request_url":""}]`, &called)).
		ReviewCommentsPage("acme", "shop", 1)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "names no pull request")
}

// §2.6.3.6 reads a pull request's opener through the REST read, a GET.
func TestAPullRequestsAuthorIsReadThroughTheRESTEndpoint(t *testing.T) {
	var called []string
	author, err := WithRunner(answering(`{"number":7,"user":{"login":"alice"}}`, &called)).
		PullAuthor("acme", "shop", 7)

	require.NoError(t, err)
	assert.Equal(t, "alice", author)
	assert.Equal(t, []string{"api", "repos/acme/shop/pulls/7"}, called)
	read, why := readOnly(called)
	assert.True(t, read, why)
}

// An answer naming another pull request is not the one asked about.
func TestAnAnswerForAnotherPullRequestIsRefused(t *testing.T) {
	var called []string
	_, err := WithRunner(answering(`{"number":8,"user":{"login":"alice"}}`, &called)).PullAuthor("acme", "shop", 7)

	var refused *AnswerError
	require.ErrorAs(t, err, &refused)
}
