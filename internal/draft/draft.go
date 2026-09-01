// Package draft renders spec/0.1.0.md §7.1's editable draft.
//
// §7.1 has `cr draft <pr>` render every queued record into a single Markdown
// file at `rounds/<n>/draft.md`. That one file is the whole of P4's surface: cr
// drafts, the human edits, and §8.1.2 makes editing it the only input path for
// reader-facing prose. So what is written here is what the author eventually
// reads, and what §7.2 reads back as triage.
//
// Only queued records are rendered, and the state is asked of the record rather
// than of the caller. §9.1's table gives `cr draft` exactly one row into
// `queued`, from `draft`, so a record in any other state has already been
// decided about: a suppressed duplicate speaks through its representative
// (§6.4.3), a thread-suppressed record through the thread that already covers
// it (§3.5.4), a discard through the waiver it wrote (§7.2), and a stale record
// belongs to a head that has moved on (§9.3.4). Rendering any of them would put
// a decided record back in front of the reviewer as if it were open.
//
// Nothing here reads §6.1's table, computes a grade, or forms an opinion. It is
// handed the records the command settled and turns them into text.
package draft

import (
	"strings"

	"github.com/deligoez/cr/internal/finding"
)

// Render is §7.1's draft: every queued record as one block, in the order the
// records arrive, and nothing else.
//
// The order is the caller's. §7.1 asks for every queued record and says nothing
// about arrangement, and findings.ndjson is in the order `cr record` stored it
// — which is `cr merge`'s output order, which is the role files' own. A sort
// invented here would be a reading order cr chose for the reviewer with nothing
// in the spec behind it.
//
// A round with nothing queued renders the empty string. That is a real answer
// rather than a missing one: the file is written either way, so a reviewer who
// opens it sees a draft with no blocks rather than a stale draft from the round
// before.
func Render(queued []*finding.Finding) string {
	blocks := make([]string, 0, len(queued))
	for _, record := range queued {
		blocks = append(blocks, block(record))
	}
	return strings.Join(blocks, "\n")
}

// block is one record's rendering: §7.1.1's marker introducing it, and §7.1.2's
// body beneath, separated by a blank line so the marker reads as an
// introduction rather than as part of the prose.
func block(record *finding.Finding) string {
	return markerOf(record).String() + "\n\n" + body(record) + "\n"
}

// body is the free-form Markdown region of §7.1.2, which the user may rewrite
// entirely.
//
// It is free-form in the sense that matters: nothing downstream parses it, and
// §7.2's triage reads the marker rather than the prose, so a reviewer may
// replace every word of it without changing what the block means to cr.
//
// What it opens with is `summary`, which §6.1.1 keeps in English. §8.1.2's
// composition — the initial body drawn from `summary` and `evidence`, and the
// agent's rewrite into `render.lang` — is its own obligation and its own task;
// what §7.1 requires here is that the region exist, be the user's, and be
// non-empty, since §8.1.3 refuses to post an empty body.
func body(record *finding.Finding) string {
	return record.Summary
}
