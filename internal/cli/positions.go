package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/suggestion"
)

// validatePositions is §8.4.1's pre-validation: every comment position held to
// §8.2 before the review-creation call.
//
// It exists because that call is atomic. §8.4.1 says so in as many words — the
// review is created with all its comments or nothing is created — so a single
// position GitHub refuses loses the whole round, and the round is what carries
// every other comment the reviewer approved. Asking first turns that into one
// refusal naming one record, which §8.2.4 codes 1.
//
// §8.2 is about where a replacement lands, so a record carrying no suggestion
// passes, and a round carrying none reads no diff at all — the same guard
// indentationWarnings uses, and for the same reason: §6.1's table makes the
// field optional, most records have none, and a fixture holding none needs no
// repository behind it.
//
// The set asked about is the queued one, after the draft's discards and after
// §6.3 and §4.1.4 have run. Those are what decide which records become
// comments, and §8.4.1 is about the comments the call would carry rather than
// about the records the round recorded.
func validatePositions(
	owner, repo string, pr int, round *state.Meta, queued []*finding.Finding,
) error {
	if !anySuggestion(queued) {
		return nil
	}
	hunks, err := roundHunks(owner, repo, pr, round.Head)
	if err != nil {
		return err
	}
	for _, record := range queued {
		if err := suggestion.Validate(record, hunks); err != nil {
			return err
		}
	}
	return nil
}
