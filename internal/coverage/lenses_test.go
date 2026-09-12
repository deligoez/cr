package coverage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/reinvention"
)

// keyPattern is §3.2's default, spelled out so the resolution below is the one
// a run with no `intent.key_pattern` set would perform.
const keyPattern = `[A-Z][A-Z0-9]+-[0-9]+`

// builtin parses one of the profiles v0.1 ships, so the axis decision under
// test is taken over a real profile rather than over a literal written to
// produce the answer the test wants.
func builtin(t *testing.T, id string) profile.Profile {
	t.Helper()
	p, err := profile.Parse(id+".json", []byte(profile.Builtins()[id]))
	require.NoError(t, err)
	return p
}

// unresolved is a §3.2 resolution that found no key: the branch carries none
// and no other source is offered, which is the state §4.5.3 marks the intent
// axis unavailable for.
func unresolved(t *testing.T) intent.Intent {
	t.Helper()
	i, err := intent.Resolve(intent.KeySources{Branch: "feature/add-a-thing"}, keyPattern, intent.Source{})
	require.NoError(t, err)
	return i
}

// skippedRole is §4.6.4's entry for a role whose prerequisites were unmet. The
// producer is `cr review`, which is review-skipped-roles' to build; what this
// package owns is that the entry has a shape and reaches the report.
var skippedRole = SkippedRole{
	Role:   "test-adequacy",
	Reason: "the resolved profile declares no tests.globs, so this role has no test file to read",
}

// All four kinds of §4.5.4 entry reach the reader together with §10.2's
// completeness verdict.
//
// This is lens-honesty-reporting's third criterion, and the fixture is built
// the way the criterion words it: `generic` declares no `tests.cmd`, so §4.5.2
// disables the test axis; the branch carries no issue key, so §4.5.3 marks the
// intent axis unavailable; the same profile declares no `symbols.lang`, so
// §4.3.1's reinvention half cannot run; and one role is skipped per §4.6.4.
// Three of the four are asked of the packages that own them rather than
// hand-written here, so a change to what those packages report is a change this
// test sees.
//
// The assertion is that all four arrive in one call with the verdict. §10.2
// requires a verdict of complete to be printed together with every lens that
// did not run, because completeness across three axes is not completeness
// across four and neither is completeness with a role that never looked.
func TestAllFourKindsOfLensReachTheCompletenessVerdict(t *testing.T) {
	p := builtin(t, "generic")
	axes := activation.Activate(&p, unresolved(t))
	require.Len(t, axes.Disabled, 1, "§4.5.2: generic declares no tests.cmd")
	require.Len(t, axes.Unavailable, 1, "§4.5.3: the branch carries no issue key")

	half := reinvention.Attach(&p, nil, nil, reinvention.Ranking{})
	require.Len(t, half.Unavailable, 1, "§4.3.1: generic declares no symbols.lang")

	lenses := Lenses{
		Axes:   axes.Disclosures(),
		Halves: []finding.HonestyDisclosure{half.Unavailable[0]},
		Roles:  []SkippedRole{skippedRole},
	}

	lines := lenses.Verdict(true, "")
	require.Len(t, lines, 5, "§10.2: the verdict and one line per lens that did not run")
	assert.Equal(t, "round complete, per §10.2", lines[0])

	printed := strings.Join(lines, "\n")
	for name, reason := range map[string]string{
		"the disabled axis":          axes.Disabled[0].Reason,
		"the unavailable axis":       axes.Unavailable[0].Reason,
		"the reinvention half":       half.Unavailable[0].Reason,
		"the role skipped by §4.6.4": skippedRole.Reason,
	} {
		assert.Containsf(t, printed, reason,
			"§4.5.4: %s reaches the reader with its reason", name)
	}
}

// A verdict of false carries the exact reason, and a verdict of true carries
// none.
//
// §10.2 asks for the reason only when the verdict is false, and the asymmetry
// is the point: a round is complete when the four conditions hold and there is
// nothing further to say, so a sentence invented for the true case would be cr
// forming a judgement about a round it only counted.
func TestTheVerdictCarriesItsReasonOnlyWhenItIsFalse(t *testing.T) {
	empty := Lenses{}

	complete := empty.Verdict(true, "")
	require.Len(t, complete, 1, "every lens ran, so the verdict stands alone")
	assert.Equal(t, "round complete, per §10.2", complete[0])

	blocked := empty.Verdict(false, "u2 holds no cell for role correctness")
	require.Len(t, blocked, 1)
	assert.Contains(t, blocked[0], "not complete")
	assert.Contains(t, blocked[0], "u2 holds no cell for role correctness",
		"§10.2: a false verdict names the exact reason")
}

// Every kind the report collects satisfies finding.HonestyDisclosure, which is
// the channel §11.1 exempts from `--quiet`.
//
// The skipped role is the one of §4.5.4's four kinds this package introduces,
// so it is the one that could reach the report as data and leave the terminal
// silent. The two words are asserted as mutually exclusive for the reason
// internal/activation asserts the same pair: §4.5.4's kinds are different
// sentences to the reader, and one carrying another's word lets them take the
// wrong one.
func TestASkippedRoleReachesTheReaderAsADisclosure(t *testing.T) {
	var _ finding.HonestyDisclosure = SkippedRole{}

	text := skippedRole.Disclosure()
	assert.Contains(t, text, skippedRole.Role)
	assert.Contains(t, text, skippedRole.Reason)
	assert.Contains(t, text, "skipped")
	assert.Contains(t, text, "§4.6.4")
	assert.NotContains(t, text, "unavailable")
	assert.NotContains(t, text, "disabled")
}
