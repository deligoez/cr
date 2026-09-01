package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
)

// The head every fixture here was measured at, and one that is not it.
const (
	gradingHead = "3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f"
	otherHead   = "aa11bb22cc33dd44ee55ff6600718293a4b5c6d7"
)

// passingBaseline resolves the Baseline a probe record points at, out of a
// stored run that passed at the same head.
//
// It goes through ResolveBaseline rather than building the value, because there
// is no other way: §5.2.6's fence lives in that resolution, and a test that
// could construct a Baseline directly would be testing a type this package does
// not have.
func passingBaseline(t *testing.T, record *Record, passed bool) Baseline {
	t.Helper()
	resolved, ok := record.ResolveBaseline([]run.Record{{
		ID: record.Baseline, Passed: passed, Filter: record.Filter,
		Stamp: state.Stamp{Head: record.Head},
	}})
	require.True(t, ok, "the fixture run has to be one §5.2.6 admits")
	return resolved
}

// mapped is §4.1.6's mapping with one pair in it, which is §5.4.4's second
// condition answered yes for that claim and unit.
func mapped(round int, claim, unit string) ClaimMapping {
	return MapClaim([]mapping.Pair{{
		Claim: claim, Unit: unit, Stamp: state.Stamp{Round: round},
	}}, round, claim, unit)
}

// §5.3.5's result condition, read off the stored record rather than off the run.
//
// The record is what §6.2.1 names as grading's input, and the Outcome Proves
// reads is gone by then, so this is the form the condition survives in. The
// values are varied across the mutation vocabulary because §5.3.5 admits
// exactly one of them: `no-test-failed` is the rung on which the suite ran,
// selected tests, and noticed nothing, and every other rung — the timeout that
// established nothing, the `error` §5.1.7 writes over a voided probe, the empty
// selection, the unparseable recap, and the mutation the suite caught — says
// something else or says nothing.
//
// A gap probe carrying `no-test-failed` is refused by kind, which is §5.5's
// "the result vocabulary is per kind and MUST NOT be shared" read at the one
// place that would otherwise let a gap probe be taken for the missing test only
// §5.3 establishes.
func TestProvenIsSection535sResultConditionOverTheStoredRecord(t *testing.T) {
	for name, tc := range map[string]struct {
		record Record
		proven bool
	}{
		"a mutation the suite did not notice": {
			Record{Kind: Mutation, Result: resultNoTestFailed}, true,
		},
		"a mutation the suite caught": {
			Record{Kind: Mutation, Result: resultFailed}, false,
		},
		"a run that never finished": {
			Record{Kind: Mutation, Result: resultTimeout}, false,
		},
		"a probe §5.1.7 voided": {
			Record{Kind: Mutation, Result: ResultError}, false,
		},
		"a run that selected nothing": {
			Record{Kind: Mutation, Result: resultNoTestsSelected}, false,
		},
		"a run whose counts did not read": {
			Record{Kind: Mutation, Result: resultInconclusive}, false,
		},
		"a gap probe carrying the mutation ladder's value": {
			Record{Kind: Gap, Result: resultNoTestFailed}, false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.proven, Proven(&tc.record))
		})
	}
}

// §6.2's `probed` row: the referenced probe's head matches, and its result
// supports the claim per §5.3 and §5.4.
//
// Each kind is routed to its own ladder and to no other. The two records below
// carry the value the *other* kind's ladder rewards as well as the value its
// own does, so a router that read one vocabulary for both would grade one of
// them wrongly in a way a same-vocabulary fixture could not show.
func TestEstablishesRoutesEachKindToItsOwnLadder(t *testing.T) {
	const round, claim, unit = 2, "CR-7#c1", "u1"

	mutationProbe := func(result Result) Record {
		return Record{
			ID: "p1", Kind: Mutation, Result: result, Baseline: "r1",
			Stamp: state.Stamp{Head: gradingHead, Round: round},
		}
	}
	gapProbe := func(result Result) Record {
		return Record{
			ID: "p2", Kind: Gap, Result: result, Baseline: "r1", Target: "app.go:3",
			Stamp: state.Stamp{Head: gradingHead, Round: round},
		}
	}

	for name, tc := range map[string]struct {
		record      Record
		establishes bool
	}{
		"a mutation on §5.3.5's rung": {mutationProbe(resultNoTestFailed), true},
		"a mutation the suite caught": {mutationProbe(resultFailed), false},
		"a gap probe that reproduced": {gapProbe(resultFailed), true},
		"a gap probe that passed":     {gapProbe(resultPassed), false},
	} {
		t.Run(name, func(t *testing.T) {
			record := tc.record
			assert.Equal(t, tc.establishes, Establishes(
				&record, gradingHead,
				passingBaseline(t, &record, true), mapped(round, claim, unit),
			))
		})
	}
}

// The three ways §6.2's `probed` row is refused that are not about the result:
// no probe at all, a probe from another head, and a baseline that did not pass.
//
// The last two are the conditions §5.5.3, §5.3.5 and §5.4.4 add on top of the
// ladder, and each is asserted against a record carrying the one result its own
// ladder rewards — so what refuses is the condition under test and not the run.
func TestEstablishesRefusesWhatSection553AndTheBaselineConditionRefuse(t *testing.T) {
	const round, claim, unit = 2, "CR-7#c1", "u1"
	proving := Record{
		ID: "p1", Kind: Mutation, Result: resultNoTestFailed, Baseline: "r1",
		Stamp: state.Stamp{Head: gradingHead, Round: round},
	}
	holds := mapped(round, claim, unit)

	assert.False(t, Establishes(nil, gradingHead, Baseline{}, holds),
		"a record referencing no probe has not met §6.2's first row")

	assert.False(t, Establishes(&proving, otherHead, passingBaseline(t, &proving, true), holds),
		"§5.5.3: a probe from another head grades no finding in this round")

	assert.False(t, Establishes(&proving, gradingHead, passingBaseline(t, &proving, false), holds),
		"§5.3.5: a suite already red at this head makes every mutation look survivable")

	assert.False(t, Establishes(&proving, gradingHead, Baseline{}, holds),
		"and a baseline §5.2.6 admitted none for did not pass either")
}

// §5.4.4's second condition is the gap probe's alone: an unmapped claim refuses
// the gap probe and leaves the mutation probe untouched.
//
// The two are asserted in one test because the asymmetry is the point. §5.3.5
// states two conditions and §5.4.4 states three, and a router that applied the
// mapping to both would silently make every mutation probe depend on a tracker
// — which §4.6.6 makes empty in a repository that has none, so the whole
// mutation half of §6.2 would go dark in exactly the repositories §4.5.3 says
// keep working.
func TestOnlyTheGapLadderReadsTheRoundsMapping(t *testing.T) {
	const round = 2
	unmapped := MapClaim(nil, round, "CR-7#c1", "u1")

	proving := Record{
		ID: "p1", Kind: Mutation, Result: resultNoTestFailed, Baseline: "r1",
		Stamp: state.Stamp{Head: gradingHead, Round: round},
	}
	reproducing := Record{
		ID: "p2", Kind: Gap, Result: resultFailed, Baseline: "r1", Target: "app.go:3",
		Stamp: state.Stamp{Head: gradingHead, Round: round},
	}

	assert.True(t, Establishes(&proving, gradingHead, passingBaseline(t, &proving, true), unmapped),
		"§5.3.5 states two conditions and the mapping is neither of them")
	assert.False(t, Establishes(
		&reproducing, gradingHead, passingBaseline(t, &reproducing, true), unmapped),
		"§5.4.4's second condition is unmet, so the probe supports no probed grade")
}

// A kind outside §5.5's two establishes nothing.
//
// CheckResult refuses such a record at the write, so this is unreachable
// through a stored line — which is exactly why it is asserted. §6.2's `argued`
// row is "neither of the above", and a router falling through to the mutation
// ladder for an unrecognised kind would hand the strongest grade to the one
// record cr does not understand.
func TestAKindOutsideSection55EstablishesNothing(t *testing.T) {
	unknown := Record{
		ID: "p1", Kind: "fuzz", Result: resultNoTestFailed, Baseline: "r1",
		Stamp: state.Stamp{Head: gradingHead},
	}
	assert.False(t, Establishes(
		&unknown, gradingHead, passingBaseline(t, &unknown, true), ClaimMapping{}))
}
