package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
)

// The pull request the fixtures below sit on: one head, one round, one claim
// mapped to one unit.
const (
	supportHead  = "9f8e7d6"
	supportRound = 2
	supportClaim = "CR-1#c2"
	supportUnit  = "u1"
)

// §5.4.4: a failed gap probe supports the finding only when its baseline passed
// and the finding's `claim` field names a claim mapped to the finding's unit.
//
// The row that matters most is the mismatched claim against a passing baseline.
// It is the row an agent reaches by writing one field — `claim` is the agent's
// to write, and the mapping is not — so it is the whole difference between a
// probe that licenses an assertion to a colleague and one that leaves the
// record argued under §6.2 and asked as a question under §6.3.
//
// The empty-mapping row is the same refusal arrived at from the other side, and
// round 12's axis-availability-coupling finding is about it: a repository with
// tests and no tracker has no mapping at all, so the condition is unreachable
// rather than unmet. GapUnmappable is what says so out loud.
func TestAFailedGapProbeSupportsAFindingOnlyOnAPassingBaselineAndAMappedClaim(t *testing.T) {
	mapped := mappedPairs()
	for _, tc := range []struct {
		name     string
		passed   bool
		pairs    []mapping.Pair
		claim    string
		unit     string
		supports bool
	}{
		{
			name:     "a passing baseline and a claim mapped to the unit",
			passed:   true,
			pairs:    mapped,
			claim:    supportClaim,
			unit:     supportUnit,
			supports: true,
		},
		{
			name:   "the same experiment against a baseline that did not pass",
			passed: false,
			pairs:  mapped,
			claim:  supportClaim,
			unit:   supportUnit,
		},
		{
			name:   "a claim field naming a claim mapped to some other unit",
			passed: true,
			pairs:  mapped,
			claim:  supportClaim,
			unit:   "u2",
		},
		{
			name:   "a claim field naming a claim the mapping does not hold",
			passed: true,
			pairs:  mapped,
			claim:  "CR-1#c9",
			unit:   supportUnit,
		},
		{
			name:   "a record naming no claim at all",
			passed: true,
			pairs:  mapped,
			unit:   supportUnit,
		},
		{
			name:   "an empty mapping, which is §4.6.6's repository with no tracker",
			passed: true,
			claim:  supportClaim,
			unit:   supportUnit,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := gapRecord(resultFailed)

			supports := Supports(
				record,
				resolvedFor(t, record, tc.passed),
				MapClaim(tc.pairs, supportRound, tc.claim, tc.unit),
			)

			assert.Equal(t, tc.supports, supports,
				"§5.4.4: the baseline passed and the claim is mapped, or the probe supports nothing")
		})
	}
}

// §5.4.5: none of the five results it names supports a `probed` grade, and the
// baseline and the mapping are held at their most generous to prove it.
//
// Both conditions §5.4.4 asks for are met in every row — the baseline passed
// and the claim is mapped to the unit — so the only thing refusing support is
// the result itself. That is the shape §5.4.5 has: `passed` and the four the
// section lists beside it are refused unconditionally, and a record resting on
// one falls to `argued` under §6.2 and is asked as a question under §6.3.
//
// `passed` is then separated from the other four. It is the one result of the
// five that establishes something — that the behaviour the supplied test
// asserts is present — and §5.4.5's whole first sentence exists to keep that
// from being read as the missing test only §5.3's `no-test-failed` establishes.
func TestNoGapResultOtherThanFailedSupportsAFinding(t *testing.T) {
	for _, tc := range []struct {
		result  Result
		present bool
	}{
		{result: resultPassed, present: true},
		{result: resultTimeout},
		{result: ResultError},
		{result: resultNoTestsSelected},
		{result: resultInconclusive},
	} {
		t.Run(string(tc.result), func(t *testing.T) {
			record := gapRecord(tc.result)

			assert.False(t, Supports(
				record,
				resolvedFor(t, record, true),
				MapClaim(mappedPairs(), supportRound, supportClaim, supportUnit),
			), "§5.4.5 refuses this result a probed grade however good the rest is")
			assert.Equal(t, tc.present, Present(record),
				"§5.4.5: a passed gap probe establishes the behaviour, and no other result does")
		})
	}
}

// §5.4.4's first half, which is all the run itself settles: the supplied test
// failed, and cr cannot tell whether the behaviour or the test is wrong.
//
// The mutation row is the one worth having. `failed` is a value both
// vocabularies of §5.5 hold and the two sections read it opposite ways — §5.3.7
// has the agent not raise the finding at all, §5.4.4 has the result recorded
// and its output offered as a reproduction — so a gap-side predicate that
// answered for a mutation record would read one section's evidence under the
// other's rule.
func TestOnlyAFailedGapProbeIsOfferedAsAReproduction(t *testing.T) {
	for _, tc := range []struct {
		kind      Kind
		result    Result
		reproduce bool
	}{
		{kind: Gap, result: resultFailed, reproduce: true},
		{kind: Gap, result: resultPassed},
		{kind: Gap, result: ResultError},
		{kind: Mutation, result: resultFailed},
		{kind: Mutation, result: resultNoTestFailed},
	} {
		t.Run(string(tc.kind)+" "+string(tc.result), func(t *testing.T) {
			record := gapRecord(tc.result)
			record.Kind = tc.kind

			assert.Equal(t, tc.reproduce, Reproduces(record),
				"§5.4.4 speaks of a failed gap probe and of no other record")
		})
	}
}

// §4.1.6 and §9.3.5: the mapping the condition is read from is the round's, and
// a pair some earlier round wrote is not it.
//
// §3.4.6 makes a unit id round-scoped and forbids carrying it across rounds, so
// `u1` of round 1 is a different piece of code from `u1` of round 2. A mapping
// read from the whole file would license this round's probe on the strength of
// a judgement made about code the round never formed.
func TestTheMappedClaimIsReadFromTheRoundsOwnPairs(t *testing.T) {
	earlier := []mapping.Pair{{
		Claim: supportClaim, Unit: supportUnit,
		Stamp: state.Stamp{Head: "0000000", Round: supportRound - 1},
	}}
	record := gapRecord(resultFailed)

	assert.False(t, Supports(
		record,
		resolvedFor(t, record, true),
		MapClaim(earlier, supportRound, supportClaim, supportUnit),
	), "the pair belongs to an earlier round, so this round has no mapping for the unit")
}

// §5.5's `baseline` column and §5.2.6's fence: the verdict §5.4.4 reads comes
// from the run this probe was measured against, and only when that run is one
// §5.2.6 admits.
//
// The rows after the first are the fence. A run carrying a `probe` measured
// mutated or probe-injected code; a contaminated one measured a sandbox §5.1.6
// found unclean; a run at another head measured other code. None of them is a
// baseline, whatever the probe record names — and resolving one anyway would
// hand §5.4.4 a `passed: true` that was never about this experiment.
func TestTheProbesOwnBaselineIsResolvedByIdAndStillFenced(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stored   run.Record
		resolved bool
	}{
		{
			name:     "the run the probe record names",
			stored:   supportBaseline(),
			resolved: true,
		},
		{
			name:   "a run at another head",
			stored: supportBaseline(func(r *run.Record) { r.Head = "0000000" }),
		},
		{
			name:   "a run that measured a probe's own code",
			stored: supportBaseline(probed),
		},
		{
			name:   "a run §5.1.6 found the sandbox unclean after",
			stored: supportBaseline(func(r *run.Record) { r.Contaminated = true }),
		},
		{
			name:   "a run carrying a filter, which §5.5 does not have a gap probe point at",
			stored: supportBaseline(func(r *run.Record) { r.Filter = "--group=probe" }),
		},
		{
			name:   "some other run of the same head",
			stored: supportBaseline(func(r *run.Record) { r.ID = "r9" }),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseline, resolved := gapRecord(resultFailed).
				ResolveBaseline([]run.Record{tc.stored})

			assert.Equal(t, tc.resolved, resolved,
				"§5.2.6 admits the run, or the probe has no baseline to be graded against")
			assert.Equal(t, tc.resolved, baseline.Passed(),
				"a baseline that did not resolve carries no verdict either")
		})
	}
}

// mappedPairs is the round's mapping.ndjson with §5.4.4's second condition met:
// the claim the fixtures name, joined to the unit they sit on.
func mappedPairs() []mapping.Pair {
	return []mapping.Pair{{
		Claim: supportClaim, Unit: supportUnit,
		Stamp: state.Stamp{Head: supportHead, Round: supportRound},
	}}
}

// gapRecord is the probe record the fixtures are about: one gap probe at the
// round's head, pointing at the unfiltered baseline §5.5 gives its kind.
func gapRecord(result Result) *Record {
	return &Record{
		ID:       "p1",
		Stamp:    state.Stamp{Head: supportHead, Round: supportRound},
		Kind:     Gap,
		Result:   result,
		Baseline: "r1",
		Target:   "src/Order.php:31",
	}
}

// supportBaseline is the run record gapRecord's `baseline` column names, with
// §5.2.5's verdict already true so the fence is what the tests vary.
func supportBaseline(mark ...func(*run.Record)) run.Record {
	record := baselineRun(supportHead, "")
	record.Passed = true
	for _, apply := range mark {
		apply(&record)
	}
	return record
}

// resolvedFor is the baseline a probe carries, built the only way one can be:
// out of the stored run record §5.5's `baseline` column names. Supports reads
// §5.2.5's verdict through an unexported field, so a caller cannot hand it a
// passing baseline the runs never recorded.
func resolvedFor(t *testing.T, record *Record, passed bool) Baseline {
	t.Helper()
	stored := supportBaseline(func(r *run.Record) { r.Passed = passed })
	baseline, resolved := record.ResolveBaseline([]run.Record{stored})
	require.True(t, resolved, "the fixture is a run record §5.2.6 admits")
	return baseline
}
