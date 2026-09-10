package render

import (
	"strings"

	"github.com/deligoez/cr/internal/finding"
)

// questionMark is the character §8.1.5 requires in every question's body.
const questionMark = "?"

// ValidatePostBody holds the agent region of one comment to everything §8.1
// asks of it before posting: §8.1.3's non-empty body free of the reserved
// sequence, and §8.1.5's question mark in every `kind=question` body.
//
// agent is the agent region alone — what AgentRegion recovered from the draft
// — because §8.1.3 applies §8.1.5 to that region and to no other. A `?` inside
// an owned region is cr's, not the agent's: an evidence region's output tail
// can hold one, and counting it would let a question written as a statement
// through on the strength of a test runner's output.
//
// The kind is the record's register as it will be posted, after §6.3's forcing
// and the reviewer's §7.2 edits. The register of a question is not cosmetic:
// the label tells the reader the comment is a question, and a body that then
// asserts would take that back in the very next line — so a forced question
// whose body was never rewritten into one is refused, naming the record, rather
// than posted as the assertion the forcing was there to prevent.
//
// The draft does not run this. §8.1.2's initial body is the record's English
// summary and evidence, which are statements by construction, and it is the
// agent's rewrite in `draft.md` that turns them into a question; refusing the
// draft for lacking one would refuse the only place it can be written.
func ValidatePostBody(record string, kind finding.Kind, agent string) error {
	if err := ValidateBody(record, agent); err != nil {
		return err
	}
	if kind == finding.KindQuestion && !strings.Contains(agent, questionMark) {
		return &BodyError{Record: record, Problem: "is a kind=question body holding no \"?\" character, " +
			"and §8.1.5 refuses to post a question written as a statement"}
	}
	return nil
}
