package gh

import (
	"fmt"
	"strconv"
)

// PullRequest is the pull request under review as §3.7.1 identifies it, and as
// §3.2 resolves an issue key out of.
//
// The two revisions are object ids rather than ref names, and that is the whole
// reason this type exists instead of a pair of `git rev-parse` calls. §3.4.1
// takes the diff against the merge base "at the current head", and the current
// head is GitHub's answer about the pull request: a local branch of the same
// name may sit anywhere, and on a fork it names a different history altogether.
// §9.3.1 then compares this value against `meta.json`'s recorded head, so a head
// read from the local checkout would make the round's staleness a property of
// what the user happened to have checked out.
//
// The cost is that both commits must already be in the local repository. cr
// cannot fetch them: §2.2 permits no write inside the repository under review
// beyond §5.1's worktree registration, and a fetch writes refs and objects. A
// revision the checkout does not hold fails as the git read it is.
type PullRequest struct {
	// Number is the pull request number, as GitHub reports it.
	Number int `json:"number"`
	// Title is §3.2's third key source.
	Title string `json:"title"`
	// Body is §3.2's fourth.
	Body string `json:"body"`
	// HeadRefName is the head branch name, §3.2's second source.
	HeadRefName string `json:"head_ref_name"`
	// Head is the head commit, which §3.7.1 prints and §9.3.1 compares
	// against the recorded head.
	Head string `json:"head"`
	// BaseRefName is the branch the pull request targets.
	BaseRefName string `json:"base_ref_name"`
	// Base is that branch's commit, which §3.4.1's merge base is taken
	// against.
	Base string `json:"base"`
	// Author is the login that opened the pull request, empty when the
	// account is gone. §3.5.5 offers that login's replies inside the
	// ingested threads as candidate context notes.
	Author string `json:"author"`
	// State is GitHub's PullRequestState for it: OPEN, CLOSED or MERGED.
	State string `json:"state"`
	// ClosedAt and MergedAt are the times GitHub reports the pull request
	// closed and merged, as it writes them, and empty while it has not.
	ClosedAt string `json:"closed_at"`
	MergedAt string `json:"merged_at"`
}

// The PullRequestState values GitHub answers for a pull request that is no
// longer open.
const (
	StateClosed = "CLOSED"
	StateMerged = "MERGED"
)

// Closure is the state and the time GitHub reports for a pull request that is
// no longer open, and ok false for one that is open or whose state GitHub did
// not answer.
func (p *PullRequest) Closure() (state, at string, ok bool) {
	switch p.State {
	case StateMerged:
		return "merged", p.MergedAt, true
	case StateClosed:
		return "closed", p.ClosedAt, true
	}
	return "", "", false
}

// AnswerError reports a `gh api graphql` call that ran, returned zero, and
// answered something cr cannot use.
//
// §3.1.3 fixes the code for an external command that fails, and this is the
// same failure one step later: gh ran and GitHub answered, but the answer names
// no pull request or omits a revision GitHub declares non-null. §11.2 codes it
// 3 beside CommandError. It is not a usage error, and being one is what it was
// until unreadable-input-exit-code — measured 2026-09-13 on the built binary
// against a local `gh` answering `{}`, `cr record 1 <file> --repo o/r` printed
// `o/r#1: GitHub answered no pull request` and exited 2, which told the reader
// to retype a command line that was right.
type AnswerError struct {
	// Owner, Repo and Number are the pull request that was asked about.
	Owner  string
	Repo   string
	Number int
	// Reason is what was wrong with the answer.
	Reason string
}

func (e *AnswerError) Error() string {
	return fmt.Sprintf("%s/%s#%d: %s", e.Owner, e.Repo, e.Number, e.Reason)
}

// pullRequestQuery reads the identity of one pull request.
//
// It is GraphQL rather than `gh pr view` because the read boundary in write.go
// admits `gh api` and nothing else: a subcommand allowlist that grew every time
// a read was needed would be an allowlist that goes stale in the direction that
// posts something. The document opens with `query`, which is what the boundary
// reads it by.
const pullRequestQuery = `query($owner:String!,$repo:String!,$number:Int!){
  repository(owner:$owner,name:$repo){
    pullRequest(number:$number){
      number title body headRefName headRefOid baseRefName baseRefOid author{login}
      state closedAt mergedAt
    }
  }
}`

// pullRequestNode is the answer's shape, in GitHub's own field names.
type pullRequestNode struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	BaseRefName string `json:"baseRefName"`
	BaseRefOid  string `json:"baseRefOid"`
	State       string `json:"state"`
	// ClosedAt and MergedAt are null while the pull request is open, which
	// decodes as the empty string.
	ClosedAt string `json:"closedAt"`
	MergedAt string `json:"mergedAt"`
	// Author is null when the account that opened the pull request is
	// gone, as a comment's is.
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
}

// pullRequestResponse wraps that node the way the API answers it. The pull
// request is a pointer so an answer naming none is a nil rather than a zero
// value indistinguishable from a pull request with an empty title.
type pullRequestResponse struct {
	Data struct {
		Repository struct {
			PullRequest *pullRequestNode `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// PullRequest reads the identity of one pull request.
//
// The two revisions are checked rather than taken on trust, for the reason
// threadNode.anchor checks `diffSide`: GitHub declares both non-null, so an
// empty one means this is not the answer the query asked for. An empty head
// would be recorded in meta.json as the round's head and every §9.3.1
// comparison afterwards would run against it, which is a wrong answer that
// looks like a right one.
func (c Client) PullRequest(owner, repo string, number int) (PullRequest, error) {
	var answer pullRequestResponse
	args := []string{
		"api", "graphql",
		// -f rather than -F for the owner and the repository, for the
		// reason Threads gives: -F would convert a name that looks
		// like a number to the wrong GraphQL type.
		"-f", "query=" + pullRequestQuery,
		"-f", "owner=" + owner,
		"-f", "repo=" + repo,
		"-F", "number=" + strconv.Itoa(number),
	}
	if err := c.query(&answer, args); err != nil {
		return PullRequest{}, err
	}
	node := answer.Data.Repository.PullRequest
	if node == nil {
		return PullRequest{}, &AnswerError{
			Owner: owner, Repo: repo, Number: number,
			Reason: "GitHub answered no pull request",
		}
	}
	if node.HeadRefOid == "" || node.BaseRefOid == "" {
		return PullRequest{}, &AnswerError{
			Owner: owner, Repo: repo, Number: number,
			Reason: fmt.Sprintf(
				"GitHub answered head %q and base %q, and §3.4.1 needs both "+
					"commits to take the diff", node.HeadRefOid, node.BaseRefOid),
		}
	}
	opened := PullRequest{
		Number:      node.Number,
		Title:       node.Title,
		Body:        node.Body,
		HeadRefName: node.HeadRefName,
		Head:        node.HeadRefOid,
		BaseRefName: node.BaseRefName,
		Base:        node.BaseRefOid,
		State:       node.State,
		ClosedAt:    node.ClosedAt,
		MergedAt:    node.MergedAt,
	}
	if node.Author != nil {
		opened.Author = node.Author.Login
	}
	return opened, nil
}
