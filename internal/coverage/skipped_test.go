package coverage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/role"
)

// A role skipped on an axis that ran is given the reason that decided, and a
// list that admits the resolved profile is never offered as that reason.
//
// Measured before the fix: a role whose profiles list is empty was reported as
// "its profiles list names  and this round resolved profile generic; the role
// looks only under a profile it names" — a sentence naming nothing, pointing at
// a list that admits every profile. Such a role is left out by the active-role
// set `cr brief` recorded, and the reason now says so; a list that names
// another profile still decides, and still names itself.
func TestASkippedRoleOnARunningAxisIsGivenTheReasonThatDecided(t *testing.T) {
	axes := activation.Activation{Active: []string{"correctness"}}
	corpus := []role.Resolved{
		{Role: role.Role{ID: "error-paths", Axis: "correctness", Profiles: []string{}}},
		{Role: role.Role{ID: "generic-only", Axis: "correctness", Profiles: []string{"generic"}}},
		{Role: role.Role{ID: "pest-only", Axis: "correctness", Profiles: []string{"laravel-pest"}}},
	}

	skipped := Skipped(axes, corpus, []string{}, "generic")

	require.Len(t, skipped, 3)
	reasons := map[string]string{}
	for _, entry := range skipped {
		reasons[entry.Role] = entry.Reason
	}
	assert.Equal(t, "this round's active roles, which `cr brief` recorded per §4.5.1, do not name it, "+
		"though its axis correctness ran and its profiles list is empty, which admits every profile; "+
		"run `cr brief` again to record the round's active roles anew", reasons["error-paths"])
	assert.NotContains(t, reasons["error-paths"], "names  ", "the sentence carries no empty name")
	assert.Contains(t, reasons["generic-only"], "its profiles list names profile generic",
		"a list naming the resolved profile admits the role, so it did not decide either")
	assert.Equal(t, "its profiles list names laravel-pest and this round resolved profile generic; "+
		"the role looks only under a profile it names", reasons["pest-only"],
		"a list excluding the resolved profile is what decided, and it is named")
}
