package intent

import (
	"strconv"
	"strings"
)

// claimIDInfix separates a claim id's issue key from its number, per §3.3's
// `<ISSUE-KEY>#c<n>`.
const claimIDInfix = "#c"

// SplitClaimID reads the issue key out of a claim id.
//
// §3.3 forms every id as `<ISSUE-KEY>#c<n>`, so an id already names the issue
// it belongs to, exactly as note.SplitID reads §3.6.1's `<ISSUE-KEY>#n<n>`.
// Reading it back is how a claim's id and the key §3.2 resolved are held to
// each other: this half is the spelling, which is all an id can be checked for
// on its own, and DecodeClaims compares the key that comes out against the
// run's own — the same division finding.Decode makes when it takes the current
// round's unit ids rather than deciding for itself what a unit is.
//
// The split is at the *last* infix, because §2.2 only forbids an issue key to
// hold a path separator — one may contain `#c` — while the number after the
// last one is fixed. What follows it is then checked by canonicalClaimNumber,
// so only the canonical spelling splits at all.
func SplitClaimID(id string) (issueKey string, ok bool) {
	// Both faults at once: -1 is an id with no infix, and 0 is one whose
	// key is empty, which §3.2 resolves for no issue.
	at := strings.LastIndex(id, claimIDInfix)
	if at < 1 {
		return "", false
	}
	if !canonicalClaimNumber(id[at+len(claimIDInfix):]) {
		return "", false
	}
	return id[:at], true
}

// canonicalClaimNumber reports whether number is the `<n>` §3.3 writes after
// the infix, in the one spelling it writes.
//
// CR-1#c7 is an id; CR-1#c+7, CR-1#c07 and CR-1#c-7 are not, and neither is
// CR-1#c0, because claims are numbered from one. An id spelled some other way
// names nothing §3.3 can form — a claim id is cited by §6.1's `claim` row and
// by §4.1.6's mapping, so a spelling nothing else can reproduce is a citation
// that resolves nowhere.
//
// It is given the number alone rather than the whole id and the key, because
// the caller has already split the two and re-deriving the boundary here would
// be a second reading of the same id that could disagree with the first.
func canonicalClaimNumber(number string) bool {
	n, err := strconv.Atoi(number)
	return err == nil && number == strconv.Itoa(n) && n >= 1
}
