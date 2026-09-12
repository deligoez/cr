package finding

import (
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
)

// The head the fixtures below were graded at, and one that is not it.
const (
	gradedHead    = "3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f"
	unrelatedHead = "aa11bb22cc33dd44ee55ff6600718293a4b5c6d7"
	gradedRound   = 2
	gradedUnit    = "u1"
	gradedClaim   = "CR-7#c1"
)

// holding stands in for the record's own unit: the locations it holds, by path.
//
// It is a lookup rather than a range test on purpose. §6.2.1's predicate is
// unit.Unit's — its hunk ranges are recorded in the head coordinates the
// section evaluates containment in, and internal/unit's own test holds it to
// the section — so a second range test written here would be a second reading
// of the same rule, agreeing with the first only on the day it was written.
// What this package owns is which side of the boundary buys the grade, and a
// fixture that simply names the inside says exactly that.
type holding map[string][]int

func (h holding) Contains(path string, line int) bool {
	return slices.Contains(h[path], line)
}

// theUnit is the unit every fixture record sits on: one file, three lines of it.
var theUnit = holding{"internal/api/handler.go": {42, 43, 44}}

// resolvedCitation is one entry of §6.1's citations array as it stands after
// §6.2.3 has resolved it: the hash is cr's, stamped from the line the head
// holds, and the origin is cr's too.
//
// The hash is what tells a resolved entry from one that never reached the head,
// so a fixture that left it off would be testing the unresolved case by
// accident.
func resolvedCitation(path string, line int, origin Origin) Citation {
	return Citation{Path: path, Line: line, ContentHash: "0f1e2d3c4b5a6978", Origin: origin}
}

// aGradedRecord is a valid §6.1 record on the fixture unit, carrying prose that
// asserts far more than the record's evidence supports. Every case below starts
// from it, so what decides a grade is the field the case changed.
func aGradedRecord() *Finding {
	return &Finding{
		ID: "f1", Kind: KindFinding, Axis: "correctness", Role: "correctness",
		Class:    "unchecked-error",
		Severity: SeverityHigh, Unit: gradedUnit, Claim: gradedClaim,
		Anchor: Anchor{
			Path: "internal/api/handler.go", Side: "RIGHT", StartLine: 42, Line: 44,
			ContentHash: "0123456789abcdef",
		},
		Summary: "The error Decode returns is dropped.",
		Evidence: "Proven by experiment: the mutation was applied, the suite was run, " +
			"and the gap is confirmed and verified beyond doubt.",
	}
}

// provenProbe is a mutation probe on §5.3.5's rung, with the passing baseline
// §5.2.6 admits for it, at the head under review.
func provenProbe(t *testing.T, head string) (*probe.Record, probe.Baseline) {
	t.Helper()
	record := &probe.Record{
		ID: "p1", Kind: probe.Mutation, Result: "no-test-failed", Baseline: "r1",
		Target: "internal/api/handler.go:43",
		Stamp:  state.Stamp{Head: head, Round: gradedRound},
	}
	baseline, ok := record.ResolveBaseline([]run.Record{{
		ID: "r1", Passed: true, Stamp: state.Stamp{Head: head, Round: gradedRound},
	}})
	require.True(t, ok, "the fixture run has to be one §5.2.6 admits as this probe's baseline")
	return record, baseline
}

// theMapping is §4.1.6's mapping joining the fixture claim to the fixture unit.
func theMapping() probe.ClaimMapping {
	return probe.MapClaim([]mapping.Pair{{
		Claim: gradedClaim, Unit: gradedUnit, Stamp: state.Stamp{Round: gradedRound},
	}}, gradedRound, gradedClaim, gradedUnit)
}

// §6.2's table, row by row, over one record and the evidence cr resolved for it.
//
// The `cited` row is three separate conditions and each is exercised on its
// own: an entry cr resolved, that entry lying outside the record's own unit or
// carrying `origin: rule`, and — asserted in the case below this one — the
// record's axis not being `test`.
//
// The citation inside the unit is the case the row is written to refuse. A
// record cites to give the human somewhere to look that the summary can be
// checked against, and a location inside the very hunks the record is about
// gives them the code they are already reading. `origin: rule` is the one
// exception, because then the location is cr's own detection output rather than
// the agent's choice of where to point.
func TestTheGradeIsSection62sTableComputedFromTheEvidence(t *testing.T) {
	proven, passing := provenProbe(t, gradedHead)
	stale, staleBaseline := provenProbe(t, unrelatedHead)

	for name, tc := range map[string]struct {
		citations []Citation
		probe     *probe.Record
		baseline  probe.Baseline
		grade     Grade
	}{
		"a probe on §5.3.5's rung at this head": {
			probe: proven, baseline: passing, grade: GradeProbed,
		},
		"a probe outranks the citations it comes with": {
			citations: []Citation{resolvedCitation("internal/api/handler.go", 42, OriginAgent)},
			probe:     proven, baseline: passing, grade: GradeProbed,
		},
		"a citation outside the record's own unit": {
			citations: []Citation{resolvedCitation("internal/api/store.go", 7, OriginAgent)},
			grade:     GradeCited,
		},
		"a citation in the unit's file but outside its hunks": {
			citations: []Citation{resolvedCitation("internal/api/handler.go", 90, OriginAgent)},
			grade:     GradeCited,
		},
		"a rule's own hit inside the unit": {
			citations: []Citation{resolvedCitation("internal/api/handler.go", 43, OriginRule)},
			grade:     GradeCited,
		},
		"one entry outside is enough": {
			citations: []Citation{
				resolvedCitation("internal/api/handler.go", 42, OriginAgent),
				resolvedCitation("internal/api/store.go", 7, OriginAgent),
			},
			grade: GradeCited,
		},
		"a citation inside the record's own unit": {
			citations: []Citation{resolvedCitation("internal/api/handler.go", 43, OriginAgent)},
			grade:     GradeArgued,
		},
		"an entry cr never resolved": {
			citations: []Citation{{Path: "internal/api/store.go", Line: 7}},
			grade:     GradeArgued,
		},
		"a probe from another head": {
			probe: stale, baseline: staleBaseline, grade: GradeArgued,
		},
		"no probe and no citation": {grade: GradeArgued},
	} {
		t.Run(name, func(t *testing.T) {
			record := aGradedRecord()
			record.Citations = tc.citations
			assert.Equal(t, tc.grade, ComputeGrade(record, Resolved(
				theUnit, &record.Anchor, tc.probe, gradedHead, tc.baseline, theMapping(),
			)))
		})
	}
}

// §6.2.1: the grade is computed by cr from the record, and `evidence` prose is
// never parsed.
//
// The fixture's evidence says "proven", "confirmed" and "verified" in one
// sentence, and its summary asserts a defect outright. Neither moves the grade
// off `argued`, because neither is an input: a record that names no experiment
// and cites no location has nothing behind it however it is written, and the
// register it reaches the author in is §6.3's business rather than the prose's.
//
// This is the whole of P5 at the one place it is easiest to lose. An
// implementation that read the evidence for the word "probe" would be forming
// the judgement §2.1.3 gives the agent, and it would do it invisibly — the
// record would look identical on disk.
func TestARecordWhoseProseClaimsProofButCitesNothingIsArgued(t *testing.T) {
	record := aGradedRecord()
	require.Empty(t, record.Citations, "the fixture cites nothing; only the prose claims anything")

	graded := ComputeGrade(record, Resolved(theUnit, &record.Anchor, nil, gradedHead, probe.Baseline{}, theMapping()))

	assert.Equal(t, GradeArgued, graded,
		"§6.2's third row: neither of the above, whatever the evidence sentence says")
}

// The closed input set of §6.2.1, made structural.
//
// Evidence is the only way the two inputs a record does not carry reach the
// computation, and its fields are what the computation may read. Asserting the
// field set is asserting that there is nowhere to put a summary, a severity, a
// role or an evidence sentence — so "the only inputs are ..." is a property of
// the type rather than a discipline at each call site.
//
// The record's own five rows are not asserted here, because they are asserted
// by the table above: every case differs from every other only in `citations`
// and in the probe cr resolved, and the grades come out different.
func TestGradingHasNowhereToPutAFieldSection621DoesNotName(t *testing.T) {
	inputs := reflect.TypeFor[Evidence]()
	named := make([]string, 0, inputs.NumField())
	for i := range inputs.NumField() {
		named = append(named, inputs.Field(i).Name)
	}
	assert.Equal(t, []string{"own", "probed"}, named,
		"§6.2.1: the unit's boundary and the referenced probe's answer, and nothing else")

	for i := range inputs.NumField() {
		assert.False(t, inputs.Field(i).IsExported(),
			"%s is unexported, so Resolved is the only way to fill it", named[i])
	}
}

// Prose the grade does not read is prose no field of the record can smuggle in.
//
// The two records below differ in every agent-written sentence and in the
// severity and role the agent chose, and agree in the five rows §6.2.1 names.
// They grade alike, which is the sentence "`evidence` prose is never parsed"
// stated as an equality rather than as an absence.
func TestTwoRecordsAgreeingOnSection621sInputsGradeAlike(t *testing.T) {
	evidence := Resolved(theUnit, &aGradedRecord().Anchor, nil, gradedHead, probe.Baseline{}, theMapping())
	cited := []Citation{resolvedCitation("internal/api/store.go", 7, OriginAgent)}

	modest := aGradedRecord()
	modest.Citations = cited
	modest.Summary = "It is not clear what happens to the error here."
	modest.Evidence = "I could not tell from reading it."
	modest.Severity = SeverityLow
	modest.Role = "convention"

	insistent := aGradedRecord()
	insistent.Citations = cited
	insistent.Severity = SeverityCritical

	assert.Equal(t,
		ComputeGrade(modest, evidence), ComputeGrade(insistent, evidence),
		"the grade is the evidence's, not the register the agent wrote in")
}

// Round 13's mutable-grading-input: a recomputation MUST NOT raise a record's
// grade within a round.
//
// §6.3.1 recomputes three times over one round, and §4.1.6 lets the mapping —
// a grading input — be replaced between them. So a record can reach record time
// as `argued`, be forced to a question, be shown to the human as a question and
// approved as one, and then meet a stronger grade at post time. The reviewer
// would have approved a question and the author would receive an assertion.
//
// Lowering is left open, and is not the mirror of the same risk: a record whose
// evidence stopped standing up says less than it did, and §6.3 turns the weaker
// grade into a question. The direction that has to be refused is the one that
// takes a record out of the question register behind the human's back.
func TestARecomputationLowersAGradeAndNeverRaisesIt(t *testing.T) {
	proven, passing := provenProbe(t, gradedHead)
	anchor := &aGradedRecord().Anchor
	nothing := Resolved(theUnit, anchor, nil, gradedHead, probe.Baseline{}, theMapping())
	everything := Resolved(theUnit, anchor, proven, gradedHead, passing, theMapping())

	raised := aGradedRecord()
	Regrade(raised, nothing)
	require.Equal(t, GradeArgued, raised.Grade, "the first computation stands, whatever it is")
	Regrade(raised, everything)
	assert.Equal(t, GradeArgued, raised.Grade,
		"§6.3.1 recomputes within the round, and a recomputation may not raise the grade")

	lowered := aGradedRecord()
	Regrade(lowered, everything)
	require.Equal(t, GradeProbed, lowered.Grade)
	Regrade(lowered, nothing)
	assert.Equal(t, GradeArgued, lowered.Grade,
		"a recomputation may lower it: evidence that stopped standing up says less")

	held := aGradedRecord()
	Regrade(held, everything)
	Regrade(held, everything)
	assert.Equal(t, GradeProbed, held.Grade, "and may leave it unchanged")
}

// The ratchet is over §6.4.2's order, so every pair of grades is covered rather
// than the two the round-13 finding happens to name.
//
// `cited` is the middle rung and the one a two-value fixture cannot reach: a
// record graded `cited` meeting fresh `probed` evidence is exactly the
// mid-round rise §6.3.1 makes possible, and it is refused like the others.
func TestTheRatchetHoldsOverEveryPairOfGrades(t *testing.T) {
	for _, held := range gradeStrength {
		for _, computed := range gradeStrength {
			settled := notAbove(computed, held)
			assert.NotEqual(t, -1, slices.Index(gradeStrength, settled),
				"the answer is always one of §6.2's three")
			assert.GreaterOrEqual(t,
				rankIn(gradeStrength, settled), rankIn(gradeStrength, held),
				"%s held, %s computed: the settled grade is never stronger than the held one",
				held, computed)
		}
	}

	assert.Equal(t, GradeProbed, notAbove(GradeProbed, ""),
		"a record that has not been graded holds no grade to be raised above")
}
