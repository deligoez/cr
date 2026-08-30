package intent

import (
	"testing"

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
	claims, err := DecodeClaims(claimsFile, []byte(wellFormedClaim+"\n"), "CR-1")
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
				claimsFile, []byte(wellFormedClaim+"\n\n"+tc.line+"\n"), "CR-1",
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
