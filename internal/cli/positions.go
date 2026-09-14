package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/suggestion"
)

// PositionError refuses a comment whose anchor names lines the round's diff
// does not carry on the anchor's side, which is a position GitHub's
// review-creation call would refuse.
//
// It names the record, as §8.2.4 has a suggestion's refusal do, because the
// reviewer's fix is the same kind of fix: move one marker in draft.md back into
// the diff, or delete one block. The exit table codes it 1 beside
// suggestion.RangeError.
type PositionError struct {
	// Record is the id of the record whose comment is refused.
	Record string
	// Anchor is the position the comment would have been sent to.
	Anchor finding.Anchor
}

func (e *PositionError) Error() string {
	return fmt.Sprintf(
		"the comment on record %s is anchored at %s %s:%d-%d, which no one hunk of this round's diff "+
			"carries on that side; §8.4.1 validates every comment position before the review-creation "+
			"call, because that call refuses the whole review over one position",
		e.Record, e.Anchor.Side, e.Anchor.Path, e.Anchor.StartLine, e.Anchor.Line)
}

// positionHunks is how validatePositions reads the round's diff: roundHunks,
// held in a variable for the reason repoDir is one. Most of `cr post`'s
// fixtures stand a round on a head that is no commit, and one of them can stand
// in the hunks its records are anchored in without a checkout behind it; the
// tests about the position check itself leave it as it is and read a real diff.
var positionHunks = roundHunks

// validatePositions is §8.4.1's pre-validation: every comment position held to
// §8.2 before the review-creation call.
//
// It exists because that call is atomic. §8.4.1 says so in as many words — the
// review is created with all its comments or nothing is created — so a single
// position GitHub refuses loses the whole round, and the round is what carries
// every other comment the reviewer approved. Asking first turns that into one
// refusal naming one record, which §8.2.4 codes 1.
//
// Every queued record is asked, not only one carrying a suggestion. §7.2 lets
// the reviewer move a marker's location, and that move is re-validated against
// the tree rather than against the diff, so a plain comment can reach this point
// anchored on a line GitHub was never shown. A record carrying a suggestion is
// held to §8.2.1 and §8.2.2 first, so its refusal still names the suggestion.
//
// The suggestion held to them is the one the comment will carry, as
// draft.SentSuggestion reads it out of the body that will be posted, and never
// the stored field alone: preserved is §7.1.6's kept bodies, the map the
// payload is built from. A fence the reviewer wrote into draft.md reaches the
// author, so it is range-checked — on a LEFT anchor the diff carries too, which
// the position check below would let through — and a stored suggestion they
// deleted from the body reaches nobody, so it blocks nothing.
//
// The set asked about is the queued one, after the draft's discards and after
// §6.3 and §4.1.4 have run. Those are what decide which records become
// comments, and §8.4.1 is about the comments the call would carry rather than
// about the records the round recorded. A round queuing none reads no diff.
func validatePositions(
	owner, repo string, pr int, round *state.Meta, queued []*finding.Finding, preserved map[string]string,
) error {
	if len(queued) == 0 {
		return nil
	}
	hunks, err := positionHunks(owner, repo, pr, round.Head)
	if err != nil {
		return err
	}
	for _, record := range queued {
		sent := *record
		sent.Suggestion = draft.SentSuggestion(record, preserved)
		if err := suggestion.Validate(&sent, hunks); err != nil {
			return err
		}
		if !positionInDiff(&record.Anchor, hunks) {
			return &PositionError{Record: record.ID, Anchor: record.Anchor}
		}
	}
	return nil
}

// positionInDiff reports whether one hunk of the diff carries the anchor whole
// on its side: suggestion.Placeable's reading for a RIGHT anchor, and for a LEFT
// one the merge-base lines removedHeadRange finds a hunk for, which is how
// `cr record` carries a LEFT anchor into its unit.
func positionInDiff(anchor *finding.Anchor, hunks []git.Hunk) bool {
	if anchor.Side == git.Left {
		_, _, found := removedHeadRange(anchor, hunks)
		return found
	}
	return suggestion.Placeable(anchor, hunks)
}
