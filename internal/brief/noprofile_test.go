package brief

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/profile"
)

// §2.4.4 disables "every axis that requires" a profile, not every axis, and
// §4.5.4 then has every lens that did not run reported with its reason. The
// active set and the report are asserted together because each is only honest
// beside the other: a round that ran nothing and said only that the test axis
// was off — which is what a brief did here before — is a report whose every line
// is true and whose silence about intent, correctness and convention is not.
//
// The shipped profiles are installed and the fixture carries no marker file, so
// nothing matching is the real outcome of the real candidates, and the key
// resolves, so the intent axis has what §4.5.3 asks of it and runs.
func TestARepositoryNoProfileMatchesRunsEveryAxisThatNeedsNone(t *testing.T) {
	dir, head, base := repository(t)
	src := sources(t, dir, answering(head, base, noThreads))
	shipped(t, src.Layout)

	assembled, err := Run(src)
	require.NoError(t, err)
	require.False(t, assembled.Profile.Selected, "the fixture carries no marker file")
	require.Equal(t, testIssue, assembled.Issue.Key)

	testOff := activation.Disabled{
		Axis: axis.Test,
		Rule: activation.RuleNoTestCommand,
		Reason: "no profile matched this repository, so no tests.cmd is declared for this axis to run; " +
			"set `profile` in the per-repository config to name the profile this repository is",
	}
	assert.Equal(t, activation.Activation{
		Active:      []string{axis.Intent, axis.Correctness, axis.Convention},
		Disabled:    []activation.Disabled{testOff},
		Unavailable: []intent.Unavailable{},
	}, assembled.Axes)
	assert.Equal(t, []string{"convention", "correctness", "intent-coverage"}, assembled.ActiveRoles)

	recorded, err := src.Layout.Briefed(testOwner, testRepo, testPR,
		func() (string, error) { return head, nil })
	require.NoError(t, err)
	assert.Equal(t, assembled.ActiveRoles, recorded.ActiveRoles,
		"meta.json carries the roles that run, so §4.5.6 accepts their cells")

	// §2.4.4's report names the test axis and the reinvention half, so the
	// axis entry for the test axis is not repeated before it; the one role
	// that does not look is named after it with the axis sentence that
	// decided it.
	assert.Equal(t, []string{
		"no profile matched this repository, so cr disabled every axis that needs one rather than guessing: " +
			"axis test disabled, per §4.5.2; lens convention/reinvention unavailable, per §4.3.1; " +
			"set `profile` in the per-repository config to name the profile this repository is",
		"role test-adequacy skipped, per §4.6.4: " + testOff.Disclosure(),
	}, rendered(assembled))
	assert.Equal(t, profile.Unmatched(), *assembled.Profile.Missing)
}
