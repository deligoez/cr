package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// resting is a record graded cited, written as an assertion, in the given class
// and resting on the given claim.
func resting(id, class, claim string) *Finding {
	record := classed(class)
	record.ID, record.Grade, record.Kind, record.Claim = id, GradeCited, KindFinding, claim
	return record
}

// §3.6.6: a record resting on a claim whose note no longer stands is held as a
// question and counted per class, while a record on a standing claim and a
// record naming no claim keep the register the agent wrote. The grade is left
// as §6.2 computed it.
func TestARecordRestingOnAWithdrawnNoteIsHeldAsAQuestion(t *testing.T) {
	first := resting("f1", "unchecked-error", "CR-1#c1")
	standing := resting("f2", "unchecked-error", "CR-1#c2")
	unclaimed := resting("f3", "unchecked-error", "")
	second := resting("f4", "naming-drift", "CR-1#c1")
	third := resting("f5", "unchecked-error", "CR-1#c3")
	records := []*Finding{first, standing, unclaimed, second, third}
	withdrawn := map[string]bool{"CR-1#c1": true, "CR-1#c3": true}

	held := ForceWithdrawn(records, withdrawn)

	assert.Equal(t, Withdrawn{
		{Class: "naming-drift", Count: 1},
		{Class: "unchecked-error", Count: 2},
	}, held, "a count per class, by class ascending")
	for _, record := range []*Finding{first, second, third} {
		assert.Equal(t, KindQuestion, record.Kind, "%s rests on a withdrawn note", record.ID)
		assert.Equal(t, GradeCited, record.Grade, "%s keeps the grade §6.2 computed", record.ID)
	}
	assert.Equal(t, KindFinding, standing.Kind, "a standing note leaves the register alone")
	assert.Equal(t, KindFinding, unclaimed.Kind, "a record naming no claim rests on no note")

	assert.Equal(t, held, ForceWithdrawn(records, withdrawn),
		"the count is of the records held, so a second application reports the same")
}

// The report is printed at zero and names each class when something is held,
// and an empty report is an empty slice rather than a nil one.
func TestTheWithdrawnReportIsADisclosure(t *testing.T) {
	var quietProof HonestyDisclosure = ForceWithdrawn(nil, nil)
	assert.NotNil(t, quietProof)
	assert.Equal(t, "§3.6.6: 0 records resting on a withdrawn note held as question",
		quietProof.Disclosure())

	held := ForceWithdrawn([]*Finding{
		resting("f1", "unchecked-error", "CR-1#c1"),
		resting("f2", "naming-drift", "CR-1#c1"),
		resting("f3", "unchecked-error", "CR-1#c1"),
	}, map[string]bool{"CR-1#c1": true})
	assert.Equal(t,
		"§3.6.6: 3 records resting on a withdrawn note held as question — naming-drift 1, unchecked-error 2",
		held.Disclosure())
}
