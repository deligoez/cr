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

// Sent is `rounds/<n>/posted.json` as §8.4.4 reads it back: the payload, and
// the record ids cr wrote into the same document beside it.
//
// The ids are not part of the request and could not be — Comment.Record is
// `json:"-"` because GitHub has no field for it — so a payload read back off
// disk names no record at all. §8.4.4 needs them: adopting a review as posted
// means moving the records that reached the author to `posted`, and inferring
// which those were from the comments' anchors would mark a record posted on a
// resemblance. A record wrongly marked posted takes a §9.3.6 index entry with
// it, and the next round then drops a finding nobody ever received.
//
// They live in this document rather than in one of their own for the reason
// the returned thread ids do: §2.3's table gives a round one posted.json.
type Sent struct {
	// Review is the payload as it was sent.
	Review
	// Records are the ids of the records the comments were drawn from, in
	// payload order, as cr wrote them beside the payload.
	Records []string `json:"records"`
	// Discards are the records the draft discarded when the payload was
	// built, with the disposition each was discarded under, as cr wrote
	// them beside the payload. §8.4.4's adoption waives them: the send
	// that wrote them writes their waivers only once the call has
	// succeeded, so a review adopted after an unknown outcome is the one
	// place those waivers are still owed.
	Discards []Discard `json:"discards"`
	// Outcomes are §7.3.1's outcome for every record the draft's triage
	// read when the payload was built, in the order the records arrived,
	// as cr wrote them beside the payload. §8.4.4's adoption writes their
	// events: the send writes them only once the call has succeeded, and a
	// softening is not a state the records hold, so this is the one place
	// it survives a call whose outcome cr never learned.
	Outcomes []Settlement `json:"outcomes"`
}

// Settlement is one record's §7.3.1 outcome, as posted.json names it.
type Settlement struct {
	// Record is the record's id.
	Record string `json:"record"`
	// Outcome is the outcome action the draft's triage settled on.
	Outcome finding.Outcome `json:"outcome"`
	// Kind, Grade, Severity and Anchor are the record's as the send held it
	// when the payload was built: §7.2.2's recomputed grade, the forcings
	// applied over it, and §7.2's severity and location rows. A successful
	// send stores those values; §8.4.4's adoption reads them from here,
	// because the send never stored them when its outcome was unknown.
	Kind     finding.Kind     `json:"kind"`
	Grade    finding.Grade    `json:"grade"`
	Severity finding.Severity `json:"severity"`
	Anchor   finding.Anchor   `json:"anchor"`
}

// Discard is one record the draft discarded, as posted.json names it.
type Discard struct {
	// Record is the record's id.
	Record string `json:"record"`
	// Disposition is the §7.2 verb it was discarded under, which decides
	// the scope of its waiver.
	Disposition finding.Disposition `json:"disposition"`
}

// Decode reads `rounds/<n>/posted.json` back.
//
// §8.4.4 is what needs it: the hash a reconciliation matches is the hash of
// what was sent, and what was sent is the document §8.3.3 wrote before the call
// — so the recovery reads that file rather than rebuilding the round out of
// whatever the draft and the records hold now.
//
// A field the document carries and this type does not is dropped, which is the
// reading the round needs: §8.3.3 has the returned thread ids added to the same
// document afterwards, and a decoder that refused them would refuse every
// payload whose call succeeded.
func Decode(payload []byte) (*Sent, error) {
	var sent Sent
	if err := json.Unmarshal(payload, &sent); err != nil {
		return nil, err
	}
	return &sent, nil
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
