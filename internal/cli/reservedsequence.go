package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// ReservedSequenceError refuses a record whose `summary` or `evidence` carries
// §8.1.3's reserved sequence, at the door the record enters cr by.
//
// It is a §6.1.3 rejection — file, line, field — and unwraps to one, so it
// reports as every other rejection `cr record` and `cr merge` make. It carries
// its own row in the exit table for the step: the fault is one the role can fix
// by rewriting its own sentence, and the sequence has to be named for that
// sentence to be findable.
//
// The refusal is here rather than only at draft time because a draft-time
// refusal has no way out. §8.1.2 renders a block's first body out of these two
// fields verbatim, and §8.1.3 refuses a body holding the sequence, so a stored
// record carrying it makes `cr draft` exit 1 — while `cr triage` needs a draft
// that the same refusal prevents, and no command edits a stored record's prose.
// Measured on v0.3.0 (field feedback M-1.1): a convention role quoting
// draft/marker.go put `<!-- cr:` in one record's evidence and that record could
// never be drafted.
type ReservedSequenceError struct {
	*finding.RejectedRecordError
	// Record is the id of the record at fault, which the role finds its
	// own sentence by.
	Record string
}

func (e *ReservedSequenceError) Unwrap() error {
	return e.RejectedRecordError
}

// reservedTextFields are the fields §8.1.2 and §7.1.3 place in a block's body
// verbatim, in §6.1's table order, which is the order a record is walked in so a record
// carrying the sequence in both is always reported by the same field.
var reservedTextFields = []struct {
	name string
	of   func(*finding.Finding) string
}{
	{"summary", func(r *finding.Finding) string { return r.Summary }},
	{"evidence", func(r *finding.Finding) string { return r.Evidence }},
	// §7.1.3 renders the suggestion into the body as a fenced block, so it
	// reaches the draft the same way and is held to the same rule.
	{"suggestion", func(r *finding.Finding) string { return r.Suggestion }},
}

// refuseReservedSequences holds every record of one input file to §8.1.3's
// reserved sequence, and refuses the file naming the record that carries it.
//
// The detection is render.HoldsReserved, the one every refusal of the sequence
// makes, so what is refused here is exactly what `cr draft` would refuse later.
// Both doors ask: `cr record` through decodeInput, and `cr merge` through
// readPerRole, where the line numbers are still the role's own file's.
//
// It runs straight after the decode, before §6.4.4's drops shorten the slice
// the line numbers index — refuseUnknownThreads' position and for its reason.
func refuseReservedSequences(file string, body []byte, records []*finding.Finding) error {
	at := state.RecordLines(body)
	for i, record := range records {
		for _, field := range reservedTextFields {
			if !render.HoldsReserved(field.of(record)) {
				continue
			}
			return &ReservedSequenceError{
				Record: record.ID,
				RejectedRecordError: &finding.RejectedRecordError{
					File: file, Line: at[i], Field: field.name,
					Problem: fmt.Sprintf(
						"of record %s carries %q, which §8.1.3 reserves for the record marker "+
							"and cr's own regions",
						record.ID, render.Reserved),
				},
			}
		}
	}
	return nil
}
