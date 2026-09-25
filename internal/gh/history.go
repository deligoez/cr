package gh

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// HistoryComment is one pull request review comment of a repository, as
// §2.6.3.5's harvest reads it: who wrote it and as what kind of account, where
// it hangs, what it says, and which comment it answers.
//
// It is the REST shape and not the GraphQL one Thread is built from. §2.6.3.5
// reads a repository's comments across every pull request, and REST's
// repository-wide listing is the one read that answers that without first
// listing the pull requests; what it lacks, resolution state, §2.6.3 does not
// ask for.
type HistoryComment struct {
	// ID is GitHub's numeric id for the comment, which a reply's InReplyTo
	// names.
	ID int64 `json:"id"`
	// PR is the number of the pull request the comment was posted on.
	PR int `json:"pr"`
	// URL is where a human can read it.
	URL string `json:"url"`
	// Author is the login that wrote it, empty when the account is gone.
	Author string `json:"author"`
	// AuthorType is §3.5.2's tag over the author, the same judgement a
	// thread's author receives: `bot` exactly when GitHub reports the
	// account's type as `Bot`.
	AuthorType AuthorType `json:"author_type"`
	// CreatedAt is when it was posted, in RFC 3339 as GitHub writes it.
	CreatedAt string `json:"created_at"`
	// Path, Line and Side are where it hangs. Line falls back to the
	// original line when GitHub reports no current one, so an outdated
	// comment still names a place.
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side"`
	// Body is the comment's text as it was written.
	Body string `json:"body"`
	// InReplyTo is the id of the comment this one answers, zero for a
	// comment that opens a thread.
	InReplyTo int64 `json:"in_reply_to"`
}

// historyPageSize is GitHub's maximum page for the listing.
const historyPageSize = 100

// historyNode is one comment of the REST answer, in GitHub's own field names.
type historyNode struct {
	ID             int64  `json:"id"`
	HTMLURL        string `json:"html_url"`
	PullRequestURL string `json:"pull_request_url"`
	CreatedAt      string `json:"created_at"`
	Path           string `json:"path"`
	Line           *int   `json:"line"`
	OriginalLine   *int   `json:"original_line"`
	Side           string `json:"side"`
	Body           string `json:"body"`
	InReplyToID    int64  `json:"in_reply_to_id"`
	// User is null when the account is gone, as a GraphQL author is.
	User *struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"user"`
}

// ReviewCommentsPage reads one page of a repository's pull request review
// comments, newest first by creation time, which is the order §2.6.3.5 reads
// them in. page counts from 1; a page shorter than the page size is the last.
//
// It is a GET with the query in the endpoint and no field flag, which is what
// write.go's read boundary admits: a field flag would make gh send a POST.
// GitHub's own `since` parameter is not used, because it filters on
// `updated_at` rather than on `created_at`, and §2.6.3.5's window is the
// latter.
func (c Client) ReviewCommentsPage(owner, repo string, page int) (comments []HistoryComment, last bool, err error) {
	out, err := c.run("api", "repos/"+owner+"/"+repo+"/pulls/comments?sort=created&direction=desc&per_page="+
		strconv.Itoa(historyPageSize)+"&page="+strconv.Itoa(page))
	if err != nil {
		return nil, false, err
	}
	var nodes []historyNode
	if err := json.Unmarshal([]byte(out), &nodes); err != nil {
		return nil, false, fmt.Errorf("cannot read the answer to `gh api repos/%s/%s/pulls/comments`: %w",
			owner, repo, err)
	}
	comments = make([]HistoryComment, 0, len(nodes))
	for i := range nodes {
		comment, err := nodes[i].comment()
		if err != nil {
			return nil, false, err
		}
		comments = append(comments, comment)
	}
	return comments, len(nodes) < historyPageSize, nil
}

// comment converts one node of the answer, refusing one whose pull request it
// cannot name: a comment attributed to no pull request can be neither
// grouped by distinct pull requests nor compared against its author.
func (n *historyNode) comment() (HistoryComment, error) {
	number := n.PullRequestURL[strings.LastIndex(n.PullRequestURL, "/")+1:]
	pr, err := strconv.Atoi(number)
	if err != nil || pr < 1 {
		return HistoryComment{}, fmt.Errorf(
			"review comment %d names no pull request GitHub numbers: pull_request_url is %q", n.ID, n.PullRequestURL)
	}
	comment := HistoryComment{
		ID: n.ID, PR: pr, URL: n.HTMLURL, CreatedAt: n.CreatedAt, AuthorType: AuthorHuman,
		Path: n.Path, Line: lineOr(n.Line, lineOr(n.OriginalLine, 0)), Side: n.Side,
		Body: n.Body, InReplyTo: n.InReplyToID,
	}
	if n.User != nil {
		comment.Author, comment.AuthorType = n.User.Login, authorType(n.User.Type)
	}
	return comment, nil
}

// PullAuthor reads the login that opened one pull request, empty when the
// account is gone. It is the REST pull request read, a GET the read boundary
// admits, and it asks for nothing else §2.6.3.6 needs.
func (c Client) PullAuthor(owner, repo string, number int) (string, error) {
	out, err := c.run("api", "repos/"+owner+"/"+repo+"/pulls/"+strconv.Itoa(number))
	if err != nil {
		return "", err
	}
	var answer struct {
		Number int `json:"number"`
		User   *struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		return "", &AnswerError{
			Owner: owner, Repo: repo, Number: number,
			Reason: fmt.Sprintf("GitHub's answer for the pull request is not readable: %v", err),
		}
	}
	if answer.Number != number {
		return "", &AnswerError{Owner: owner, Repo: repo, Number: number, Reason: "GitHub answered no pull request"}
	}
	if answer.User == nil {
		return "", nil
	}
	return answer.User.Login, nil
}
