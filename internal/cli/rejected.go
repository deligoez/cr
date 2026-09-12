package cli

import (
	"errors"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/post"
)

// rejectedPost is §8.4.2 read off a review-creation call that failed: the
// report GitHub's refusal calls for, and nil when GitHub did not refuse.
//
// It is the one reading of §8.4.2 in cr, and postOutcome's §8.4 fork asks it
// rather than repeating it — two readings of "did GitHub say no" that could
// drift is a run reporting a rejection GitHub never made, or setting
// `post_unresolved` on a round GitHub plainly refused.
//
// §8.4.1 makes the call atomic, so a rejection created nothing: no record walks
// to `posted`, no thread id is adopted, and §7.3.1's outcome events are not
// written — every one of those is on the caller's other branch.
//
// Three failures arrive here and only one is §8.4.2's. A gh that could not run
// is §3.1.3's external command failure; a response cr cannot parse is §8.4.4's
// unknown outcome, where cr does not know whether the review was created and
// must not say it was rejected; and a document GitHub said no in is this
// section's, coded 4. Only the third is answered here, and the nil the other
// two get is what leaves the caller free to report them as what they are.
// The nil is returned as a nil of this function's own type and never as the
// typed pointer post.Rejection answers with. A `*post.RejectedError` that is
// nil is not a nil `error`: it is an interface carrying a nil pointer, every
// caller's `!= nil` is true of it, and the first one to call Error on it
// panics. That is the whole reason the pointer is compared here rather than
// returned straight through.
func rejectedPost(review *post.Review, err error) error {
	var failed *gh.CommandError
	if !errors.As(err, &failed) {
		return nil
	}
	rejected := post.Rejection(review, failed.Stdout)
	if rejected == nil {
		return nil
	}
	return rejected
}
