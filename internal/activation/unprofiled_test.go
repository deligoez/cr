package activation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/profile"
)

// §2.4.4 disables every axis that requires a profile and no other, and §4.5.3
// still decides the intent axis on the issue key. The disabled set is asserted
// against profile.Unmatched's own list, so the axis report and §2.4.4's report
// cannot name different axes, and OfRound is asserted to give a round that
// recorded no profile the same answer a brief gives.
func TestUnprofiledDisablesOnlyTheAxesThatNeedAProfile(t *testing.T) {
	t.Run("a key resolved", func(t *testing.T) {
		a := Unprofiled(resolved(t))
		assert.Equal(t, []string{axis.Intent, axis.Correctness, axis.Convention}, a.Active)
		assert.Equal(t, []off{{Axis: axis.Test, Rule: RuleNoTestCommand}}, switchedOff(a.Disabled))
		assert.Equal(t, profile.Unmatched().Disabled, []string{a.Disabled[0].Axis})
		assert.Empty(t, axesOf(a.Unavailable))
	})

	t.Run("no key resolved", func(t *testing.T) {
		a := Unprofiled(unresolved(t))
		assert.Equal(t, []string{axis.Correctness, axis.Convention}, a.Active)
		assert.Equal(t, []off{{Axis: axis.Test, Rule: RuleNoTestCommand}}, switchedOff(a.Disabled))
		assert.Equal(t, []string{axis.Intent}, axesOf(a.Unavailable))
	})

	t.Run("a round that recorded no profile", func(t *testing.T) {
		assert.Equal(t, Unprofiled(unresolved(t)), OfRound(&profile.Profile{}, "", "", keyPattern))
	})
}
