package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// §6.2.2 through the grade: the fixture probe targets handler.go:43, inside
// aGradedRecord's 42..44 range, and grades it probed. The same probe, with
// every other condition still met, grades argued a record whose anchor ends one
// line above the target, and a record whose anchor is LEFT on the very lines
// the target names.
func TestAProbeGradesOnlyTheRecordWhoseRightAnchorHoldsItsTarget(t *testing.T) {
	proven, passing := provenProbe(t, gradedHead)
	grade := func(record *Finding) Grade {
		return ComputeGrade(record, Resolved(theUnit, &record.Anchor, proven, gradedHead, passing, theMapping()))
	}

	holding := aGradedRecord()
	assert.Equal(t, GradeProbed, grade(holding), "the target falls within 42..44")

	short := aGradedRecord()
	short.Anchor.StartLine, short.Anchor.Line = 41, 42
	assert.Equal(t, GradeArgued, grade(short), "§6.2.2: a target one line outside the anchor range supports nothing")

	removed := aGradedRecord()
	removed.Anchor.Side = "LEFT"
	assert.Equal(t, GradeArgued, grade(removed), "§6.2.2: a LEFT-anchored record cannot reach probed")
}
