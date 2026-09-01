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
	"encoding/json"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/finding"
)

// The delimiters of §7.1.1's marker.
//
// The prefix is `<!-- cr:`, which §8.1.3 reserves for cr-owned regions, and
// sharing it is deliberate rather than incidental. §8.1.3 rejects any body
// carrying a `<!-- cr:` sequence, so no block body can hold a string shaped
// like a marker — which makes "the marker" and "text that looks like one" the
// same set, and leaves the parser of §7.2 one reading rather than two. A prefix
// of its own would be a second reserved sequence with no rule refusing it
// inside a body, and every ambiguity that follows would be ours to invent an
// answer for.
const (
	markerOpen  = "<!-- cr:record"
	markerClose = "-->"
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
	return marker(record) + "\n\n" + body(record) + "\n"
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

// marker is §7.1.1's HTML comment: the eight fields that section names, in the
// order it names them, each written as `name="value"`.
//
// Every field is written even when it is empty, `disposition` being the one
// that always is at draft time. §7.2's table gives `disposition` an edit
// semantics the reviewer reaches by typing `wrong` into it, and a field that
// only appeared once it had a value would leave them nothing to type into.
func marker(record *finding.Finding) string {
	var out strings.Builder
	out.WriteString(markerOpen)
	for _, field := range markerFields(record) {
		out.WriteString(" " + field.name + "=" + quote(field.value))
	}
	out.WriteString(" " + markerClose)
	return out.String()
}

// markerField is one `name="value"` pair of the marker.
type markerField struct {
	name  string
	value string
}

// markerFields is §7.1.1's field list, in the order the section writes it:
// `id`, `kind`, `path`, `start_line`, `line`, `severity`, `grade`, and
// `disposition`.
//
// The three anchor fields are read off the anchor rather than restated, so the
// marker names the location §9.2 recorded and there is no second copy of it to
// drift.
func markerFields(record *finding.Finding) []markerField {
	return []markerField{
		{"id", record.ID},
		{"kind", string(record.Kind)},
		{"path", record.Anchor.Path},
		{"start_line", strconv.Itoa(record.Anchor.StartLine)},
		{"line", strconv.Itoa(record.Anchor.Line)},
		{"severity", string(record.Severity)},
		{"grade", string(record.Grade)},
		{"disposition", string(record.Disposition)},
	}
}

// quote renders one marker value as a JSON string.
//
// JSON rather than a bare word, because a path is the one value cr does not
// choose: it may hold a space, a quote, or a newline, and an unquoted marker
// would then end somewhere other than where it was meant to. Go's encoder also
// escapes `<`, `>` and `&`, which is what keeps `-->` out of every value and
// leaves the marker's own terminator unambiguous.
//
// The error is discarded because there is none to report: encoding/json
// replaces invalid UTF-8 with the replacement rune rather than failing, so a
// string always encodes.
func quote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
