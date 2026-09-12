package draft

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// PostBodies is the comment §8.1.3 posts for each of a round's records, keyed
// by record id: the agent region the draft carries, and the cr-owned regions
// regenerated from the record.
//
// It exists to be the same path a block is drafted through, and that is the
// whole of its justification. §8.1.3 has cr regenerate every owned region and
// discard edits inside them, at post time as much as at draft time, so the
// label, the provenance block and the evidence block are built here by
// commentOf — once per record, the one function `cr draft` renders a block
// with. A renderer written for the posting half would be a second answer to
// what those regions say: the two would agree on the day they were written and
// drift on the next edit to either, and the reader would have no way to tell
// which half they were looking at. Round 12's evidence-region finding is that
// risk, named before the second renderer existed.
//
// preserved is §7.1.6's kept bodies, which at post time is the reviewer's draft
// itself: §8.1.2 makes editing draft.md the one input path for reader-facing
// prose, so the agent region that reaches the author is the one read back out
// of the file. A record whose block was untouched has no entry and renders the
// body cr renders in the draft, byte for byte.
//
// §8.1.5 is applied here and not in the draft, for the reason ValidatePostBody
// gives: the initial body is the record's English summary and evidence, which
// are statements by construction, and the rewrite into a question is the
// agent's to make in draft.md. This is the last point at which it can be asked
// for and the first at which it can honestly be required.
func PostBodies(
	records []*finding.Finding, lang render.Lang, sources *Provenances, preserved map[string]string,
) (map[string]string, error) {
	bodies := make(map[string]string, len(records))
	for _, record := range records {
		comment, err := commentOf(record, lang, sources, preserved)
		if err != nil {
			return nil, err
		}
		if err := render.ValidatePostBody(record.ID, record.Kind, comment.Body); err != nil {
			return nil, err
		}
		bodies[record.ID] = comment.String()
	}
	return bodies, nil
}
