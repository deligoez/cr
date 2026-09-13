package intent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/text"
)

// The two texts §3.3 validates a span against, made disjoint on purpose.
//
// No substring either side uses occurs in the other, so a claim checked against
// the wrong text always fails. That is what turns §3.3.2's "each claim is
// checked against exactly one source, so the two rules never collide" into
// something a test can observe: with overlapping texts, a note claim wrongly
// checked against the issue text would pass and the routing would go unmeasured.
const (
	trackerText = "The upload retries on a 5xx response.\n"
	noteText    = "The customer confirmed a ten-second cap in chat."
	noteOne     = "CR-1#n1"
)

// disjointSpans is the SpanTexts those two make: one issue text, one standing
// note, sharing no wording.
func disjointSpans() SpanTexts {
	return SpanTexts{
		Issue: trackerText,
		Notes: []note.Note{{ID: noteOne, Text: noteText, Source: note.SourceChat, PR: 1}},
	}
}

// aClaimLine is one claim as an agent writes it, with the note_id left off when
// it is empty so the line is exactly what §3.3 admits for its source.
func aClaimLine(source, span, noteID string) []byte {
	line := `{"id":"CR-1#c1","text":"The upload is retried.","source":"` + source +
		`","span":"` + span + `"`
	if noteID != "" {
		line += `,"note_id":"` + noteID + `"`
	}
	return []byte(line + "}\n")
}

// §3.3.1 and §3.3.2, both directions: each claim is checked against exactly one
// text, and the two rules never collide.
//
// The four cases the criterion names are the four here — a valid tracker span,
// an invalid tracker span, a valid note span, and a note span checked against
// the wrong source — and the last is written twice, once in each direction,
// because the collision has two ways to happen and each is silent on its own.
//
// A tracker span that occurs only in the note is the reading in which a claim
// drawn from the issue text is checked against hearsay: it must be refused, and
// refused by naming the issue text, so the user is told which rule applied. A
// note span that occurs only in the issue text is the reading §3.3.2 rules out
// from the other side: the claim carries a note_id, rests on that note, and is
// exactly the claim §8.1.6 has to disclose as weakly founded, so validating it
// against the tracker would buy it the tracker's provenance for nothing.
func TestAClaimIsCheckedAgainstOneTextAndNeverTheOther(t *testing.T) {
	spans := disjointSpans()

	accepted, err := DecodeClaims(
		claimsFile, aClaimLine("acceptance", "retries on a 5xx", ""), "CR-1", spans,
	)
	require.NoError(t, err, "§3.3.1: a tracker span that occurs in the issue text stands")
	assert.Equal(t, "retries on a 5xx", accepted[0].Span)

	accepted, err = DecodeClaims(
		claimsFile, aClaimLine("note", noteText, noteOne), "CR-1", spans,
	)
	require.NoError(t, err, "§3.3.2: a note span that is the named note's body stands")
	assert.Equal(t, noteOne, accepted[0].NoteID,
		"§8.1.6 discloses the note, so the claim keeps naming it")

	notTheBody := "claims.ndjson line 1: span is not the body of note " + noteOne +
		"; §3.3.2 sets the span of a claim drawn from a note to that note's body"
	for name, tc := range map[string]struct{ source, span, noteID, want string }{
		"a tracker span that is in neither text": {
			source: "acceptance", span: "refunded within a day",
			want: "claims.ndjson line 1: span does not occur in the issue text" +
				"; §3.3 draws a claim from a verbatim substring of its source",
		},
		"a tracker span that is only in the note": {
			source: "acceptance", span: "ten-second cap",
			want: "claims.ndjson line 1: span does not occur in the issue text" +
				"; §3.3 draws a claim from a verbatim substring of its source",
		},
		"a note span that is only in the issue text": {
			source: "note", span: "retries on a 5xx", noteID: noteOne, want: notTheBody,
		},
		"a note span that is part of the named note": {
			source: "note", span: "ten-second cap", noteID: noteOne, want: notTheBody,
		},
		"a note span that is the named note with a sentence added": {
			source: "note", span: noteText + " It was twice.", noteID: noteOne, want: notTheBody,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeClaims(
				claimsFile, aClaimLine(tc.source, tc.span, tc.noteID), "CR-1", spans,
			)
			var rejected *RejectedClaimError
			require.ErrorAs(t, err, &rejected, "§3.3 rejects a span its source does not hold")
			assert.Equal(t, "span", rejected.Field)
			assert.Equal(t, tc.want, err.Error(),
				"the refusal names the one text the claim was checked in, and the rule")
		})
	}
}

// §3.3.2 sets a note-sourced claim's span to the note's body, and "normalised"
// is §1.4's one comparison of text: a span that differs from the body only in
// the whitespace §1.4 folds is that body, and its span_hash is the body's.
func TestANoteSpanIsComparedToTheBodyNormalised(t *testing.T) {
	spaced := "The customer  confirmed a ten-second\\tcap in chat.  \\n\\n"
	accepted, err := DecodeClaims(
		claimsFile, aClaimLine("note", spaced, noteOne), "CR-1", disjointSpans(),
	)
	require.NoError(t, err, "§1.4 folds the spacing, so the span is the note's body")
	require.Len(t, accepted, 1)
	assert.Equal(t, "The customer  confirmed a ten-second\tcap in chat.  \n\n", accepted[0].Span,
		"the span is stored as the agent wrote it")

	require.NoError(t, ComputeClaimHashes(accepted, trackerText))
	bodyHash, err := text.NormalisedHash(noteText)
	require.NoError(t, err)
	assert.Equal(t, bodyHash, accepted[0].SpanHash, "an accepted span hashes as the body does")
}

// §3.3.2 validates a note-sourced claim against "the named note", so an id
// naming no note leaves the rule with nothing to carry out.
//
// The refusal is on `note_id` rather than on `span`, because the span is not
// what is wrong: it may well be a faithful copy of a note somebody recorded
// somewhere else. What fails is the provenance §8.1.6 would disclose — cr would
// print a note id no store holds — and the check itself, since falling through
// to the issue text is the one collision §3.3.2 rules out.
func TestANoteSourcedClaimNamingNoNoteIsRefused(t *testing.T) {
	_, err := DecodeClaims(
		claimsFile, aClaimLine("note", "ten-second cap", "CR-1#n9"), "CR-1", disjointSpans(),
	)
	var rejected *RejectedClaimError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, "note_id", rejected.Field)
	assert.Equal(t,
		`claims.ndjson line 1: note_id names "CR-1#n9", and the context store for CR-1 holds `+
			`no such note; §3.3.2 validates this claim against that note and against nothing else`,
		err.Error())
}

// A retracted note still founds a claim, and that is a reading of §3.3.2 and
// §3.6.6 together rather than an oversight.
//
// §3.3.2 says the claim is validated against the named note and says nothing
// about its standing. §3.6.6 gives retraction a different consequence
// altogether: a coverage cell or record citing a retracted note is *reported as
// needing re-evaluation in the next round* rather than silently retained — a
// report, not a refusal, and one that nothing can produce if the claim was
// never recorded in the first place.
//
// note.Standing is derived from the store every time it is asked for, so `cr
// draft` and `cr post` read the retraction as it stands when they run. Refusing
// here would move that decision to extraction time, where neither section puts
// it, and would lose the audit trail §3.6.6 keeps the retracted note on disk for.
func TestARetractedNoteStillFoundsItsClaim(t *testing.T) {
	at := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	spans := disjointSpans()
	spans.Notes[0].RetractedAt = &at
	require.Equal(t, note.StandingRetracted, note.StandingOf(spans.Notes, noteOne),
		"the fixture's note is retracted, which is what this case is about")

	accepted, err := DecodeClaims(
		claimsFile, aClaimLine("note", noteText, noteOne), "CR-1", spans,
	)
	require.NoError(t, err,
		"§3.6.6 reports a citation of a retracted note; it does not refuse the extraction")
	assert.Equal(t, noteOne, accepted[0].NoteID)
}
