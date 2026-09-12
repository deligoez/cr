// Package post builds the one review §8.3 has a round reach the author as.
//
// §8.3.1 is a sentence about a person, not about an API: all comments in a
// round are posted as one GitHub review so the author receives a single
// notification. Every comment cr has queued for a round is therefore one
// argument vector, one call, and one notification — and the shape of this
// package is what makes a second call awkward to write rather than merely
// discouraged. Create takes the whole review and returns one result; there is
// no per-comment door.
//
// The sending is not decided here. §8.5 mints the confirmation, and this
// package asks for it as a Sender it cannot construct: the only value in cr
// that satisfies the interface is a gh.Confirmation, and one built anywhere but
// §8.5's gate is the zero value that writes nothing.
package post

import (
	"encoding/json"
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
)

// event is the review event §8.3.2 fixes for v0.1.
//
// It is an unexported constant with no setter, which is how "cr MUST NOT emit
// APPROVE or REQUEST_CHANGES" is kept true. A parameter would be a value some
// later caller could choose, and a review event is not a detail the caller
// gets an opinion about: APPROVE and REQUEST_CHANGES are verdicts on the change
// as a whole, and P5 leaves cr no way to form one.
const event = "COMMENT"

// Comment is one review comment of the payload, in the fields GitHub's
// review-creation endpoint names them by.
type Comment struct {
	// Record is the id of the record the comment was drawn from. It is not
	// part of the request — GitHub has no field for it — and it is carried
	// so §8.4.2 can name the record behind a position GitHub rejected.
	Record string `json:"-"`
	// Path is the file the comment is on.
	Path string `json:"path"`
	// StartLine is the first line of a multi-line comment, left out when
	// the comment is on one line, which is how GitHub tells the two apart.
	StartLine int `json:"start_line,omitempty"`
	// StartSide is the side StartLine is numbered on, and travels with it.
	StartSide git.Side `json:"start_side,omitempty"`
	// Line is the comment's last line, and its only line when StartLine is
	// absent.
	Line int `json:"line"`
	// Side is the side Line is numbered on, per §9.2.
	Side git.Side `json:"side"`
	// Body is the text the author reads, as §8.1 renders it.
	Body string `json:"body"`
}

// Review is a round's single review: every comment it holds, and the review's
// own body.
type Review struct {
	// Event is §8.3.2's, written from the constant above and never from a
	// caller. It is a field because the request document has one.
	Event string `json:"event"`
	// Body is the review's own body, which §8.4.3 fills with §4.5.4's
	// disclosure and the payload hash. It is not a line comment.
	Body string `json:"body"`
	// Comments are the round's comments, in the order they were queued.
	Comments []Comment `json:"comments"`
}

// Build assembles the round's one review from the records it queued and the
// bodies §8.1.2 produced for them, keyed by record id.
//
// Every queued record becomes exactly one comment, in the order given, and a
// record whose body is missing becomes a comment with an empty one rather than
// an error: §8.1.3's refusal of an empty body is its own step and names the
// record, and answering it here would replace a refusal that says what is
// wrong with a payload that is quietly one comment short.
//
// The anchor is copied rather than interpreted. §9.2 numbers a RIGHT anchor in
// the head and a LEFT one in the merge base, and GitHub's `side` is that same
// distinction, so the only work is the one GitHub reserves for itself: a
// single-line comment carries no `start_line`, and a multi-line one carries it
// beside the side it is numbered on.
func Build(records []*finding.Finding, bodies map[string]string) *Review {
	review := &Review{Event: event, Comments: make([]Comment, 0, len(records))}
	for _, record := range records {
		comment := Comment{
			Record: record.ID,
			Path:   record.Anchor.Path,
			Line:   record.Anchor.Line,
			Side:   record.Anchor.Side,
			Body:   bodies[record.ID],
		}
		if record.Anchor.StartLine > 0 && record.Anchor.StartLine < record.Anchor.Line {
			comment.StartLine, comment.StartSide = record.Anchor.StartLine, record.Anchor.Side
		}
		review.Comments = append(review.Comments, comment)
	}
	return review
}

// Payload marshals the review as the request body of the review-creation call,
// indented the way cr indents every document it writes.
//
// The event is written here as well as in Build, because this is the last point
// before the bytes exist: a Review assembled by hand, or one whose field was
// overwritten between building and sending, still leaves with §8.3.2's value.
func (r *Review) Payload() ([]byte, error) {
	r.Event = event
	return json.MarshalIndent(r, "", "  ")
}

// Sender is the write door of internal/gh, as this package uses it.
//
// It is an interface so the door is named rather than opened here: the only
// value in cr satisfying it is a gh.Confirmation, which §8.5's gate alone can
// mint, and a Confirmation built anywhere else is the zero value whose Write
// refuses. Taking the door as a parameter also keeps this package free of the
// mint, so nothing here can supply the confirmation §8.5.3 requires the flag
// to supply.
type Sender interface {
	Write(args ...string) (string, error)
}

// Request is the one `gh api` invocation that creates a round's review.
//
// The body is read from a file rather than built into the argument vector,
// because §8.3.3 has the exact payload written to `rounds/<n>/posted.json`
// before the network call — so the bytes that were written are the bytes that
// are sent, with no second serialisation between them that could differ.
func Request(owner, repo string, pr int, payload string) []string {
	return []string{
		"api",
		fmt.Sprintf("repos/%s/%s/pulls/%d/reviews", owner, repo, pr),
		"--method", "POST",
		"--input", payload,
	}
}

// Create posts the round's review in one call, whatever the round holds.
//
// The comment count reaches the call as the payload's own length and never as
// a number of calls, which is §8.3.1: one review is one notification, and a
// round posting its comments one at a time would notify the author once per
// finding. It is also what §8.4.1's atomicity rests on — the review is created
// with all its comments or nothing is created — which a sequence of calls
// cannot have at all.
func Create(sender Sender, owner, repo string, pr int, payload string) (string, error) {
	return sender.Write(Request(owner, repo, pr, payload)...)
}
