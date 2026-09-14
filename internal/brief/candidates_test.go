package brief

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/note"
)

// repliedThread is a page holding one human thread whose reviewer's question the
// pull request's author answered, followed by a second reviewer's reply.
const repliedThread = `{"data":{"repository":{"pullRequest":{"reviewThreads":{` +
	`"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{` +
	`"id":"PRRT_1","isResolved":false,"isOutdated":false,"path":"order.go",` +
	`"line":3,"startLine":3,"originalLine":3,"originalStartLine":3,"diffSide":"RIGHT",` +
	`"comments":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[` +
	`{"id":"PRRC_1","url":"https://example.invalid/1","body":"Is shipping always charged?",` +
	`"createdAt":"2026-08-30T09:00:00Z","author":{"__typename":"User","login":"reviewer"}},` +
	`{"id":"PRRC_2","url":"https://example.invalid/2","body":"Yes, free shipping was dropped last sprint.",` +
	`"createdAt":"2026-08-30T10:00:00Z","author":{"__typename":"User","login":"author"}},` +
	`{"id":"PRRC_3","url":"https://example.invalid/3","body":"Agreed.",` +
	`"createdAt":"2026-08-30T11:00:00Z","author":{"__typename":"User","login":"reviewer"}}` +
	`]}}]}}}}}`

// openedBy is answering's pull request, opened by author.
func openedBy(head, base, author, threads string) gh.Runner {
	answered := answering(head, base, threads)
	return func(args ...string) (string, error) {
		body, err := answered(args...)
		if err != nil || strings.Contains(strings.Join(args, " "), "reviewThreads") {
			return body, err
		}
		return strings.Replace(body, `"baseRefOid"`, `"author":{"login":"`+author+`"},"baseRefOid"`, 1), nil
	}
}

// §3.5.5: the pull request author's reply inside an ingested thread is offered
// as a candidate context note, and neither the reviewer's opener nor the
// reviewer's own reply is. It is offered and never written: the issue key's
// context store is not created, and the payload names `cr note --source thread`
// as the way to record it.
func TestAnAuthorReplyIsOfferedAsACandidateNoteAndNoNoteIsWritten(t *testing.T) {
	dir, head, base := repository(t)
	src := sources(t, dir, openedBy(head, base, "author", repliedThread))

	assembled, err := Run(src)
	require.NoError(t, err)

	require.Len(t, assembled.CandidateNotes, 1, "one reply of three is the author's")
	offered := assembled.CandidateNotes[0]
	assert.Equal(t, "PRRT_1", offered.Thread)
	assert.Equal(t, "PRRC_2", offered.Reply.ID)
	assert.Equal(t, "Yes, free shipping was dropped last sprint.", offered.Reply.Body)
	assert.Equal(t, `cr note `+testIssue+` "<text>" --source thread --pr 7`, offered.Record)

	assert.Empty(t, assembled.Notes, "§3.6: offering a candidate records nothing")
	_, err = os.Stat(src.Layout.ContextFile(testIssue))
	assert.ErrorIs(t, err, os.ErrNotExist, "no note file is created for the issue key")
}

// A pull request whose author GitHub reports no login for offers nothing, rather
// than every reply whose account is gone as well.
func TestAnAuthorWithNoLoginIsOfferedNoCandidate(t *testing.T) {
	gone := []gh.Thread{{ID: "PRRT_1", Replies: []gh.Comment{{ID: "PRRC_2", Author: ""}}}}

	assert.Empty(t, candidateNotes(gone, "", testIssue, 7, nil))
	assert.NotNil(t, candidateNotes(gone, "", testIssue, 7, nil), "§12.3: an empty offer is [], not null")

	answered := []gh.Thread{{ID: "PRRT_1", Replies: []gh.Comment{{ID: "PRRC_2", Author: "author"}}}}
	offered := candidateNotes(answered, "author", "", 7, nil)
	require.Len(t, offered, 1)
	assert.Equal(t, `cr note <ISSUE-KEY> "<text>" --source thread --pr 7`, offered[0].Record,
		"with no key resolved the command names the key it still needs")
}

// A reply whose body a stored note already holds under §1.4's normalisation is
// not offered, and that holds for a note §3.6.6 has since retracted: offering
// it would invite storing a withdrawn fact again. A reply no note holds stays
// offered.
func TestAReplyTheStoreHoldsIsNotOffered(t *testing.T) {
	retractedAt := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	threads := []gh.Thread{{ID: "PRRT_1", Replies: []gh.Comment{
		{ID: "PRRC_2", Author: "author", Body: "Yes, free shipping was dropped."},
		{ID: "PRRC_3", Author: "author", Body: "Tax is rounded once."},
		{ID: "PRRC_4", Author: "author", Body: "Totals are cached."},
	}}}
	stored := []note.Note{
		{ID: testIssue + "#n1", Text: "Yes,  free shipping was dropped.\t"},
		{ID: testIssue + "#n2", Text: "Tax is rounded once.", RetractedAt: &retractedAt},
	}

	offered := candidateNotes(threads, "author", testIssue, 7, stored)

	require.Len(t, offered, 1)
	assert.Equal(t, "PRRC_4", offered[0].Reply.ID)
}
