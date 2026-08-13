package finding

import "fmt"

// HonestyDisclosure is a report §11.1 exempts from `--quiet`. Every lens of
// §4.5.4 that did not run, the probe cap of §5.6.4, the forcing counts of
// §6.3.2, the waiver and duplicate counts of §10.1.6, the sandbox recreation
// notice of §5.1.6, the stale-round report of §9.3.2, and the comment cap of
// §1.6.2 are the seven, and they are always printed.
//
// The shared writer that holds the exemption for all seven is not built here.
// This is the shape it consumes, so a disclosure implements the contract before
// there is a writer to hand it to, and adoption is a call site rather than a
// rewrite.
type HonestyDisclosure interface {
	// Disclosure is the text that is printed whatever the flags say.
	Disclosure() string
}

// CommentCap is the decision §1.6.2 makes over one round's queued comments: the
// count, the cap it was measured against, and nothing else.
//
// It is deliberately not a fitted queue. §1.6.2 is one rule written twice — cr
// MUST block posting above the cap, and cr MUST NOT silently drop comments to
// fit — and a function handing back the queue trimmed to length would satisfy
// the first while breaking the second, in code that reads like a courtesy. So
// there is no shape here that can carry a subset of the queue: fitting is not
// something a caller can reach for and find waiting, it has to be written from
// scratch against a type that offers nothing towards it.
// TestNothingCarriesAFittedQueueBackFromTheCap holds that open.
//
// Triaging down to the cap is the user's, through §1.6.3's non-destructive
// discard, and cr's part is to report the number and refuse.
type CommentCap struct {
	// Count is how many comments the round has queued. §1.6.1 leaves v0.1
	// one comment channel and every queued record is rendered into it, so
	// one record is one comment.
	Count int
	// Max is the resolved post.max_comments of §2.7, whose built-in default
	// is 20. It is passed in rather than read here, because §2.7 resolves
	// it across five layers and a second path to the value is a second
	// answer.
	Max int
}

// CommentCapFor measures a round's queued records against post.max_comments.
//
// §7.1.4's draft header and `cr post` read their number from this one call, so
// the count the user triages against and the count that blocks the post are one
// number by construction rather than two that happen to agree.
func CommentCapFor(queued []*Finding, maxComments int) CommentCap {
	return CommentCap{Count: len(queued), Max: maxComments}
}

// Disclosure is the §11.1 honesty disclosure of the comment cap, printed
// whether or not the cap was exceeded and whatever `--quiet` says.
//
// A suppressed cap report is the silent drop §1.6.2 forbids wearing a different
// hat: the comments the round could not fit would go unmentioned, which is the
// outcome the clause exists to prevent, reached by way of an output flag rather
// than a truncation.
func (c CommentCap) Disclosure() string {
	if c.exceeded() {
		return fmt.Sprintf("%d comments queued against post.max_comments %d, %d over the cap",
			c.Count, c.Max, c.Count-c.Max)
	}
	return fmt.Sprintf("%d comments queued against post.max_comments %d", c.Count, c.Max)
}

// Err is the block of §1.6.2: an error naming the count above the cap, and nil
// at or below it. It is the only thing exceeding the cap yields, so a caller
// that ignores it posts nothing it would not have posted anyway.
func (c CommentCap) Err() error {
	if c.exceeded() {
		return &CommentCapExceededError{Cap: c}
	}
	return nil
}

// exceeded is the cap comparison, in one place because Disclosure and Err must
// never disagree about it. The cap is the last count that fits: post.max_comments
// 20 caps the round at 20 comments and blocks the 21st.
func (c CommentCap) exceeded() bool {
	return c.Count > c.Max
}

// CommentCapExceededError reports a round whose queued comments exceed
// post.max_comments, per §1.6.2.
//
// The cli layer maps it onto exit code 1. Every record in the round may be
// well-formed; what fails validation is the round's payload as a whole, and
// §11.2 runs that validation before the confirmation gate, so the block lands
// whether or not `--confirm` was given.
type CommentCapExceededError struct {
	// Cap is the decision that blocked the post, so the count reaches a
	// caller as a number and not only as text inside a message.
	Cap CommentCap
}

func (e *CommentCapExceededError) Error() string {
	return fmt.Sprintf("%s: triage the draft down to %d, or raise post.max_comments; cr will not drop %d to fit",
		e.Cap.Disclosure(), e.Cap.Max, e.Cap.Count-e.Cap.Max)
}
