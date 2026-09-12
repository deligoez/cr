package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/probe"
)

// The axis §6.2's `cited` row denies is §1.5's, spelled the same.
//
// grade.go holds the id as a literal because internal/axis's own tests reach
// internal/config, internal/render and this package, so importing the closed
// set from grade.go would close a cycle there. Nothing forbids the import in
// this direction, and this is the assertion that keeps the two spellings one
// string: an axis renamed in §1.5 and not here would leave §4.4.2's denial
// pointing at nothing, and every test-adequacy record could buy `cited`.
func TestTheDeniedAxisIsSection15sTestAxis(t *testing.T) {
	assert.Equal(t, axis.Test, testAxis,
		"§6.2's cited row denies the axis §1.5 names test")
	require.True(t, axis.Valid(testAxis), "and §1.5's closed set still holds it")
}

// §4.4.2: a test-adequacy finding asserts only with an experiment, and
// citations to test files never grade it `cited`.
//
// §6.2's `cited` row carries ", and the record's `axis` is not `test`", so the
// denial is not about which files were cited — it is the whole axis. Both
// outcomes are asserted: the same record grades `argued` on its citations alone
// and `probed` once an experiment stands behind it, and there is no third
// answer for the axis.
//
// The citations are to test files, which is the shape §4.4.2 is written
// against: a role reading the suite can always find a test file to point at,
// and pointing at one is not evidence that the behaviour is untested. Only
// §5.3's `no-test-failed` establishes that — the suite ran, selected tests, and
// noticed nothing — which is why the grade the row withholds is exactly the one
// an experiment restores.
func TestATestAdequacyRecordIsExactlyProbedOrArgued(t *testing.T) {
	proven, passing := provenProbe(t, gradedHead)

	cited := aGradedRecord()
	cited.Axis, cited.Role = testAxis, "test-adequacy"
	cited.Citations = []Citation{
		resolvedCitation("internal/api/handler_test.go", 12, OriginAgent),
		resolvedCitation("internal/api/store_test.go", 90, OriginAgent),
	}

	assert.Equal(t, GradeArgued, ComputeGrade(cited, Resolved(
		theUnit, &cited.Anchor, nil, gradedHead, probe.Baseline{}, theMapping(),
	)), "§4.4.2: citations to test files grade a test-adequacy record nothing")

	probed := *cited
	probed.Probe = proven.ID
	assert.Equal(t, GradeProbed, ComputeGrade(&probed, Resolved(
		theUnit, &probed.Anchor, proven, gradedHead, passing, theMapping(),
	)), "and the experiment is what lets it assert")
}

// The denial is the axis's and not the citation's path.
//
// A record on any other axis grades `cited` on exactly the citations the record
// above was refused for, so what refused them was `axis: test` — not something
// about a `_test.go` path, which §6.2 never mentions and cr must not start
// reading file names for.
func TestTheSameCitationsGradeARecordOnAnotherAxisCited(t *testing.T) {
	elsewhere := aGradedRecord()
	elsewhere.Citations = []Citation{
		resolvedCitation("internal/api/handler_test.go", 12, OriginAgent),
	}

	assert.Equal(t, GradeCited, ComputeGrade(elsewhere, Resolved(
		theUnit, &elsewhere.Anchor, nil, gradedHead, probe.Baseline{}, theMapping(),
	)), "§6.2's cited row reads the axis, and no rule of it reads a path")
}

// An axis cr did not compute cannot reach `cited`.
//
// §6.1 makes `axis` the axis of the record's `role`, written by cr, and §6.1.4
// refuses it on the wire — so an empty axis is a role §2.5.5's corpus did not
// resolve, not a record on some fifth axis. §6.2's row asks cr to establish
// that the axis is not `test`, and cr cannot establish that about an axis it
// never worked out.
//
// It is the safe direction and only that: the record falls to `argued`, which
// §6.3 forces to a question, so nothing is asserted on the strength of a role
// nobody could look up.
func TestAnAxisCrDidNotComputeCannotReachCited(t *testing.T) {
	unresolved := aGradedRecord()
	unresolved.Axis, unresolved.Role = "", "a-role-the-corpus-does-not-hold"
	unresolved.Citations = []Citation{
		resolvedCitation("internal/api/store.go", 7, OriginAgent),
	}

	assert.Equal(t, GradeArgued, ComputeGrade(unresolved, Resolved(
		theUnit, &unresolved.Anchor, nil, gradedHead, probe.Baseline{}, theMapping(),
	)))
}
