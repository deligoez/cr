package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/suggestion"
)

// indentationWarnings is §8.2.3 over the records a draft holds: one warning per
// suggestion whose first line is indented unlike the line it replaces.
//
// The draft is where the warning belongs, because §8.2.3 ends in a decision the
// reviewer makes and `draft.md` is where they make it — they edit the block or
// leave it, and leaving it posts what it says. A warning first shown by the
// posting run would arrive after the moment it is about.
//
// Nothing is read when nothing carries a suggestion. §6.1's table makes the
// field optional and most records have none, so a round without one asks
// GitHub and git for nothing — which is also why a fixture holding no
// suggestion drafts without a repository behind it.
func indentationWarnings(
	owner, repo string, pr int, round *state.Meta, queued []*finding.Finding,
) ([]string, error) {
	if !anySuggestion(queued) {
		return make([]string, 0), nil
	}
	hunks, err := roundHunks(owner, repo, pr, round.Head)
	if err != nil {
		return nil, err
	}
	warnings := make([]string, 0)
	for _, record := range queued {
		if warning := suggestion.WarnIndentation(record, hunks); warning != nil {
			warnings = append(warnings, warning.String())
		}
	}
	return warnings, nil
}

// anySuggestion reports whether any of the records carries replacement lines.
func anySuggestion(records []*finding.Finding) bool {
	for _, record := range records {
		if record.Suggestion != "" {
			return true
		}
	}
	return false
}
