package draft

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// Rendered is §7.1.5's rendered.json for the queued records: each record's
// agent region exactly as cr renders it, keyed by record id, and nothing else.
//
// It is one entry per record because §7.1.6 compares one record's body at a
// time. No aggregate of the entries is kept: nothing reads one, §8.1.2 has the
// agent rewrite every body so a hash over them all would differ on every round,
// and the one reading it invites — the draft changed, so a human was involved —
// is the inference §8.5.4 forbids.
//
// The entry is always the body cr generates from the record, never what the
// draft currently holds. §7.1.5 has a body preserved per §7.1.6 leave its entry
// alone, and this is where that holds by construction: the function is handed
// records and no draft, so no edit can reach it. Were the entry replaced, the
// next regeneration would find the preserved body equal to it, conclude it was
// untouched, and put cr's English back over the reviewer's prose.
//
// The region is recovered from the rendered comment through render.AgentRegion
// rather than taken from the body directly, so the entry is what reading the
// untouched block back out of draft.md recovers. The cr-owned regions are
// excluded that way, since cr regenerates them, and §7.1.6's byte-exact
// comparison compares like with like: a body the reviewer did not touch is
// equal to its entry by construction, whatever newlines its fields end on.
func Rendered(queued []*finding.Finding, lang render.Lang, sources *Provenances) (map[string]string, error) {
	regions := make(map[string]string, len(queued))
	for _, record := range queued {
		comment, err := commentOf(record, lang, sources, nil)
		if err != nil {
			return nil, err
		}
		regions[record.ID] = render.AgentRegion(comment.String())
	}
	return regions, nil
}
