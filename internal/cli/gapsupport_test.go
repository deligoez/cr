package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
)

// The claim the fixture's intent axis produced, and the unit the fixture's
// mapping joins it to.
const (
	gapIssue = "CR-1"
	gapClaim = gapIssue + "#c2"
	gapUnit  = "u1"
)

// gapFixture is one state tree `cr record` reads §5.4 out of: a gap probe, the
// run record §5.5 has it point at, and the round's mapping.
//
// Every field is one of §5.4's own conditions, so a test varies exactly the
// condition it is about and the others stay as they were.
type gapFixture struct {
	// result is §5.5's `result` for the probe, as Decide wrote it.
	result probe.Result
	// head is the head the probe ran at, empty for the round's own.
	head string
	// passed is §5.2.5's verdict on the baseline run.
	passed bool
	// mapped says the round's mapping joins gapClaim to gapUnit.
	mapped bool
	// issue is the round's resolved issue key, empty for §4.5.3's
	// unavailable intent axis.
	issue string
}

// probedHome is recordedHome's pull request with the fixture's probe, baseline
// and mapping written into it.
//
// The records are marshalled from their own types rather than spelled as lines,
// which is the opposite of what unitLine does and for the opposite reason: the
// unit fixture proves a line holding two fields is read correctly, while these
// are read through the same structs cr writes them with, and a hand-spelled
// probe record would be a second declaration of §5.5's shape.
func probedHome(t *testing.T, f gapFixture) state.Layout {
	t.Helper()
	layout := recordedHome(t)
	head := f.head
	if head == "" {
		head = recordHead
	}
	stamp := state.Stamp{Head: recordHead, Round: recordRound}
	pairs := []mapping.Pair{}
	if f.mapped {
		pairs = append(pairs, mapping.Pair{Claim: gapClaim, Unit: gapUnit, Stamp: stamp})
	}

	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: recordOwner, Repo: recordRepo, PR: recordPRNum, IssueKey: f.issue,
		Round: recordRound, Head: recordHead,
	}))
	require.NoError(t, held.Write(state.FileRuns, ndjson(t, run.Record{
		ID: "r1", Stamp: stamp, Passed: f.passed, OutputTail: "OK",
	})))
	require.NoError(t, held.Write(state.FileProbes, ndjson(t, probe.Record{
		ID: "p1", Stamp: state.Stamp{Head: head, Round: recordRound},
		Kind: probe.Gap, Input: "it('cancels', ...)", Result: f.result,
		Target: "src/Order.php:31", Baseline: "r1",
		OutputTail: "FAILED  Tests\\OrderTest > cancels",
	})))
	require.NoError(t, held.Write(state.FileMapping, ndjson(t, pairs...)))
	require.NoError(t, held.Unlock())
	return layout
}

// ndjson renders records as the file cr writes, one JSON document per line.
func ndjson[T any](t *testing.T, records ...T) []byte {
	t.Helper()
	var body strings.Builder
	for _, record := range records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		body.Write(line)
		body.WriteByte('\n')
	}
	return []byte(body.String())
}

// aProbedRecord is aRecord resting on the fixture's gap probe, naming a claim.
// The two fields are the agent's own: §6.1 marks `claim` and `probe` optional,
// and neither is computed, which is what makes the claim condition of §5.4.4
// worth checking against a file the agent does not write.
func aProbedRecord(claim string) map[string]any {
	record := aRecord("f1", gapUnit)
	record["probe"] = "p1"
	if claim != "" {
		record["claim"] = claim
	}
	return record
}

// atMostMedium lowers a record's severity to the ceiling §5.4.5 puts on a
// record resting on a gap probe that supports no probed grade.
//
// The fixture record carries `high`, which is the severity of a finding an
// agent believes in — and that is the point of the bound: the belief is not
// evidence, and §5.4.5's results are not evidence either.
func atMostMedium(record map[string]any) map[string]any {
	record["severity"] = "medium"
	return record
}

// probeAnswer is the §5.4 answer `cr record` printed for one record, read back
// out of the JSON payload.
type probeAnswer struct {
	Record   string `json:"record"`
	Probe    string `json:"probe"`
	Result   string `json:"result"`
	Supports bool   `json:"supports"`
	Reason   string `json:"reason"`
}

// recordedAnswers runs `cr record` over one record and returns what §5.4 made of
// the probe it names, together with §4.5.4's disclosures.
func recordedAnswers(
	t *testing.T, f gapFixture, record map[string]any,
) (answers []probeAnswer, honesty []string) {
	t.Helper()
	_ = probedHome(t, f)
	file := writeRecordFile(t, "merged.ndjson", record)

	printed, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	var payload struct {
		Probes  []probeAnswer `json:"probes"`
		Honesty []string      `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &payload))
	return payload.Probes, payload.Honesty
}

// §5.4.4 through the command: a failed gap probe supports the record only when
// its baseline passed and its `claim` names a claim mapped to its unit.
//
// The rows are one condition each, and the mismatched claim is the one that
// carries the most. It is the row an agent reaches by writing a single field —
// `claim` is the agent's, the mapping is not — so the answer has to come from
// mapping.ndjson rather than from the record's own account of itself. A cr that
// read the record's word for it would grade an assertion to a colleague on the
// strength of the assertion.
//
// A supported answer is also held to what §5.4.4 states and no more. §5.4.4 is
// explicit that cr cannot distinguish a wrong behaviour from a wrong test, so a
// report saying the probe proved, verified or confirmed anything would be cr
// claiming the very thing the section says it cannot — round 12's
// unearned-probed-grade finding, in the one sentence a reader actually sees.
func TestAFailedGapProbeSupportsARecordOnlyOnABaselineAndAMappedClaim(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fixture  gapFixture
		claim    string
		supports bool
		reason   string
	}{
		{
			name:     "a passing baseline and a claim mapped to the record's unit",
			fixture:  gapFixture{result: "failed", passed: true, mapped: true, issue: gapIssue},
			claim:    gapClaim,
			supports: true,
			reason:   "§5.4.4",
		},
		{
			name:    "the same experiment on a suite that was already red",
			fixture: gapFixture{result: "failed", mapped: true, issue: gapIssue},
			claim:   gapClaim,
			reason:  "§5.2.5",
		},
		{
			name:    "a claim field the round's mapping does not join to the unit",
			fixture: gapFixture{result: "failed", passed: true, issue: gapIssue},
			claim:   gapClaim,
			reason:  "mapping.ndjson",
		},
		{
			name:    "a record naming no claim at all",
			fixture: gapFixture{result: "failed", passed: true, mapped: true, issue: gapIssue},
			reason:  "mapping.ndjson",
		},
		{
			name: "a probe that ran at another head",
			fixture: gapFixture{
				result: "failed", head: "1f2e3d4c5b6a79880997a6b5c4d3e2f11f2e3d4c",
				passed: true, mapped: true, issue: gapIssue,
			},
			claim:  gapClaim,
			reason: "§5.5.3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answers, _ := recordedAnswers(t, tc.fixture, aProbedRecord(tc.claim))

			require.Len(t, answers, 1, "the record names a gap probe, so §5.4 answers for it")
			assert.Equal(t, "f1", answers[0].Record)
			assert.Equal(t, "p1", answers[0].Probe)
			assert.Equal(t, tc.supports, answers[0].Supports,
				"§5.4.4: the baseline passed and the claim is mapped, or the probe supports nothing")
			assert.Contains(t, answers[0].Reason, tc.reason,
				"the reason names the condition that decided")

			for _, overclaimed := range []string{"prove", "verif", "confirm"} {
				assert.NotContains(t, answers[0].Reason, overclaimed,
					"§5.4.4 and §6.2.4: cr does not describe what it recorded as proven")
			}
		})
	}
}

// §5.4.5 through the command: each of the five results it names leaves the
// record with nothing to be graded `probed` on, so it stays argued under §6.2
// and is asked as a question under §6.3.
//
// Every row is given §5.4.4's two conditions in full — a baseline that passed
// and a claim mapped to the record's unit — so what refuses support is the
// result and nothing else. A record that rested on any of these and still
// reached `probed` would be an assertion to a colleague built on a run that
// timed out, crashed, selected nothing, printed no count cr could read, or
// showed the behaviour working.
//
// The `passed` row's reason is held to §5.4.5's own distinction as well: the
// behaviour is present, and that is not the missing test §5.3's
// `no-test-failed` establishes. It is the one result of the five that
// establishes anything, and reporting it as "nothing" would throw that away.
func TestEachResultSection545NamesLeavesTheRecordArgued(t *testing.T) {
	for _, result := range []probe.Result{
		"passed", "timeout", "error", "no-tests-selected", "inconclusive",
	} {
		t.Run(string(result), func(t *testing.T) {
			answers, _ := recordedAnswers(t, gapFixture{
				result: result, passed: true, mapped: true, issue: gapIssue,
			}, atMostMedium(aProbedRecord(gapClaim)))

			require.Len(t, answers, 1)
			assert.False(t, answers[0].Supports,
				"§5.4.5: this result supports no probed grade, whatever else is in place")
			assert.Contains(t, answers[0].Reason, "§5.4.5")
			assert.Contains(t, answers[0].Reason, "argued",
				"the reader is told where the record lands, not only what it lost")
		})
	}

	behaviour, _ := recordedAnswers(t, gapFixture{
		result: "passed", passed: true, mapped: true, issue: gapIssue,
	}, atMostMedium(aProbedRecord(gapClaim)))
	assert.Contains(t, behaviour[0].Reason, "no-test-failed",
		"§5.4.5: a passed gap probe is not the missing test §5.3 establishes")
}

// §4.5.4 and round 12's axis-availability-coupling: when the intent axis is
// unavailable there is no mapping to meet §5.4.4's second condition with, and
// the reader is told so.
//
// The failure this closes is a silent one. The experiment runs, the test fails,
// the record is stored, and it comes out `argued` — which reads to the agent as
// a judgement on its evidence, when in fact §4.5.3 marked the intent axis
// unavailable, §4.6.6 left the mapping empty, and the condition was unreachable
// before the probe started. §4.5.4 requires the lens that could not look to
// appear with its reason, and this is the report.
//
// The second half of the test is what keeps the disclosure worth reading: the
// same repository with a tracker says nothing, because there the mapping was
// asked and answered.
func TestTheUnavailableIntentAxisIsDisclosedWhenItIsWhatKeptAGapProbeOut(t *testing.T) {
	unavailable := gapFixture{result: "failed", passed: true}

	answers, honesty := recordedAnswers(t, unavailable, aProbedRecord(gapClaim))

	require.Len(t, answers, 1)
	assert.False(t, answers[0].Supports, "§5.4.4's second condition cannot be met")
	require.Len(t, honesty, 1, "§4.5.4 owes the reader the reason it could not be met")
	assert.Contains(t, honesty[0], "p1", "the disclosure names the experiment")
	assert.Contains(t, honesty[0], "§4.6.6")
	assert.Contains(t, honesty[0], "--issue", "and what would make the axis available")

	tracked := unavailable
	tracked.issue = gapIssue
	_, quiet := recordedAnswers(t, tracked, aProbedRecord(gapClaim))
	assert.Empty(t, quiet,
		"with an issue key the mapping was asked and answered, so no lens was blocked")
}

// §5.5.2 has a finding reference a probe rather than the other way round, so
// two records can rest on one experiment — and the disclosure names it once.
//
// Repetition here would be read as information. A reader shown "p1, p1" counts
// two experiments that went nowhere and starts looking for the second one,
// which is the sort of small wrongness that costs the report its standing.
func TestTheCouplingDisclosureNamesOneExperimentOnce(t *testing.T) {
	probedHome(t, gapFixture{result: "failed", passed: true})
	second := aProbedRecord(gapClaim)
	second["id"] = "f2"
	file := writeRecordFile(t, "merged.ndjson", aProbedRecord(gapClaim), second)

	printed, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	var payload struct {
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &payload))
	require.Len(t, payload.Honesty, 1, "one coupling, one disclosure")
	assert.Equal(t, 1, strings.Count(payload.Honesty[0], "p1"),
		"the probe both records rest on is named once")
}

// §5.4.4's floor and §5.4.5's ceiling through the command: `cr record` is the
// enforcer, it exits 1, and it names the record, the probe and the result.
//
// Round 8's unenforced-severity-bound finding is what this closes. Severity is
// agent-written and §6.1.3 checks its presence alone, so until a command
// refuses on them both MUSTs bind nobody — and the cost is a misdirected human:
// severity is what the reviewer triages on under §1.6.2's blocking cap, so a
// probe that showed the behaviour working could put its record at the top of
// the draft, and a probe that supports a real finding could hide it at the
// bottom.
//
// Nothing is stored either. The refusal happens before the write, as §6.1.3's
// do, because the agent is about to correct the file and hand the whole of it
// in again.
func TestRecordRefusesASeverityEitherGapBoundForbids(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fixture  gapFixture
		severity string
		bound    string
	}{
		{
			name:     "a supported failed gap probe below §5.4.4's floor",
			fixture:  gapFixture{result: "failed", passed: true, mapped: true, issue: gapIssue},
			severity: "medium",
			bound:    "§5.4.4",
		},
		{
			name:     "a passed gap probe above §5.4.5's ceiling",
			fixture:  gapFixture{result: "passed", passed: true, mapped: true, issue: gapIssue},
			severity: "critical",
			bound:    "§5.4.5",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout := probedHome(t, tc.fixture)
			record := aProbedRecord(gapClaim)
			record["severity"] = tc.severity
			file := writeRecordFile(t, "merged.ndjson", record)

			_, err := runRecord(t, recordPR, file, "--repo", recordSlug)

			require.Error(t, err)
			assert.Equal(t, ExitValidation, exitCodeFor(err), "§11.2 codes a refused record 1")
			var refused *finding.GapSeverityError
			require.ErrorAs(t, err, &refused, "cr record raises no refusal of its own")
			assert.Equal(t, "f1", refused.Record)
			assert.Equal(t, "p1", refused.Probe)
			assert.Equal(t, string(tc.fixture.result), refused.Result)
			assert.Contains(t, err.Error(), tc.bound, "the refusal names the section it comes from")

			stored, readErr := state.ReadRecords[finding.Finding](
				layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
			require.NoError(t, readErr)
			assert.Empty(t, stored, "the refusal lands before anything is written")
		})
	}
}

// §12.1's other shape: the terminal reader is told the same answer the JSON
// carries, in the same words, and the disclosure reaches them too.
func TestATerminalRecordNamesWhatTheGapProbeSupports(t *testing.T) {
	probedHome(t, gapFixture{result: "failed", passed: true})
	file := writeRecordFile(t, "merged.ndjson", aProbedRecord(gapClaim))

	out := throughATerminal(t, "record", recordPR, file, "--repo", recordSlug)

	assert.Contains(t, out, "supports no probed grade")
	assert.Contains(t, out, "gap probe support unavailable",
		"§11.1 exempts the disclosure from every flag, so a terminal gets it as well")
}
