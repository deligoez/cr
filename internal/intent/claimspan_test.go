package intent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/note"
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
		claimsFile, aClaimLine("note", "ten-second cap", noteOne), "CR-1", spans,
	)
	require.NoError(t, err, "§3.3.2: a note span that occurs in the named note stands")
	assert.Equal(t, noteOne, accepted[0].NoteID,
		"§8.1.6 discloses the note, so the claim keeps naming it")

	for name, tc := range map[string]struct{ source, span, noteID, in string }{
		"a tracker span that is in neither text": {
			source: "acceptance", span: "refunded within a day", in: "the issue text",
		},
		"a tracker span that is only in the note": {
			source: "acceptance", span: "ten-second cap", in: "the issue text",
		},
		"a note span that is only in the issue text": {
			source: "note", span: "retries on a 5xx", noteID: noteOne, in: "note " + noteOne,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeClaims(
				claimsFile, aClaimLine(tc.source, tc.span, tc.noteID), "CR-1", spans,
			)
			var rejected *RejectedClaimError
			require.ErrorAs(t, err, &rejected, "§3.3 rejects a span its source does not hold")
			assert.Equal(t, "span", rejected.Field)
			assert.Equal(t,
				"claims.ndjson line 1: span does not occur in "+tc.in+
					"; §3.3 draws a claim from a verbatim substring of its source",
				err.Error(),
				"the refusal names the one text the claim was checked in")
		})
	}
}
