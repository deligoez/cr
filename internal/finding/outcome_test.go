package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §7.3.4: the demotion numerator is `discarded-wrong + withdrawn-wrong +
// softened`, and §7.2 keeps both `not-here` outcomes out of it in as many words
// — a true finding not worth saying on this pull request is no evidence the
// class is imprecise. A kept record is the class being right.
func TestOnlyWrongAndSoftenedCountAgainstTheClass(t *testing.T) {
	for outcome, counts := range map[Outcome]bool{
		OutcomeKept:             false,
		OutcomeSoftened:         true,
		OutcomeDiscardedNotHere: false,
		OutcomeDiscardedWrong:   true,
		OutcomeWithdrawnNotHere: false,
		OutcomeWithdrawnWrong:   true,
	} {
		assert.Equal(t, counts, outcome.CountsAgainstClass(), string(outcome))
	}
}

// §7.3.6: the volume numerator is both `not-here` outcomes, and nothing that
// says the class was wrong. The two sets share no outcome, which is what lets
// a class be a demotion candidate, a volume candidate, both, or neither.
func TestOnlyTheNotHereOutcomesCountTowardVolume(t *testing.T) {
	for _, action := range TriageActions() {
		outcome := Outcome(action)
		if action == ActionRaised {
			continue
		}
		want := outcome == OutcomeDiscardedNotHere || outcome == OutcomeWithdrawnNotHere
		assert.Equal(t, want, outcome.NotHere(), string(outcome))
		assert.False(t, outcome.NotHere() && outcome.CountsAgainstClass(),
			"%s cannot be evidence for volume and against the class at once", outcome)
	}
}

// A withdrawal's rate is its class's: `withdrawn-wrong` enters §7.3.4's
// numerator and `withdrawn-not-here` §7.3.6's, over the same raises.
func TestAWithdrawalCountsInTheRateItsDispositionNames(t *testing.T) {
	counts := TriageCounts{Raised: 4, Kept: 1, WithdrawnWrong: 2, WithdrawnNotHere: 1}

	assert.InDelta(t, 0.5, counts.DemotionRate(), 1e-9)
	assert.InDelta(t, 0.25, counts.NotHereRate(), 1e-9)
}

// §9.6.2: the disposition is the reviewer's, and each names one outcome.
func TestAWithdrawalOutcomeFollowsItsDisposition(t *testing.T) {
	wrong, err := WithdrawalOutcome(DispositionWrong)
	require.NoError(t, err)
	assert.Equal(t, OutcomeWithdrawnWrong, wrong)

	notHere, err := WithdrawalOutcome(DispositionNotHere)
	require.NoError(t, err)
	assert.Equal(t, OutcomeWithdrawnNotHere, notHere)

	_, err = WithdrawalOutcome("")
	var unknown *UnknownDispositionError
	require.ErrorAs(t, err, &unknown, "a posted record carries no disposition, and none is guessed for it")
}
