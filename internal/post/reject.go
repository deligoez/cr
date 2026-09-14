package post

import (
	"encoding/json"
	"strconv"
	"strings"
)

// InvalidPosition is one position §8.4.2 reports: something GitHub named
// invalid, and the record of the payload it belongs to.
type InvalidPosition struct {
	// Record is the id of the record the position was drawn from, which
	// §8.4.2 requires to be named beside it.
	Record string
	// Path, StartLine and Line are the position as the payload carried
	// it, so the reviewer can open the file GitHub refused rather than
	// work out which comment a field name was about. StartLine is zero
	// for a comment on one line, as the payload leaves it out.
	Path      string
	StartLine int
	Line      int
	// Field is the field GitHub named, when it named one.
	Field string
	// Message is GitHub's own words, carried verbatim: what the API
	// objected to is its answer to give, not cr's to paraphrase.
	Message string
}

// String is one line of the refusal: the record, its position, and what GitHub
// said about it.
func (p InvalidPosition) String() string {
	said := p.Message
	if p.Field != "" {
		said = p.Field + ": " + said
	}
	return p.Record + " " + p.Path + ":" + Lines(p.StartLine, p.Line) + ": " + strings.TrimSpace(said)
}

// Lines names a comment's lines the way a reader opens them: `33-35` for a
// comment spanning three lines, and `35` for one on a single line, where the
// payload carries no start line.
func Lines(start, line int) string {
	if start <= 0 || start >= line {
		return strconv.Itoa(line)
	}
	return strconv.Itoa(start) + "-" + strconv.Itoa(line)
}

// RejectedError is §8.4.2's refusal: the review-creation call was rejected, so
// nothing was created and nothing is marked posted.
//
// It is a type of its own rather than the gh.CommandError underneath it, and
// deliberately does not wrap one: §3.1.3 codes an external command failure 3,
// and §8.4.2 codes this 4. The difference is real — a gh that could not run is
// a tool failure the user fixes on their machine, and a rejected review is a
// disagreement between cr's §8.4.1 pre-validation and GitHub about the diff —
// so an error that was both would have to pick a code, and picking 3 would
// report a state conflict as a broken installation.
type RejectedError struct {
	// Message is GitHub's own top-level message for the refusal.
	Message string
	// Status is the HTTP status GitHub answered with, when the response
	// carried one.
	Status string
	// Positions are the positions GitHub named invalid, each with the
	// record it belongs to, in payload order within each thing GitHub
	// said.
	Positions []InvalidPosition
}

func (e *RejectedError) Error() string {
	said := make([]string, 0, len(e.Positions)+1)
	opening := "§8.4.2: GitHub rejected the review, so nothing was posted: " + e.Message
	if e.Status != "" {
		opening += " (HTTP " + e.Status + ")"
	}
	said = append(said, opening)
	for _, position := range e.Positions {
		said = append(said, "  "+position.String())
	}
	return strings.Join(said, "\n")
}

// response is GitHub's error document, in the fields it names them by.
//
// `errors` is decoded one entry at a time because the API writes it both ways:
// an array of objects for a validation failure, and an array of plain strings
// elsewhere. Reading it as one shape would leave the other silently empty, and
// an empty list here is a refusal that names no position at all.
type response struct {
	Message string            `json:"message"`
	Status  string            `json:"status"`
	Errors  []json.RawMessage `json:"errors"`
}

// reported is one entry of that array, as an object.
type reported struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// said renders one entry as the field GitHub named and what it said about it.
func (r *reported) said() (field, message string) {
	message = r.Message
	if message == "" {
		message = r.Code
	}
	return r.Field, message
}

// Rejection reads GitHub's response to a review-creation call it refused and
// answers §8.4.2's report over the payload that was sent.
//
// status is the HTTP status the transport reported, and "" when it reported
// none; the document's own `status` field stands in only then. §8.4.2's
// rejection is a response stating the review was not created, and only a
// client error states that: GitHub refused the request it read. A server error
// or a timeout — GitHub's 504 carries "We couldn't respond to your request in
// time" as its message — says nothing of whether the review exists, and neither
// does a response naming no status. Each of those answers nil.
//
// It answers nil as well for a response that is not GitHub saying no — bytes
// that do not decode, or a document carrying no message. That is §8.4.4's
// unknown outcome and not this section's: cr could not parse the response, so
// it does not know whether the review was created, and reporting "rejected"
// would be cr asserting the one thing it failed to establish.
//
// Every position GitHub named is attributed to the records it could be about,
// and never to one it could not. GitHub's validation errors name a field and
// not a comment, so an entry naming no path belongs to every comment of the
// payload and each is listed with its record id; an entry whose message names
// a path belongs to the comments on that path alone. Narrowing further would
// be cr inventing an attribution the API did not make.
func Rejection(review *Review, body, status string) *RejectedError {
	var said response
	if err := json.Unmarshal([]byte(body), &said); err != nil || said.Message == "" {
		return nil
	}
	if status == "" {
		status = said.Status
	}
	if !refused(status) {
		return nil
	}
	rejected := &RejectedError{
		Message:   said.Message,
		Status:    status,
		Positions: make([]InvalidPosition, 0, len(said.Errors)),
	}
	for _, raw := range said.Errors {
		field, message := entry(raw)
		rejected.Positions = append(rejected.Positions, attribute(review, field, message)...)
	}
	if len(said.Errors) == 0 {
		// A refusal carrying no error array still refused every
		// comment, since §8.4.1 makes the call atomic.
		rejected.Positions = append(rejected.Positions, attribute(review, "", said.Message)...)
	}
	return rejected
}

// refused reports whether an HTTP status states that the request was refused
// rather than left unanswered: a client error, less 408, which is a timeout
// and §8.4.4's by name. A status that is not a number reads as 0, which is
// outside the range, so it refuses nothing.
func refused(status string) bool {
	code, _ := strconv.Atoi(status)
	return code >= 400 && code < 500 && code != 408
}

// entry reads one element of the errors array, whichever of its two shapes it
// arrived in.
func entry(raw json.RawMessage) (field, message string) {
	var object reported
	if err := json.Unmarshal(raw, &object); err == nil {
		return object.said()
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return "", text
	}
	return "", strings.TrimSpace(string(raw))
}

// attribute names the comments one thing GitHub said is about, each as a
// position carrying its record id.
func attribute(review *Review, field, message string) []InvalidPosition {
	named := make([]InvalidPosition, 0, len(review.Comments))
	for i := range review.Comments {
		comment := &review.Comments[i]
		if onPath(review, message) && !strings.Contains(message, comment.Path) {
			continue
		}
		named = append(named, InvalidPosition{
			Record: comment.Record, Path: comment.Path, StartLine: comment.StartLine, Line: comment.Line,
			Field: field, Message: message,
		})
	}
	return named
}

// onPath reports whether GitHub's words name a path the payload carries, which
// is the only way a response identifies one comment out of the round's.
func onPath(review *Review, message string) bool {
	for i := range review.Comments {
		if review.Comments[i].Path != "" && strings.Contains(message, review.Comments[i].Path) {
			return true
		}
	}
	return false
}
