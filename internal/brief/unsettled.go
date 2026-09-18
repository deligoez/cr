package brief

import "fmt"

// UnsettledPostError reports a `cr brief` on a moved head over a round that
// still carries §8.4.4's `post_unresolved`.
//
// §9.3.4 would move the round's queued records to `stale`, and §9.1 lists no
// move out of `stale`, so a review the unsettled send did create could never be
// adopted: `cr post --reconcile` would find a new round holding no payload, and
// the flag would stand over every round after it. The round is refused before
// anything is written, and §9.3.2 leaves the way forward open, because it
// exempts `cr post --reconcile` from the moved-head refusal.
//
// internal/cli reports it as its own UnresolvedPostError, which carries §11.2's
// code 4 and the hint naming the reconciliation; this type exists because that
// one lives above this package.
type UnsettledPostError struct {
	// Owner, Repo, and PR name the pull request.
	Owner string
	Repo  string
	PR    int
	// Round is the recorded round whose send was never settled.
	Round int
	// Recorded is the head meta.json holds, and Current the head §3.7.1
	// fetched, which §9.3.1 reports side by side.
	Recorded string
	Current  string
}

func (e *UnsettledPostError) Error() string {
	return fmt.Sprintf(
		"the head moved from %s to %s, and §9.3.4 would move round %d's queued records "+
			"to stale while a send carrying them has an outcome cr never learned",
		e.Recorded, e.Current, e.Round,
	)
}
