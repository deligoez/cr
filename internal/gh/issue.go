package gh

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Issue is one GitHub issue's text, as §3.1's `github` tracker reads it.
type Issue struct {
	Title string
	Body  string
}

// issueResponse is the REST answer's shape. PullRequest is held raw because
// only its presence matters: GitHub answers an issue's endpoint for a pull
// request too, and the key is the one tell it gives.
type issueResponse struct {
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	PullRequest json.RawMessage `json:"pull_request"`
}

// NotAnIssueError reports an issue number that names a pull request.
//
// GitHub numbers issues and pull requests from one sequence and answers the
// issue endpoint for both, and `gh issue view` reads a pull request under an
// issue number without a word — measured 2026-09-22 on a repository with no
// issues at all, where `gh issue view 1` returned pull request #1. Read as an
// issue, a pull request's own description would become the intent the change
// is reviewed against, and the intent axis would grade the change against
// itself. So it is refused, and §11.2 codes it 1: the number is wrong, and
// nothing about the command line or the network is.
type NotAnIssueError struct {
	Owner, Repo string
	Number      int
}

func (e *NotAnIssueError) Error() string {
	return fmt.Sprintf(
		"%s/%s#%d is a pull request, not an issue: §3.1.8 reads the issue a change is reviewed against, "+
			"and a pull request's description read as one would review the change against itself",
		e.Owner, e.Repo, e.Number)
}

// Hint is §12.4's next actionable step.
func (e *NotAnIssueError) Hint() string {
	return "pass --issue with the number of the issue the pull request closes"
}

// Issue reads one issue's title and body through the REST endpoint, a GET that
// write.go's read boundary admits.
//
// It is the REST endpoint and not `gh issue view`, because the boundary admits
// `gh api` and nothing else, and because REST's answer carries the
// `pull_request` key the refusal above turns on while `gh issue view --json`
// offers no field that says so.
func (c Client) Issue(owner, repo string, number int) (Issue, error) {
	out, err := c.run("api", "repos/"+owner+"/"+repo+"/issues/"+strconv.Itoa(number))
	if err != nil {
		return Issue{}, err
	}
	var answer issueResponse
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		return Issue{}, &AnswerError{
			Owner: owner, Repo: repo, Number: number,
			Reason: fmt.Sprintf("GitHub's answer for the issue is not readable: %v", err),
		}
	}
	if len(answer.PullRequest) > 0 && string(answer.PullRequest) != "null" {
		return Issue{}, &NotAnIssueError{Owner: owner, Repo: repo, Number: number}
	}
	return Issue{Title: answer.Title, Body: answer.Body}, nil
}
