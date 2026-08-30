package intent

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/deligoez/cr/internal/state"
)

// RejectedClaimError reports a claim §3.3 refuses.
//
// §3.3.1 gives its rejection one shape — exit code 1 — and §6.1.3 gives the
// same rejection for a record the shape the message takes here: name the line
// and name the field, so the user can open the one and drop or fill the other.
// The cli layer maps it onto exit code 1: the file was found, read, and parsed,
// and what is wrong is the data inside it.
type RejectedClaimError struct {
	// File is the NDJSON file the agent handed the command.
	File string
	// Line is the one-based line the claim sits on, counting blank lines.
	Line int
	// Field is the field at fault, named by its JSON key so the user can
	// find it in the line the error points at.
	Field string
	// Problem is what is wrong with that field.
	Problem string
}

func (e *RejectedClaimError) Error() string {
	return fmt.Sprintf("%s line %d: %s %s", e.File, e.Line, e.Field, e.Problem)
}

// DecodeClaims reads the claims an agent hands `cr claims record`, holding
// every line to §3.3's table.
//
// issueKey is §3.2's resolution for this run. §3.3 forms every claim id as
// `<ISSUE-KEY>#c<n>`, so an id is a foreign key into that resolution, and a
// claim whose id names some other issue was extracted from some other issue's
// text: §3.3.1 would then validate its span against a text it never came from.
// The check is here rather than in the struct for the reason finding.Decode
// takes the current round's unit ids — which issue this run resolved is a fact
// about the run, and no claim can carry it in a way that proves anything.
//
// spans carries the two texts of §3.3.1 and §3.3.2 in through the same door,
// so a span is validated by the function that decodes it rather than by a
// second pass. That is not tidiness: DecodeStamped is the one place that counts
// lines, blank ones included, and §3.3.1 rejects a claim by naming its line. A
// later pass over the decoded claims would have their ordinal and not their
// line, and would point the user at the wrong one of them.
//
// It also means no caller can obtain claims without their spans having been
// checked. §3.3.1 and §3.3.2 are the whole of what separates a claim drawn from
// the issue from a sentence an agent wrote, and a decode that returned claims
// and left the check to whoever remembered would make that separation optional.
//
// §3.3.3's drift comparison is still not here, and cannot be: it reads claims
// that were recorded in an earlier round against an issue text that has since
// moved, so there is no file being decoded when it runs.
func DecodeClaims(file string, body []byte, issueKey string, spans SpanTexts) ([]*Claim, error) {
	against := claimChecker{file: file, issueKey: issueKey, spans: spans}
	return state.DecodeStamped[Claim](file, body, against.check)
}

// claimChecker holds what one file's claims are checked against: the file they
// arrived in, the issue key §3.2 resolved for the run, and the texts §3.3
// validates a span against.
type claimChecker struct {
	file     string
	issueKey string
	spans    SpanTexts
}

// check holds one line to §3.3's table.
//
// Presence is read from the wire rather than from the decoded claim, for the
// reason state.DecodeStamped already gives about head and round: a struct that
// decoded cannot say which keys were there, and `""` is a value the agent chose
// exactly as much as a sentence is. The remaining checks read the decoded
// claim, because by then the field is known to be present.
//
// The computed fence is walked first, so a line that both oversteps and omits
// is reported by the overstep, which is the fault that says the file was
// produced against the wrong contract. Required fields are then walked in
// §3.3's table order, so a claim missing several is always reported by the
// same one.
//
// The span check is last, and has to be: §3.3 routes it on `source` and, for a
// note-sourced claim, on `note_id`, so it is the one check that reads fields
// two earlier ones are still deciding whether the claim may have. A claim with
// no source has no rule to be checked under, and one whose `note_id` is
// missing or does not belong names no note to be checked against.
func (c claimChecker) check(line int, supplied map[string]json.RawMessage, claim *Claim) error {
	if err := c.computed(line, supplied); err != nil {
		return err
	}
	for _, field := range claimFields {
		if field.Requirement == Required && !written(supplied[field.Name]) {
			return &RejectedClaimError{
				File: c.file, Line: line, Field: field.Name,
				Problem: "is required by §3.3 and this claim does not supply it",
			}
		}
	}
	if key, ok := SplitClaimID(claim.ID); !ok || key != c.issueKey {
		return &RejectedClaimError{
			File: c.file, Line: line, Field: "id",
			Problem: fmt.Sprintf(
				"%q is not a claim of %s; §3.3 forms every claim id as <ISSUE-KEY>#c<n>",
				claim.ID, c.issueKey,
			),
		}
	}
	if err := c.noteID(line, supplied, claim); err != nil {
		return err
	}
	return c.span(line, claim)
}

// computed holds one line to §3.3's two computed rows: cr writes `span_hash`
// and `issue_hash`, and §3.3.1 says so outright while §6.1.4 names the same two
// among the fields no agent may supply. It reports through the same
// state.ReservedFieldError that already carries §2.3.3's head and round half,
// so the whole fence has one answer and one exit code.
//
// Presence alone is the test, as it is for head and round. `"span_hash": null`
// is a key the agent wrote, and the field has the same author whatever value
// sits under it; a validator reading the value would be deciding what to make
// of the agent's answer to a question the agent may not answer. The stakes are
// §3.3.3's: drift is detected by comparing a stored `issue_hash` against a
// fresh one, so an agent that could write it could make a changed issue report
// no drift at all.
func (c claimChecker) computed(line int, supplied map[string]json.RawMessage) error {
	for _, field := range claimFields {
		if field.Requirement != Computed {
			continue
		}
		if _, held := supplied[field.Name]; held {
			return &state.ReservedFieldError{File: c.file, Line: line, Field: field.Name}
		}
	}
	return nil
}

// noteID holds one claim to §3.3's conditional row: `note_id` is required when
// `source` is `note`, and belongs on nothing else.
//
// The Required column cannot express that on its own. It answers the row "no",
// which is right for three sources out of four and wrong for the fourth, so the
// condition is checked here against the decoded source instead of being written
// into the table as an answer that is true only sometimes.
//
// Both directions are checked, because both are silent. A note-sourced claim
// with no `note_id` names no note: §3.3.2 has nothing to validate its span
// against, so the claim is checked against nothing at all, and §8.1.6's
// disclosure has no note to name — the claim would enter coverage looking
// exactly like a tracker claim while resting on unverified hearsay, which is
// the one distinction §3.3.2 exists to keep. A `note_id` on a claim drawn from
// the issue text is the same fault from the other side: §3.3.1 validates that
// claim against the issue text and never against the note, so the field would
// name a provenance nothing checked, and §8.1.6 would disclose it to the reader
// on the strength of the agent's word alone.
func (c claimChecker) noteID(line int, supplied map[string]json.RawMessage, claim *Claim) error {
	held := written(supplied["note_id"])
	switch {
	case claim.Source == ClaimFromNote && !held:
		return &RejectedClaimError{
			File: c.file, Line: line, Field: "note_id",
			Problem: fmt.Sprintf("is required by §3.3 when source is %s", ClaimFromNote),
		}
	case claim.Source != ClaimFromNote && held:
		return &RejectedClaimError{
			File: c.file, Line: line, Field: "note_id",
			Problem: fmt.Sprintf(
				"names a note, and §3.3 draws a claim sourced from %s out of the issue text",
				claim.Source,
			),
		}
	}
	return nil
}

// The two values a JSON object can hold under a key and still supply nothing.
var (
	nullLiteral = []byte("null")
	emptyString = []byte(`""`)
)

// written reports whether a wire line supplied a field.
//
// A key that is not there arrives as a nil value, which is the plain case.
// null and "" are the two ways a line can hold the key and supply nothing under
// it, and §3.3's required rows read both as missing: a claim whose span is the
// empty string was drawn from nothing, and §3.3.1's occurrence check would pass
// it against any issue text there is.
//
// It is also what ClaimSource.UnmarshalJSON asks before refusing a name, so the
// two readings of an empty `source` cannot disagree about which fault it is.
func written(value json.RawMessage) bool {
	value = bytes.TrimSpace(value)
	return len(value) > 0 &&
		!bytes.Equal(value, nullLiteral) &&
		!bytes.Equal(value, emptyString)
}
