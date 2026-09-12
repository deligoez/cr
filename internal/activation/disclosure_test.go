package activation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// §4.5.4 requires every lens that did not run to appear in the coverage report
// with its reason, and §11.1 exempts those reports from `--quiet` through
// finding.HonestyDisclosure. An axis switched off in the data and left out of
// the printed report would be honest to a caller reading JSON and silent to the
// human reading a terminal, which is the exact failure §4.5.4 exists to prevent.
//
// The two words are asserted as mutually exclusive, not merely present. §4.5.2
// says "disabled" and §4.5.3 says "marked unavailable", §4.5.4 lists them as two
// of its four kinds, and §3.7.6 asks the brief for "the active, disabled, and
// unavailable axes" — so a reader told "unavailable" learns cr was prevented
// from looking, and one told "disabled" learns cr was told not to. A disclosure
// carrying both words would let the reader take the wrong one.
func TestEveryAxisThatDidNotRunReachesTheReaderAsADisclosure(t *testing.T) {
	var _ finding.HonestyDisclosure = Disabled{}

	// `generic` declares no tests.cmd and this run resolved no issue key, so
	// one axis leaves by each of the two doors at once.
	p := builtin(t, "generic")
	a := Activate(&p, unresolved(t))
	require.Len(t, a.Disabled, 1)
	require.Len(t, a.Unavailable, 1)

	disclosures := a.Disclosures()
	require.Len(t, disclosures, 2, "§4.5.4 owes the reader one entry per lens that did not run")

	// The text is asserted against the fields rather than against a literal,
	// because the two must not be able to drift.
	disabledText := disclosures[0].Disclosure()
	assert.Contains(t, disabledText, a.Disabled[0].Axis)
	assert.Contains(t, disabledText, a.Disabled[0].Rule)
	assert.Contains(t, disabledText, a.Disabled[0].Reason)
	assert.Contains(t, disabledText, "disabled")
	assert.NotContains(t, disabledText, "unavailable")

	unavailableText := disclosures[1].Disclosure()
	assert.Contains(t, unavailableText, a.Unavailable[0].Axis)
	assert.Contains(t, unavailableText, a.Unavailable[0].Reason)
	assert.Contains(t, unavailableText, "unavailable")
	assert.NotContains(t, unavailableText, "disabled")
}

// A round with nothing disabled and one axis unavailable is the asymmetry
// §4.5.4 has to survive, and it is the ordinary case rather than a corner:
// every shipped profile but `generic` declares a `tests.cmd`, so a review of a
// pull request that resolves no issue key reaches Disclosures with an empty
// Disabled list beside a populated Unavailable one.
//
// gremlins found this, and what it found was not a missing assertion but a
// wrong annotation. The sum sizing the result slice was recorded here as an
// equivalent capacity hint; it is not. `len(a.Disabled)-len(a.Unavailable)` is
// -1 in this state and `make` panics with `makeslice: cap out of range`, so the
// brief of §3.7.6 and the report of §10.1.3 would both abort on a run whose only
// peculiarity is an absent tracker. The one fixture that called Disclosures had
// one entry on each side, where the difference is zero and nothing shows.
func TestADisclosureListSurvivesMoreUnavailableAxesThanDisabledOnes(t *testing.T) {
	p := builtin(t, "laravel-pest")
	a := Activate(&p, unresolved(t))
	require.Empty(t, a.Disabled, "laravel-pest enables every axis and declares tests.cmd")
	require.Len(t, a.Unavailable, 1)

	disclosures := a.Disclosures()
	require.Len(t, disclosures, 1, "§4.5.4 owes the reader the axis that was left unavailable")
	assert.Contains(t, disclosures[0].Disclosure(), a.Unavailable[0].Axis)
	assert.Contains(t, disclosures[0].Disclosure(), "unavailable")
}
