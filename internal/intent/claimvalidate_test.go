package intent

import (
	"testing"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// claimsFile is the file an agent hands `cr claims record`, named the way §2.3
// names it so a rejection points at the file the user has to open.
const claimsFile = "claims.ndjson"

// wellFormedClaim is one line §3.3 accepts: every required row supplied, no
// computed row, no note.
const wellFormedClaim = `{"id":"CR-1#c1","text":"An expired token is rejected.",` +
	`"source":"acceptance","span":"expired tokens are rejected"}`

// claimSpans are the texts §3.3 validates this file's spans against.
//
// The issue text holds every tracker span the cases use, `s` among them — the
// one-character stand-in the field-level cases carry where the span itself is
// not what is under test — and the store holds the one note they name. Each
// case is about some other row of §3.3's table, so the span has to pass for the
// row under test to be the thing that refuses.
var claimSpans = SpanTexts{
	Issue: "expired tokens are rejected\ns\n",
	Notes: []note.Note{{ID: "CR-1#n2", Text: "s"}},
}

// §3.3's table marks four rows required, and a claim missing any of them is
// rejected naming the line and the field. The rejection has to read the wire
// rather than the decoded claim: `""` is a value the agent chose exactly as
// much as a sentence is, and a claim whose span is the empty string was drawn
// from nothing — §3.3.1's occurrence check would pass it against any issue text
// there is.
//
// The walk is in §3.3's table order, so a claim missing several rows is always
// reported by the same one and a user fixing them meets them in the order the
// spec writes them.
func TestAClaimMustSupplyEveryRequiredRow(t *testing.T) {
	claims, err := DecodeClaims(claimsFile, []byte(wellFormedClaim+"\n"), "CR-1", claimSpans)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	assert.Equal(t, &Claim{
		ID:     "CR-1#c1",
		Text:   "An expired token is rejected.",
		Source: ClaimFromAcceptance,
		Span:   "expired tokens are rejected",
	}, claims[0], "a well-formed claim decodes with no hash and no stamp of its own")

	for name, tc := range map[string]struct{ line, field string }{
		"no id": {
			line:  `{"text":"t","source":"acceptance","span":"s"}`,
			field: "id",
		},
		"a null id": {
			line:  `{"id":null,"text":"t","source":"acceptance","span":"s"}`,
			field: "id",
		},
		"an empty id": {
			line:  `{"id":"","text":"t","source":"acceptance","span":"s"}`,
			field: "id",
		},
		"no text": {
			line:  `{"id":"CR-1#c1","source":"acceptance","span":"s"}`,
			field: "text",
		},
		"no source": {
			line:  `{"id":"CR-1#c1","text":"t","span":"s"}`,
			field: "source",
		},
		"a null source": {
			line:  `{"id":"CR-1#c1","text":"t","source":null,"span":"s"}`,
			field: "source",
		},
		"an empty source": {
			line:  `{"id":"CR-1#c1","text":"t","source":"","span":"s"}`,
			field: "source",
		},
		"no span": {
			line:  `{"id":"CR-1#c1","text":"t","source":"acceptance"}`,
			field: "span",
		},
		"an empty span": {
			line:  `{"id":"CR-1#c1","text":"t","source":"acceptance","span":""}`,
			field: "span",
		},
		"neither text nor span, named by the first": {
			line:  `{"id":"CR-1#c1","source":"acceptance"}`,
			field: "text",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeClaims(
				claimsFile, []byte(wellFormedClaim+"\n\n"+tc.line+"\n"), "CR-1", claimSpans,
			)
			var rejected *RejectedClaimError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, claimsFile, rejected.File)
			assert.Equal(t, 3, rejected.Line, "blank lines are counted, so the number opens the line")
			assert.Equal(t, tc.field, rejected.Field)
			assert.Equal(t,
				"claims.ndjson line 3: "+tc.field+
					" is required by §3.3 and this claim does not supply it",
				err.Error())
		})
	}
}

// §3.3 marks `span_hash` and `issue_hash` computed and §3.3.1 has cr compute
// both itself, so a claim arriving with either is refused. §6.1.4 names the
// same two among the fields no agent may supply, and the refusal is reported
// through the type that already carries §2.3.3's head and round half, so the
// whole fence has one answer and one exit code.
//
// Presence alone is the test. `"issue_hash": null` is a key the agent wrote,
// and the stakes are §3.3.3's: drift is detected by comparing the stored
// `issue_hash` against a fresh one, so an agent that could write that row could
// make a changed issue report no drift at all.
//
// The fence is walked before the required rows, so a line that both oversteps
// and omits is reported by the overstep — the fault that says the file was
// produced against the wrong contract, which no amount of filling rows in will
// fix.
func TestAnAgentMayNotSupplyTheClaimHashes(t *testing.T) {
	for name, tc := range map[string]struct{ line, field string }{
		"a span hash of its own": {
			line: `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s",` +
				`"span_hash":"0123456789abcdef"}`,
			field: "span_hash",
		},
		"an issue hash of its own": {
			line: `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s",` +
				`"issue_hash":"0123456789abcdef"}`,
			field: "issue_hash",
		},
		"a null span hash": {
			line:  `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","span_hash":null}`,
			field: "span_hash",
		},
		"an empty issue hash": {
			line:  `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","issue_hash":""}`,
			field: "issue_hash",
		},
		"both, named by the first": {
			line: `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s",` +
				`"issue_hash":"a","span_hash":"b"}`,
			field: "span_hash",
		},
		"an overstep on a line that also omits": {
			line:  `{"span_hash":"0123456789abcdef"}`,
			field: "span_hash",
		},
		"a head of its own": {
			line:  `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","head":"0f1e2d3"}`,
			field: "head",
		},
		"a round of its own": {
			line:  `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","round":2}`,
			field: "round",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeClaims(
				claimsFile, []byte(wellFormedClaim+"\n\n"+tc.line+"\n"), "CR-1", claimSpans,
			)
			var reserved *state.ReservedFieldError
			require.ErrorAs(t, err, &reserved)
			assert.Equal(t, claimsFile, reserved.File)
			assert.Equal(t, 3, reserved.Line)
			assert.Equal(t, tc.field, reserved.Field)
			assert.Equal(t,
				"claims.ndjson line 3: "+tc.field+" is written by cr and must not be supplied",
				err.Error())
		})
	}
}

// §3.3's fence under any letter case. encoding/json binds `"Span_Hash"` to the
// span hash exactly as it binds `"span_hash"`, and `"Note_ID"` to the note id,
// so a fence that looked keys up by their exact spelling would take a hash, or
// a provenance nothing checked, on the agent's word. Each refusal names the
// field by §3.3's own spelling, and a note-sourced claim naming its note under
// another spelling still names it.
func TestTheClaimFenceHoldsUnderAnyKeyCase(t *testing.T) {
	for name, tc := range map[string]struct{ line, field string }{
		"a span hash, capitalised": {
			line:  `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","Span_Hash":"0123"}`,
			field: "span_hash",
		},
		"an issue hash, upper-cased": {
			line:  `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","ISSUE_HASH":"0123"}`,
			field: "issue_hash",
		},
		"a head, capitalised": {
			line:  `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","Head":"0f1e2d3"}`,
			field: "head",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeClaims(claimsFile, []byte(tc.line+"\n"), "CR-1", claimSpans)
			var reserved *state.ReservedFieldError
			require.ErrorAs(t, err, &reserved)
			assert.Equal(t, &state.ReservedFieldError{File: claimsFile, Line: 1, Field: tc.field}, reserved)
		})
	}

	_, err := DecodeClaims(claimsFile,
		[]byte(`{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","Note_ID":"CR-1#n2"}`+"\n"),
		"CR-1", claimSpans)
	var rejected *RejectedClaimError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, "note_id", rejected.Field, "a claim drawn from the issue text names no note under any spelling")

	accepted, err := DecodeClaims(claimsFile,
		[]byte(`{"id":"CR-1#c1","text":"t","source":"note","span":"s","NOTE_ID":"CR-1#n2"}`+"\n"),
		"CR-1", claimSpans)
	require.NoError(t, err)
	require.Len(t, accepted, 1)
	assert.Equal(t, "CR-1#n2", accepted[0].NoteID)
}

// §3.3 forms every claim id as `<ISSUE-KEY>#c<n>`, so the id is a foreign key
// into the key §3.2 resolved for this run — and the run is the only side that
// knows what that key is. A claim whose id names some other issue was extracted
// from some other issue's text, and §3.3.1 would then check its span against a
// text it never came from and either pass it by luck or report a drift that
// never happened.
//
// The claim is held to the run's key here rather than in the struct for the
// reason finding.Decode takes the current round's unit ids: nothing a claim
// carries can prove which issue this run is about.
func TestAClaimIDMustNameTheIssueTheRunResolved(t *testing.T) {
	accepted, err := DecodeClaims(
		claimsFile,
		[]byte(`{"id":"CR-1#c9","text":"t","source":"acceptance","span":"s"}`+"\n"),
		"CR-1", claimSpans,
	)
	require.NoError(t, err)
	require.Len(t, accepted, 1)
	assert.Equal(t, "CR-1#c9", accepted[0].ID)

	for name, id := range map[string]string{
		"another issue's claim":   "CR-2#c1",
		"a record id":             "f1",
		"a note id":               "CR-1#n1",
		"the issue key alone":     "CR-1",
		"a claim numbered zero":   "CR-1#c0",
		"the right key, misspelt": "CR-1#c01",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeClaims(
				claimsFile,
				[]byte(`{"id":"`+id+`","text":"t","source":"acceptance","span":"s"}`+"\n"),
				"CR-1", claimSpans,
			)
			var rejected *RejectedClaimError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, "id", rejected.Field)
			assert.Equal(t,
				`claims.ndjson line 1: id "`+id+
					`" is not a claim of CR-1; §3.3 forms every claim id as <ISSUE-KEY>#c<n>`,
				err.Error())
		})
	}
}

// §3.3's `note_id` row is conditional — "required when `source` is `note`" —
// and the Required column cannot say so: it answers the row "no", which is
// right for three sources out of four and wrong for the fourth. So the rule is
// checked against the decoded source, in both directions, because both are
// silent faults that reach the reader as provenance.
//
// A note-sourced claim with no note names no note, and §3.3.2 has nothing to
// validate its span against: the claim would enter coverage looking exactly
// like a tracker claim while resting on unverified hearsay, which is the one
// distinction §3.3.2 exists to keep. The converse is the same fault mirrored: a
// claim drawn from the issue text is validated against the issue text and never
// against a note, so a `note_id` on it names a provenance nothing checked and
// §8.1.6 would disclose it anyway.
func TestANoteSourcedClaimNamesItsNoteAndNothingElseDoes(t *testing.T) {
	fromNote := `{"id":"CR-1#c1","text":"t","source":"note","span":"s","note_id":"CR-1#n2"}`
	accepted, err := DecodeClaims(claimsFile, []byte(fromNote+"\n"), "CR-1", claimSpans)
	require.NoError(t, err)
	require.Len(t, accepted, 1)
	assert.Equal(t, ClaimFromNote, accepted[0].Source)
	assert.Equal(t, "CR-1#n2", accepted[0].NoteID)

	for _, quiet := range []string{
		`{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s"}`,
		`{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","note_id":null}`,
		`{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","note_id":""}`,
	} {
		claims, err := DecodeClaims(claimsFile, []byte(quiet+"\n"), "CR-1", claimSpans)
		require.NoError(t, err, quiet)
		assert.Empty(t, claims[0].NoteID, "a claim drawn from the issue text names no note")
	}

	for name, tc := range map[string]struct{ line, problem string }{
		"a note-sourced claim with no note": {
			line:    `{"id":"CR-1#c1","text":"t","source":"note","span":"s"}`,
			problem: "is required by §3.3 when source is note",
		},
		"a note-sourced claim whose note is null": {
			line:    `{"id":"CR-1#c1","text":"t","source":"note","span":"s","note_id":null}`,
			problem: "is required by §3.3 when source is note",
		},
		"a note-sourced claim whose note is empty": {
			line:    `{"id":"CR-1#c1","text":"t","source":"note","span":"s","note_id":""}`,
			problem: "is required by §3.3 when source is note",
		},
		"a description claim that names a note": {
			line:    `{"id":"CR-1#c1","text":"t","source":"description","span":"s","note_id":"CR-1#n2"}`,
			problem: "names a note, and §3.3 draws a claim sourced from description out of the issue text",
		},
		"an acceptance claim that names a note": {
			line:    `{"id":"CR-1#c1","text":"t","source":"acceptance","span":"s","note_id":"CR-1#n2"}`,
			problem: "names a note, and §3.3 draws a claim sourced from acceptance out of the issue text",
		},
		"a comment claim that names a note": {
			line:    `{"id":"CR-1#c1","text":"t","source":"comment","span":"s","note_id":"CR-1#n2"}`,
			problem: "names a note, and §3.3 draws a claim sourced from comment out of the issue text",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeClaims(claimsFile, []byte(tc.line+"\n"), "CR-1", claimSpans)
			var rejected *RejectedClaimError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, "note_id", rejected.Field)
			assert.Equal(t, "claims.ndjson line 1: note_id "+tc.problem, err.Error())
		})
	}
}
