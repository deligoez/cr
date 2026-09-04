package reinvention

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/profile"
)

// shippedGeneric is the generic profile cr really ships, parsed from the bytes
// `cr init` writes. §2.4.3 makes configuration the only way to reach it, and
// what naming it buys is a review with the reinvention half honestly out.
func shippedGeneric(t *testing.T) *profile.Profile {
	t.Helper()
	p, err := profile.Parse("generic.json", []byte(profile.Builtins()["generic"]))
	require.NoError(t, err)
	require.Equal(t, "generic", p.ID)
	require.Empty(t, p.Symbols.Lang, "the generic profile knows no language, which is what it has to say")
	return &p
}

// §4.3.1: when no symbol index can be built, the reinvention half of the
// convention axis is marked unavailable per §4.5 rather than skipped silently —
// and the rest of that axis still runs, because §4.3.5 sends it to the rule
// corpus of §2.6, whose per-repository and global layers outlive any profile.
//
// Both halves of that sentence are asserted together on purpose. Reporting the
// whole convention axis out would tell the author cr never looked at their
// conventions when it did; reporting nothing would tell them cr looked for
// reinvention and found none when it never could.
func TestUnderTheGenericProfileTheReinventionHalfIsOutAndTheAxisRuns(t *testing.T) {
	p := shippedGeneric(t)

	active := activation.Activate(p, intent.Intent{})
	assert.Contains(t, active.Active, axis.Convention,
		"§4.3.5 leaves the rest of the convention axis to the rule corpus of §2.6")

	attachments := Attach(p, nil, []git.Hunk{addedAt("app/new.go", 3)})

	assert.Empty(t, attachments.Attached, "with no index there is no candidate to attach")
	require.Len(t, attachments.Unavailable, 1)
	entry := attachments.Unavailable[0]
	assert.Equal(t, profile.ReinventionLens, entry.Lens)
	assert.Equal(t, axis.Convention+"/reinvention", entry.Lens,
		"the lens is half of the convention axis and never the axis itself")
	assert.Contains(t, entry.Reason, "symbols.lang",
		"§4.5.4's reason names what would make the lens run")
}

// §11.1 exempts the honesty disclosures from `--quiet`, and
// finding.HonestyDisclosure is the shape the writer holding that exemption
// consumes. Implementing it is what puts the entry in that channel instead of
// in an ordinary informational line a flag can silence.
func TestTheUnavailableReinventionHalfIsAnHonestyDisclosure(t *testing.T) {
	attachments := Attach(shippedGeneric(t), nil, nil)
	require.Len(t, attachments.Unavailable, 1)

	var quietProof finding.HonestyDisclosure = attachments.Unavailable[0]

	assert.Equal(t,
		"lens convention/reinvention unavailable, per §4.3.1: "+
			"profile \"generic\" declares no symbols.lang, so §4.3.1's symbol index cannot be built; "+
			"set symbols.lang in the profile to name this repository's language",
		quietProof.Disclosure())
}

// The three states that leave the lens out are three different sentences to the
// person reading the report, and each names what would make it run. A profile
// that never claimed a language, a language cr cannot read, and an index that
// was asked for and did not arrive are not one another's fault, and a single
// reason would send the reader to fix the wrong thing.
func TestEachWayTheIndexIsMissingHasItsOwnReason(t *testing.T) {
	for name, tc := range map[string]struct {
		lang     string
		contains string
	}{
		"the profile declares no language": {
			"", "declares no symbols.lang"},
		"cr has no scanner for the language it declares": {
			"cobol", "which cr has no symbol scanner for"},
	} {
		t.Run(name, func(t *testing.T) {
			p := &profile.Profile{ID: "fixture", Symbols: profile.Symbols{Lang: tc.lang}}

			attachments := Attach(p, nil, nil)

			require.Len(t, attachments.Unavailable, 1)
			assert.Contains(t, attachments.Unavailable[0].Reason, tc.contains)
		})
	}

	t.Run("the index was asked for and did not arrive", func(t *testing.T) {
		attachments := Attach(indexable(), nil, nil)

		require.Len(t, attachments.Unavailable, 1)
		assert.Contains(t, attachments.Unavailable[0].Reason, "cr built no symbol index for symbols.lang \"go\"",
			"a git read that failed is not a profile that was misconfigured")
	})
}

// The entry is data as well as a sentence, so a caller rendering §10.1.3's
// report and a caller printing §4.5.4's list cannot disagree about which lens
// was out.
func TestTheUnavailableEntryCarriesTheLensAndTheReasonAsFields(t *testing.T) {
	attachments := Attach(shippedGeneric(t), nil, nil)

	encoded, err := json.Marshal(attachments.Unavailable[0])
	require.NoError(t, err)
	var decoded map[string]string
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, profile.ReinventionLens, decoded["lens"])
	assert.NotEmpty(t, decoded["reason"])
}
