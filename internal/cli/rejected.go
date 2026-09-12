package cli

import (
	"errors"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/post"
)

// rejectedPost is §8.4.2 read off a review-creation call that failed: the
// report GitHub's refusal calls for, or the failure itself when GitHub did not
// refuse.
//
// It is the seam `cr post --confirm` turns post.Create's error into a refusal
// through, and it is the whole of the section's ordering. §8.4.1 makes the
// call atomic, so a rejection created nothing: no record walks to `posted`, no
// thread id is adopted, and §7.3.1's outcome events are not written — every one
// of those is on the other branch, the one this run never reaches.
//
// Three failures arrive here and only one is §8.4.2's. A gh that could not run
// is §3.1.3's external command failure and codes 3; a response cr cannot parse
// is §8.4.4's unknown outcome, where cr does not know whether the review was
// created and must not say it was rejected; and a document GitHub said no in is
// this section's, coded 4. The first two are passed through unchanged, so the
// caller sees the error it would have seen without this seam.
func rejectedPost(review *post.Review, err error) error {
	var failed *gh.CommandError
	if !errors.As(err, &failed) {
		return err
	}
	if rejected := post.Rejection(review, failed.Stdout); rejected != nil {
		return rejected
	}
	return err
}
