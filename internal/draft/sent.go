package draft

import (
	"strings"

	"github.com/deligoez/cr/internal/finding"
)

// suggestionFence opens §7.1.3's fenced block, GitHub's one-click replacement,
// and closingFence ends it.
const (
	suggestionFence = "```suggestion"
	closingFence    = "```"
)

// SentSuggestion is the replacement a record's comment will carry: the lines
// of the first fenced `suggestion` block in the body the comment is built
// from, or the empty string when that body holds none.
//
// It answers for the suggestion that will be sent, not for the one the record
// stores. §7.1.2 lets the reviewer rewrite a block's body entirely, fence
// included, and PostBodies carries that rewrite to the author, so §8.2.3's
// comparison asked of the stored field alone would be asked of text the author
// never receives. A block the reviewer left alone renders the stored field
// into its fence, so that is what it answers; a body the reviewer rewrote is
// read for its fence, which may differ from the field, or be absent.
func SentSuggestion(record *finding.Finding, preserved map[string]string) string {
	agent, kept := preserved[record.ID]
	if !kept {
		return record.Suggestion
	}
	return fencedSuggestion(agent)
}

// fencedSuggestion returns the lines between the first suggestion fence of a
// body and the fence closing it, or the end of the body when none closes it.
func fencedSuggestion(body string) string {
	lines := strings.Split(body, "\n")
	for at, line := range lines {
		if strings.TrimSpace(line) != suggestionFence {
			continue
		}
		replacement := make([]string, 0, len(lines)-at-1)
		for _, inner := range lines[at+1:] {
			if strings.TrimSpace(inner) == closingFence {
				break
			}
			replacement = append(replacement, inner)
		}
		return strings.Join(replacement, "\n")
	}
	return ""
}
