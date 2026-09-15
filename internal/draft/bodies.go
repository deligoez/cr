package draft

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// longBody is the length, in characters of a block's agent region, above which
// §7.1.4's header names the block.
//
// It is measured rather than chosen. The field trial's pull request
// (spec/field-feedback.md, item 2.8) drafted 42 initial bodies — each a
// record's summary, a blank line and its evidence — running 440 characters at
// the shortest, 858 at the median and 1,646 at the longest. 1,200 sits between
// the median and the longest, so the header names the long tail of that
// distribution and leaves the typical body alone.
const longBody = 1200

// unaskedQuestions are the ids of the queued questions whose agent region
// `cr post` refuses under §8.1.5, in the order the records arrive.
//
// The question is asked of render.ValidatePostBody, the check PostBodies makes,
// so the draft names exactly the bodies the post will refuse rather than a
// second reading of what a question is. That check also carries §8.1.3's body
// refusals, but File and `cr draft` make those first and write nothing past
// one, so the refusal left for this to find over a written draft is §8.1.5's.
//
// It warns and never refuses. §6.3.1's forcing turns statement prose into a
// question by design, and the rewrite into one is the agent's to make in
// draft.md, so a draft holding such a body is an ordinary draft.
func unaskedQuestions(queued []*finding.Finding, preserved map[string]string) []string {
	ids := make([]string, 0)
	for _, record := range queued {
		if render.ValidatePostBody(record.ID, record.Kind, agentRegion(record, preserved)) != nil {
			ids = append(ids, record.ID)
		}
	}
	return ids
}

// QuestionWarnings is one warning per queued question whose body `cr post`
// will refuse under §8.1.5, naming the record, in the order the records
// arrive. preserved is §7.1.6's kept bodies, as File takes them.
func QuestionWarnings(queued []*finding.Finding, preserved map[string]string) []string {
	warnings := make([]string, 0)
	for _, id := range unaskedQuestions(queued, preserved) {
		warnings = append(warnings, "record "+id+": its kind=question body holds no \"?\" character, and "+
			"§8.1.5 has `cr post` refuse it; rewrite the body in draft.md into the question it asks")
	}
	return warnings
}

// longBodies are the ids of the queued records whose agent region runs past
// longBody characters, in the order the records arrive.
func longBodies(queued []*finding.Finding, preserved map[string]string) []string {
	ids := make([]string, 0)
	for _, record := range queued {
		if utf8.RuneCountInString(agentRegion(record, preserved)) > longBody {
			ids = append(ids, record.ID)
		}
	}
	return ids
}

// bodyLines are the header's lines about the bodies the draft holds: the
// questions §8.1.5 will refuse, and the bodies past longBody. A line appears
// only when it has a record to name, so a draft with neither reads as it did
// before either line existed.
func bodyLines(queued []*finding.Finding, preserved map[string]string) []string {
	lines := make([]string, 0, 2)
	if unasked := unaskedQuestions(queued, preserved); len(unasked) > 0 {
		lines = append(lines, fmt.Sprintf(
			"questions without \"?\": %d, which `cr post` refuses (§8.1.5): %s",
			len(unasked), strings.Join(unasked, ", ")))
	}
	if long := longBodies(queued, preserved); len(long) > 0 {
		lines = append(lines, fmt.Sprintf(
			"long bodies: %d over %d characters: %s", len(long), longBody, strings.Join(long, ", ")))
	}
	return lines
}

// waiverLine is the header's statement of when §7.2's discard verbs write
// their waivers. §7.1.6 fixes the moment as the regeneration, before anything
// is posted, and a reviewer who marks a block `wrong` to see how the draft
// reads would otherwise learn of the repository-wide waiver afterwards.
// `cr post --confirm` writes the waivers of discards no regeneration read.
const waiverLine = "waivers: a block marked disposition=\"wrong\" writes a repository-wide waiver, and a " +
	"deleted block a pull-request-scoped one, when the next `cr draft` regenerates this file, " +
	"before anything is posted (§7.1.6); with no `cr draft` in between, `cr post --confirm` writes them"
