package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// classed is a record graded argued, written by the agent as an assertion, in
// the given defect class.
func classed(class string) *Finding {
	record := argued()
	record.Class = class
	return record
}

// §6.3.2: the forcing is reported with a count per class.
//
// The two classes carry different counts, so the report is a breakdown rather
// than a total wearing a class name, and the classes go in ascending order so
// the same round renders the same report twice and a stored summary diffs
// cleanly.
func TestTheForcingIsReportedWithACountPerClass(t *testing.T) {
	records := []*Finding{
		classed("unchecked-error"),
		classed("naming-drift"),
		classed("unchecked-error"),
	}

	forced := ForceQuestions(records)

	assert.Equal(t, Forcings{
		{Class: "naming-drift", Count: 1},
		{Class: "unchecked-error", Count: 2},
	}, forced, "§6.3.2: a count per class, by class ascending")
	assert.Equal(t, 3, forced.Total())
	for _, record := range records {
		assert.Equal(t, KindQuestion, record.Kind, "§6.3.1 forced every one of them")
	}
}

// The count is of the records the forcing holds, not of the moves one call
// made.
//
// §6.3.1 applies the forcing three times, so by the second application every
// record the first one moved is already a question and ForceQuestion reports
// false for all of them. A report counting moves would therefore read zero at
// draft time and again at post time — the two moments a human sees it — which
// is the same as not reporting at all.
func TestTheCountDoesNotFadeWhenTheForcingIsAppliedAgain(t *testing.T) {
	records := []*Finding{classed("unchecked-error"), classed("naming-drift")}

	first := ForceQuestions(records)
	require.Equal(t, 2, first.Total())

	assert.Equal(t, first, ForceQuestions(records),
		"§6.3.2 counts the grade §6.3 forces, at every moment it is applied")
}

// Only the argued grade is counted, because only the argued grade is forced.
// A question the agent chose to ask about a cited finding was not forced into
// one, and reporting it would have cr claiming a restraint it did not impose.
func TestOnlyTheForcedRecordsAreCounted(t *testing.T) {
	asking := classed("naming-drift")
	asking.Grade, asking.Kind = GradeCited, KindQuestion

	forced := ForceQuestions([]*Finding{classed("unchecked-error"), asking})

	assert.Equal(t, Forcings{{Class: "unchecked-error", Count: 1}}, forced)
	assert.Equal(t, KindQuestion, asking.Kind, "and the agent's own question stands")
}

// §11.1 exempts §6.3.2's forcing counts from `--quiet`, and
// finding.HonestyDisclosure is the shape the writer holding that exemption
// consumes. Implementing it is what lets the report reach that writer.
func TestTheForcingCountIsAnHonestyDisclosure(t *testing.T) {
	var quietProof HonestyDisclosure = Forcings{}
	assert.Equal(t, "§6.3: 0 records forced to question", quietProof.Disclosure(),
		"a round that forced nothing says so, so silence never stands for it")

	forced := ForceQuestions([]*Finding{
		classed("unchecked-error"), classed("unchecked-error"), classed("naming-drift"),
	})
	assert.Equal(t,
		"§6.3: 3 records forced to question — naming-drift 1, unchecked-error 2",
		forced.Disclosure())
}

// An empty report is an empty slice rather than a nil one, per §12.3: it is a
// field on a printed payload, and a null there would read as a round in which
// the forcing was never asked about.
func TestAnEmptyForcingReportIsAnEmptySlice(t *testing.T) {
	assert.NotNil(t, ForceQuestions(nil))
	assert.Empty(t, ForceQuestions([]*Finding{}))
}

// §6.3.3: a record graded `argued` written as a finding is refused, and the
// refusal names the record id.
//
// The offender is put second of three so the two records around it prove what
// the refusal is about: a `cited` finding may assert per §6.2, and an `argued`
// question is exactly where §6.3 puts one, so neither is refused and the one
// that is refused is refused for the pair of fields §6.3.3 names.
func TestAnArguedAssertionIsRefusedByRecordID(t *testing.T) {
	asked := classed("naming-drift")
	asked.ID, asked.Kind = "f1", KindQuestion
	asserting := classed("unchecked-error")
	asserting.ID = "f2"
	cited := classed("unchecked-error")
	cited.ID, cited.Grade = "f3", GradeCited

	require.NoError(t, RefuseArguedAssertion([]*Finding{asked, cited}),
		"§6.2 lets a cited record assert, and an argued question is where §6.3 puts one")

	err := RefuseArguedAssertion([]*Finding{asked, asserting, cited})
	require.Error(t, err)

	var refused *ArguedAssertionError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "f2", refused.Record, "§6.3.3: the refusal names the record id")
	assert.Contains(t, err.Error(), "f2")
	assert.Contains(t, err.Error(), "§6.3.3")
}

// Applying §6.3.1 satisfies §6.3.3, which is the pair that makes invariant 4 a
// property of the records rather than of a call having been made.
//
// The check runs over the same records the forcing just moved, so a forcing
// that stopped working would be caught by the refusal rather than reaching the
// author as an assertion. That is why both are applied at each of §6.3.1's
// three moments and not only the first.
func TestTheForcingSatisfiesTheRefusalItIsCheckedBy(t *testing.T) {
	records := []*Finding{classed("unchecked-error"), classed("naming-drift")}
	require.Error(t, RefuseArguedAssertion(records), "the agent wrote them as assertions")

	ForceQuestions(records)
	assert.NoError(t, RefuseArguedAssertion(records),
		"§6.3.1 applied leaves nothing for §6.3.3 to refuse")
}
