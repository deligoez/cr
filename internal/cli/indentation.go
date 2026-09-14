package cli

import (
	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/suggestion"
)

// indentationWarnings is §8.2.3 over the comments a round would send: one
// warning per suggestion whose first line is indented unlike the line it
// replaces.
//
// The suggestion compared is the one each comment will carry, read out of the
// body that will be posted by draft.SentSuggestion, never the record's stored
// field alone. §7.1.2 lets the reviewer rewrite a block's fence in draft.md and
// that rewrite is what reaches the author, so a warning computed from the stored
// field would stay silent about exactly the suggestion a reviewer re-indented.
// preserved is §7.1.6's kept bodies, the map both `cr draft` and `cr post` build
// the bodies from.
//
// Both commands ask. The draft is where the reviewer decides — they edit the
// block or leave it, and leaving it posts what it says — and `cr post` is where
// an edit made after the last `cr draft` is first read, so the posting run shows
// the warning beside the payload it is about to send and still sends it.
//
// Nothing is read when no comment carries a suggestion. §6.1's table makes the
// field optional and most records have none, so a round without one asks
// GitHub and git for nothing — which is also why a fixture holding no
// suggestion drafts without a repository behind it.
func indentationWarnings(
	owner, repo string, pr int, round *state.Meta, queued []*finding.Finding, preserved map[string]string,
) ([]string, error) {
	sent := make(map[string]string, len(queued))
	for _, record := range queued {
		if replacement := draft.SentSuggestion(record, preserved); replacement != "" {
			sent[record.ID] = replacement
		}
	}
	if len(sent) == 0 {
		return make([]string, 0), nil
	}
	patch, err := roundPatch(owner, repo, pr, round.Head)
	if err != nil {
		return nil, err
	}
	hunks, err := git.ParseHunks(patch)
	if err != nil {
		return nil, err
	}
	// The hunks' texts carry the context lines the hunks do not, and a
	// suggestion may replace one of those: Validate admits the whole head
	// range of a hunk. HunkTexts reads the patch through the one parse
	// ParseHunks just accepted, so it has no error left to return.
	texts, _ := git.HunkTexts(patch)
	warnings := make([]string, 0)
	for _, record := range queued {
		if warning := suggestion.WarnIndentation(record, sent[record.ID], hunks, texts); warning != nil {
			warnings = append(warnings, warning.String())
		}
	}
	return warnings, nil
}
