package gh

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// resolveMutation is §9.6.1's write: GraphQL's resolveReviewThread over the
// thread's node id.
//
// It is GraphQL and not REST because REST has no endpoint for it — resolution
// is a review-thread concept the v3 API does not expose — and because the node
// id cr already holds is a GraphQL id. §3.5's read takes it from the same
// vocabulary, so nothing has to be looked up again to write here.
const resolveMutation = `mutation($thread: ID!) {
  resolveReviewThread(input: {threadId: $thread}) {
    thread { id isResolved }
  }
}`

// resolveResponse is the answer to resolveMutation, read only for the fact that
// GitHub agrees the thread is resolved.
//
// The state is read back rather than assumed from a 200, for §8.4's reason: a
// write whose outcome cr did not read is an unknown outcome, and a mutation
// that returned without resolving would otherwise be recorded as a resolution
// that happened.
type resolveResponse struct {
	Data struct {
		ResolveReviewThread struct {
			Thread struct {
				ID         string `json:"id"`
				IsResolved bool   `json:"isResolved"`
			} `json:"thread"`
		} `json:"resolveReviewThread"`
	} `json:"data"`
}

// NotResolvedError reports a resolve mutation GitHub answered without
// resolving the thread.
type NotResolvedError struct {
	// Thread is the node id the mutation named.
	Thread string
}

func (e *NotResolvedError) Error() string {
	return fmt.Sprintf(
		"GitHub answered the resolve of thread %s without resolving it; §9.6.1 records a "+
			"resolution only when GitHub reports one", e.Thread)
}

// ResolveThread resolves one review thread, per §9.6.1.
//
// It is behind Confirmation for the reason every other write is: §2.1.2 leaves
// one door, and resolving a thread is a change other people see. An unconfirmed
// run reaches Write and is refused there, so the gate is not re-implemented
// here.
func (c Confirmation) ResolveThread(thread string) error {
	variables, err := json.Marshal(map[string]string{"thread": thread})
	if err != nil {
		return err
	}
	out, err := c.Write(nil, "api", "graphql",
		"-f", "query="+resolveMutation,
		"--raw-field", "variables="+string(variables))
	if err != nil {
		return err
	}
	var answer resolveResponse
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		return fmt.Errorf("cannot read the resolve of thread %s: %w", thread, err)
	}
	if !answer.Data.ResolveReviewThread.Thread.IsResolved {
		return &NotResolvedError{Thread: thread}
	}
	return nil
}

// replyBody is the request body of a reply, which carries the text and nothing
// else. The endpoint takes the comment to reply to in its path.
type replyBody struct {
	Body string `json:"body"`
}

// replyResponse is the answer to a reply, read for the comment id GitHub gave
// it so §9.6.2's record can name what it posted.
type replyResponse struct {
	ID int64 `json:"id"`
}

// ReplyToComment posts one reply under an existing review comment, per §9.6.2,
// and returns the id of the comment it created.
//
// The endpoint is the replies collection of the comment being answered, which
// is what keeps the reply in the thread rather than starting a new one at the
// same line. A reply that started its own thread would read to the author as a
// second concern about code cr was withdrawing a concern about.
func (c Confirmation) ReplyToComment(owner, repo string, pr int, comment int64, body string) (int64, error) {
	payload, err := json.Marshal(replyBody{Body: body})
	if err != nil {
		return 0, err
	}
	endpoint := strings.Join([]string{
		"repos", owner, repo, "pulls", strconv.Itoa(pr),
		"comments", strconv.FormatInt(comment, 10), "replies",
	}, "/")
	out, err := c.Write(payload, "api", endpoint, "--method", "POST", "--input", "-")
	if err != nil {
		return 0, err
	}
	var answer replyResponse
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		return 0, fmt.Errorf("cannot read the reply posted to comment %d: %w", comment, err)
	}
	return answer.ID, nil
}
