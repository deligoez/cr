package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Field-feedback 2.6: the round's record contract states §8.1.5's rule as a
// whole line of its own — a posted question body carries "?", composed at
// draft time — so no role learns it first from `cr post`'s refusal. Since
// 0.3.0 §4.6.2 that contract is the round's contract file, which every prompt
// names.
func TestTheRecordContractStatesThatAPostedQuestionAsks(t *testing.T) {
	assert.Contains(t, Contract(1), "\nA kind=question record's posted body must contain \"?\" (§8.1.5); "+
		"that body is composed at draft time, where a question body that does not ask is rewritten into "+
		"one before `cr post` accepts it.\n")
}
