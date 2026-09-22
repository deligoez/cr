package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
