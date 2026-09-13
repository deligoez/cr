package gh

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// Comment is one comment of a review thread: its author and its body, with the
// identity and time that let a later round tell it from a new one.
type Comment struct {
	// ID is the comment's GitHub node id.
	ID string `json:"id"`
	// Author is the login that wrote it, empty when the account is gone.
	Author string `json:"author"`
	// AuthorTypename is the GraphQL type of the author — `User`, `Bot`,
	// `Organization`, or `Mannequin` — verbatim, and empty when the
	// account is gone. It is the raw fact the API reports and not the
	// tag: §3.5.2's `human` or `bot` is a judgement about this value,
	// and it is recorded separately as Thread.AuthorType.
	AuthorTypename string `json:"author_typename"`
	// Body is the comment's text as it was written.
	Body string `json:"body"`
	// CreatedAt is when it was posted, in RFC 3339.
	CreatedAt string `json:"created_at"`
	// URL is where a human can read it.
	URL string `json:"url"`
	// Review is the node id of the review the comment was posted in, and
	// empty when GitHub names none. It is the id GitHub's review-creation
	// call answers as `node_id` and §8.4.4's review listing reads as `id`,
	// so §8.3.3's read-back can tell the threads of the review a round
	// created from threads an earlier round's review opened with the same
	// words at the same place.
	Review string `json:"review"`
}

// Anchor is where a thread sits in the diff.
//
// It is the shape §3.5.3's comparison against a unit's hunks needs rather than
// the shape the API answers in, because a raw payload would push the same
// normalisation into every caller and one of them would get it wrong.
type Anchor struct {
	// Path is the file the thread hangs on.
	Path string `json:"path"`
	// Side is the file version Line is numbered in, §9.2's `RIGHT` or
	// `LEFT`. It is git.Side so that §3.5.3 compares it against a hunk's
	// side without a conversion that could invert it.
	Side git.Side `json:"side"`
	// StartLine is the first line of the thread's range at the current
	// head, equal to Line for a thread covering one line.
	StartLine int `json:"start_line"`
	// Line is the last line of that range, and zero when the head no
	// longer carries the code the thread hangs on.
	Line int `json:"line"`
	// OriginalStartLine is StartLine's counterpart in the diff the thread
	// was written against.
	OriginalStartLine int `json:"original_start_line"`
	// OriginalLine is Line's counterpart there. It survives a head change
	// that leaves Line zero, so an outdated thread still names a place.
	OriginalLine int `json:"original_line"`
}

// AuthorType is §3.5.2's tag over a thread's author: `human` or `bot`, and
// nothing else.
//
// It is a judgement about Comment.AuthorTypename rather than a copy of it, so
// it gets its own field: the raw value is what GitHub answered, and the tag is
// what §3.5.3 filters on.
type AuthorType string

const (
	// AuthorHuman tags a thread cr has not been told is a bot.
	AuthorHuman AuthorType = "human"
	// AuthorBot tags a thread GitHub itself reports a bot wrote.
	AuthorBot AuthorType = "bot"
)

// botTypename is the one GraphQL author type that means a bot. GitHub answers
// it for a GitHub App's own identity and for nothing else.
const botTypename = "Bot"

// Thread is one existing review thread on the pull request under review, with
// everything §3.5.1 requires ingestion to carry: author, body, anchor,
// resolution state, and replies.
//
// The author and the body are the opening comment's, so they are held as that
// comment rather than copied out of it. A thread is its first comment plus the
// replies to it, and splitting the pair across three fields would leave the
// opener without the id, time, and author type every reply keeps.
type Thread struct {
	// ID is the thread's GitHub node id. §3.5.4 records it as
	// `suppressed_by` on a finding an ingested thread already covers.
	ID string `json:"id"`
	// Anchor is where the thread hangs.
	Anchor Anchor `json:"anchor"`
	// Resolved is the resolution state §3.5.1 requires. A resolved thread
	// is ingested exactly like an unresolved one; the state is recorded,
	// never used to drop it.
	Resolved bool `json:"resolved"`
	// Outdated reports that the head has moved past the thread's code.
	// GitHub then reports no current line, so Anchor.Line is zero.
	Outdated bool `json:"outdated"`
	// Comment is the opening comment, which carries the thread's author
	// and body.
	Comment Comment `json:"comment"`
	// AuthorType is §3.5.2's tag over that author. It is the thread's,
	// not the reply's: a thread is attached or not as a whole, so who
	// opened it decides.
	AuthorType AuthorType `json:"author_type"`
	// Replies are the comments answering it, oldest first. §3.5.5 offers
	// the author's among them as candidate context notes.
	Replies []Comment `json:"replies"`
}

// Client reads GitHub through a Runner.
type Client struct {
	run Runner
}

// New returns a Client that reads through the gh binary.
func New() Client { return Client{run: Run} }

// WithRunner returns a Client that reads through r instead of the gh binary,
// so a test can drive ingestion from a recorded payload.
func WithRunner(r Runner) Client { return Client{run: r} }

// Page sizes for the two connections ingestion walks. Both are GitHub's
// maximum: a page is a network round trip, and the pull requests that carry
// enough threads to need a second one are the ones a reviewer most needs read
// whole.
const (
	threadPageSize  = 100
	commentPageSize = 100
)

// threadsQuery reads one page of review threads with one page of each
// thread's comments.
//
// It is GraphQL and not REST because §3.5.1 requires the resolution state of
// every thread, and REST's pull request review comments carry no such field:
// resolution lives on the GraphQL PullRequestReviewThread and nowhere else.
// Inferring it from anything REST does report would be a guess about whether
// a colleague has already dealt with a concern, and §3.5.4 lets that guess
// suppress a finding.
const threadsQuery = `query($owner:String!,$repo:String!,$number:Int!,$threads:Int!,$comments:Int!,$cursor:String){
  repository(owner:$owner,name:$repo){
    pullRequest(number:$number){
      reviewThreads(first:$threads,after:$cursor){
        pageInfo{hasNextPage endCursor}
        nodes{
          id isResolved isOutdated path line startLine originalLine originalStartLine diffSide
          comments(first:$comments){
            pageInfo{hasNextPage endCursor}
            nodes{id url body createdAt author{__typename login} pullRequestReview{id}}
          }
        }
      }
    }
  }
}`

// repliesQuery reads a further page of one thread's comments.
const repliesQuery = `query($thread:ID!,$comments:Int!,$cursor:String){
  node(id:$thread){
    ... on PullRequestReviewThread{
      comments(first:$comments,after:$cursor){
        pageInfo{hasNextPage endCursor}
        nodes{id url body createdAt author{__typename login} pullRequestReview{id}}
      }
    }
  }
}`

// pageInfo is a connection's cursor pair.
type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// commentConnection is one page of a thread's comments.
type commentConnection struct {
	PageInfo pageInfo      `json:"pageInfo"`
	Nodes    []commentNode `json:"nodes"`
}

// commentNode is a comment as the API answers it.
type commentNode struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	// Author is null when the account that wrote the comment is gone,
	// which is why it is a pointer: a deleted author must not take the
	// comment's body out of the ingest with it.
	Author *struct {
		Typename string `json:"__typename"`
		Login    string `json:"login"`
	} `json:"author"`
	// PullRequestReview is null when GitHub names no review for the
	// comment, which the schema allows, and a missing review must not take
	// the comment out of the ingest either.
	PullRequestReview *struct {
		ID string `json:"id"`
	} `json:"pullRequestReview"`
}

// threadNode is a review thread as the API answers it. Every line is a
// pointer because GitHub reports null for each of them on a thread whose code
// the head no longer carries, and for the start of a single-line thread.
type threadNode struct {
	ID                string            `json:"id"`
	IsResolved        bool              `json:"isResolved"`
	IsOutdated        bool              `json:"isOutdated"`
	Path              string            `json:"path"`
	Line              *int              `json:"line"`
	StartLine         *int              `json:"startLine"`
	OriginalLine      *int              `json:"originalLine"`
	OriginalStartLine *int              `json:"originalStartLine"`
	DiffSide          string            `json:"diffSide"`
	Comments          commentConnection `json:"comments"`
}

// threadsResponse is the answer to threadsQuery.
type threadsResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					PageInfo pageInfo     `json:"pageInfo"`
					Nodes    []threadNode `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// repliesResponse is the answer to repliesQuery.
type repliesResponse struct {
	Data struct {
		Node struct {
			Comments commentConnection `json:"comments"`
		} `json:"node"`
	} `json:"data"`
}

// Threads ingests every existing review thread on the pull request, resolved
// ones included, per §3.5.1.
//
// Nothing is filtered out here. §3.5.3 forbids cr to classify an ingested
// thread or to decide suppression, so deciding which threads are worth
// carrying is not ingestion's business either: the whole set is recorded and
// the agent is shown what it asks for.
func (c Client) Threads(owner, repo string, number int) ([]Thread, error) {
	threads := make([]Thread, 0)
	cursor := ""
	for {
		var page threadsResponse
		args := []string{
			"api", "graphql",
			// -f sends every value as the string it is. -F
			// converts a value that looks like a number or a
			// boolean, so an owner or a repository named for one
			// would arrive as the wrong GraphQL type, and a value
			// beginning with @ would be read as a file name.
			"-f", "query=" + threadsQuery,
			"-f", "owner=" + owner,
			"-f", "repo=" + repo,
			"-F", "number=" + strconv.Itoa(number),
			"-F", "threads=" + strconv.Itoa(threadPageSize),
			"-F", "comments=" + strconv.Itoa(commentPageSize),
		}
		if cursor != "" {
			args = append(args, "-f", "cursor="+cursor)
		}
		if err := c.query(&page, args); err != nil {
			return nil, err
		}
		connection := page.Data.Repository.PullRequest.ReviewThreads
		for i := range connection.Nodes {
			thread, err := c.thread(&connection.Nodes[i])
			if err != nil {
				return nil, err
			}
			threads = append(threads, thread)
		}
		if !connection.PageInfo.HasNextPage {
			return threads, nil
		}
		if connection.PageInfo.EndCursor == "" {
			return nil, fmt.Errorf(
				"%s/%s#%d: a page of review threads claims a next page and names no cursor",
				owner, repo, number,
			)
		}
		cursor = connection.PageInfo.EndCursor
	}
}

// thread builds one ingested thread from the node the API answered with.
//
// A review thread on GitHub is created by its opening comment and so always
// has one, but the guard below is not decoration: an answer carrying none
// would index past the end of the slice, and an ingest that panics takes the
// whole round down over a thread nobody can read anyway.
func (c Client) thread(node *threadNode) (Thread, error) {
	comments, err := c.comments(node)
	if err != nil {
		return Thread{}, err
	}
	anchor, err := node.anchor()
	if err != nil {
		return Thread{}, err
	}
	thread := Thread{
		ID:       node.ID,
		Anchor:   anchor,
		Resolved: node.IsResolved,
		Outdated: node.IsOutdated,
		Replies:  make([]Comment, 0),
	}
	if len(comments) > 0 {
		thread.Comment = comments[0]
		thread.Replies = append(thread.Replies, comments[1:]...)
	}
	thread.AuthorType = authorType(thread.Comment.AuthorTypename)
	return thread, nil
}

// comments returns every comment of one thread, following the connection past
// the page that arrived with the thread.
//
// A thread long enough to need a second page is rare and is exactly the thread
// whose replies decide something, so the further pages are read rather than
// dropped: §3.5.1 requires the replies, and §3.5.5 offers the author's among
// them as candidate context notes.
func (c Client) comments(node *threadNode) ([]Comment, error) {
	comments := make([]Comment, 0, len(node.Comments.Nodes))
	connection := node.Comments
	for {
		for _, reply := range connection.Nodes {
			comments = append(comments, reply.comment())
		}
		if !connection.PageInfo.HasNextPage {
			return comments, nil
		}
		if connection.PageInfo.EndCursor == "" {
			return nil, fmt.Errorf(
				"thread %s: a page of comments claims a next page and names no cursor",
				node.ID,
			)
		}
		var page repliesResponse
		args := []string{
			"api", "graphql",
			"-f", "query=" + repliesQuery,
			"-f", "thread=" + node.ID,
			"-F", "comments=" + strconv.Itoa(commentPageSize),
			"-f", "cursor=" + connection.PageInfo.EndCursor,
		}
		if err := c.query(&page, args); err != nil {
			return nil, err
		}
		connection = page.Data.Node.Comments
	}
}

// query runs one gh invocation and decodes its answer into out.
func (c Client) query(out any, args []string) error {
	body, err := c.run(args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(body), out); err != nil {
		return fmt.Errorf("cannot read the answer to `gh api graphql`: %w", err)
	}
	return nil
}

// comment converts one comment of the answer into the record that is stored.
func (n *commentNode) comment() Comment {
	comment := Comment{
		ID:        n.ID,
		Body:      n.Body,
		CreatedAt: n.CreatedAt,
		URL:       n.URL,
	}
	if n.Author != nil {
		comment.Author, comment.AuthorTypename = n.Author.Login, n.Author.Typename
	}
	if n.PullRequestReview != nil {
		comment.Review = n.PullRequestReview.ID
	}
	return comment
}

// authorType tags one thread's author `human` or `bot`, per §3.5.2, from the
// GraphQL author type its opening comment arrived with.
//
// The judgement is GitHub's own and cr adds nothing to it: the API answers
// `Bot` for a GitHub App's identity and for nothing else, and that answer
// alone makes the tag `bot`. A `User`, an `Organization`, a `Mannequin`, and
// an author the API answers null for because the account is gone are all
// `human`.
//
// That leaves `dependabot[bot]`, `github-actions[bot]`, and every CI account
// somebody made by hand tagged `human`, because GitHub reports them as `User`.
// Reading the `[bot]` suffix or carrying a login allowlist would catch them,
// and cr does neither. It would be cr deciding whose comment counts, which is
// the shape of judgement §2.1 leaves to the agent, and it errs in the
// expensive direction: a bot tagged `human` costs the agent one comment to
// read and discard, while a human tagged `bot` drops the thread §3.5.3 would
// have attached, so §3.5.4 never suppresses against it and cr raises a concern
// a colleague already raised. The agent sees the login and the raw typename on
// every thread and can tell a bot from a person itself.
func authorType(typename string) AuthorType {
	if typename == botTypename {
		return AuthorBot
	}
	return AuthorHuman
}

// anchor normalises a thread's position into the range a containment test can
// use unchanged, and refuses a position cr cannot place.
//
// GitHub reports a range as a start and an end and answers null for the start
// of a thread covering one line, so a missing start becomes the end and every
// anchor carries both. A thread whose code the head no longer holds has no
// current position at all — line and startLine are both null — and keeps only
// the original pair. Line is then zero, and no file has a line zero, so
// §3.5.3 can tell a thread that still resolves from one that does not without
// consulting anything else.
//
// The side is the one field of the answer that is not taken on trust. GitHub
// declares `diffSide` non-null over exactly RIGHT and LEFT, and §9.2 closes the
// same two, so a third value — or none, which is what an answer missing the
// field decodes to — means this is not the answer the query asked for.
// Converting it unchanged would put a side on the thread that matches no hunk:
// §3.4.4 partitions a unit's hunks by side, so §3.5.3 would attach the thread
// to no unit, §3.5.4 would suppress nothing against it, and cr would raise a
// concern a colleague has already raised. That is the expensive direction of
// wrong, and nothing in the run would say it had happened.
//
// So ingestion refuses rather than drops the thread. Dropping it leaves exactly
// the same silence with the fault hidden as well, and §3.5.1 admits no
// exception in any case: every existing thread is ingested. A refusal names the
// thread and quotes the value back, which is a failure a human can act on.
func (n *threadNode) anchor() (Anchor, error) {
	side, known := git.ParseSide(n.DiffSide)
	if !known {
		return Anchor{}, fmt.Errorf(
			"thread %s: GitHub answered diffSide %q, and §9.2's sides are %s and %s",
			n.ID, n.DiffSide, git.Right, git.Left,
		)
	}
	line := lineOr(n.Line, 0)
	original := lineOr(n.OriginalLine, 0)
	return Anchor{
		Path:              n.Path,
		Side:              side,
		StartLine:         lineOr(n.StartLine, line),
		Line:              line,
		OriginalStartLine: lineOr(n.OriginalStartLine, original),
		OriginalLine:      original,
	}, nil
}

// lineOr reads a line the API may have answered null for.
func lineOr(reported *int, absent int) int {
	if reported == nil {
		return absent
	}
	return *reported
}

// WriteThreads stores the ingested threads in threads.ndjson (§2.3).
//
// §2.3.3 does not list that file among the eight whose records carry head and
// round, so the write goes through WriteRecords, which refuses those eight.
// The file is named here and nowhere else, so a command that ingests threads
// cannot put them anywhere but where the §2.3 table says they live.
func WriteThreads(k *state.Lock, threads []Thread) error {
	return state.WriteRecords(k, state.FileThreads, threads)
}

// ReadThreads reads the ingested threads back. It takes no lock, per §2.3.2.
func ReadThreads(l state.Layout, owner, repo string, pr int) ([]Thread, error) {
	return state.ReadRecords[Thread](l, owner, repo, pr, state.FileThreads)
}
