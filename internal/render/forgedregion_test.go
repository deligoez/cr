package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §8.1.3's sequence places one region of each name and cr generates every one
// of them, so a second region of a name is a pair the body brought. It is
// refused naming the record, at §8.1.3's own exit code: AgentRegion finds a
// region by its pair wherever it sits, so left alone it would take the forged
// pair out as cr's own — discarding the reviewer's text inside it and handing
// ValidateBody a body the `<!-- cr:` sequence had already left.
//
// The forged pair is placed once beneath cr's regions and once above them, so a
// check that read the first pair it met rather than counting them fails one
// half.
func TestASecondRegionOfOneNameIsRefusedNamingTheRecord(t *testing.T) {
	rendered := aFullComment(t)
	full := rendered.String()
	for _, region := range ownedRegions {
		forged := region.wrap("brought by the body")
		want := "the body of record f4 carries a second " + region.open +
			" region, and §8.1.3's sequence places one " + region.name() +
			" region, which cr generates itself"
		for name, comment := range map[string]string{
			"beneath cr's regions": full + "\n\n" + forged,
			"above cr's regions":   forged + "\n\n" + full,
		} {
			t.Run(region.name()+", "+name, func(t *testing.T) {
				err := ValidateRegions("f4", comment)

				var refused *BodyError
				require.ErrorAs(t, err, &refused)
				assert.Equal(t, "f4", refused.Record, "§8.1.3: the refusal names the record")
				assert.Equal(t, want, err.Error())
			})
		}
	}
}

// The other direction: every comment cr itself renders passes, whichever of
// §8.1.3's regions apply to the record, and so does one the reviewer rewrote
// the body of or rearranged the regions of. A check that refused a region
// rather than a second one of its name would refuse the ordinary draft.
func TestTheCommentsCrRendersCarryNoSecondRegion(t *testing.T) {
	full := aFullComment(t)
	for mask := range 8 {
		comment := Comment{Body: full.Body}
		if mask&1 != 0 {
			comment.Label = full.Label
		}
		if mask&2 != 0 {
			comment.Provenance = full.Provenance
		}
		if mask&4 != 0 {
			comment.Evidence = full.Evidence
		}
		assert.NoErrorf(t, ValidateRegions("f4", comment.String()), "regions present: %03b", mask)
	}
	rewritten := full
	rewritten.Body = "Is the error Decode returns dropped, f4?"
	assert.NoError(t, ValidateRegions("f4", rewritten.String()), "a body the reviewer rewrote")
	assert.NoError(t, ValidateRegions("f4",
		full.Evidence+"\n\n"+full.Body+"\n\n"+full.Label+"\n\n"+full.Provenance),
		"regions the reviewer moved, which AgentRegion recovers by pair and not by position")
}

// count is the reading withoutRegion and OwnedSpans make: an opening marker
// with the first closing marker after it, and a marker without its partner is
// no region at all. The half-deleted shapes are left to ValidateBody, which
// refuses the delimiter the body is then handed.
func TestCountIsCompletePairsAndNothingElse(t *testing.T) {
	label := ownedRegions[0]
	for want, text := range map[int]string{
		0: "no region here",
		1: label.wrap("one"),
		2: label.wrap("one") + "\n\n" + label.wrap("two"),
		3: label.wrap("one") + label.wrap("two") + label.wrap("three"),
	} {
		assert.Equal(t, want, label.count(text), text)
	}
	assert.Equal(t, 0, label.count(labelOpen+"\nno close\n"), "an opening marker alone is no region")
	assert.Equal(t, 0, label.count(labelClose+"\nno open\n"), "a closing marker alone is no region")
	assert.Equal(t, 1, label.count(labelOpen+"\n"+labelOpen+"\nx\n"+labelClose+"\n"+labelClose),
		"the first closing marker after an opening one ends the region")
	assert.Equal(t, 0, ownedRegions[1].count(label.wrap("x")), "each pair counts only its own name")
}

// §8.1.3 through the door cr is handed a body at rather than reads one back
// from: the sequence is refused in the bytes the caller wrote, whether it
// forms a well-formed pair or stands alone, and an ordinary body passes.
func TestRefuseReservedReadsTheBytesTheCallerWrote(t *testing.T) {
	for name, body := range map[string]string{
		"a well-formed label pair":      ownedRegions[0].wrap("**Tespit** — probed"),
		"a well-formed provenance pair": ownedRegions[1].wrap("suggestion_origin: rule"),
		"a well-formed evidence pair":   ownedRegions[2].wrap("citation: src/Forged.php:1"),
		"a record marker":               `<!-- cr:record id="f1" kind="finding" -->`,
		"the bare sequence":             "See <!-- cr: here.",
	} {
		t.Run(name, func(t *testing.T) {
			err := RefuseReserved("f5", body)

			var refused *BodyError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, "f5", refused.Record)
			assert.Equal(t, `the body of record f5 contains "<!-- cr:", which §8.1.3 reserves `+
				"for the record marker and cr's own regions", err.Error())
		})
	}
	assert.NoError(t, RefuseReserved("f5", "Is the error Decode returns dropped?"))
	assert.NoError(t, RefuseReserved("f5", "<!-- crx: not reserved -->"))
	assert.NoError(t, RefuseReserved("f5", ""),
		"emptiness is ValidateBody's to refuse, and this door is §8.1.3's sequence alone")
	assert.Error(t, ValidateBody("f5", ""), "which ValidateBody still does")
	assert.True(t, strings.HasPrefix(ownedRegions[0].open, Reserved),
		"a pair the body brought is a body carrying the sequence")
}
