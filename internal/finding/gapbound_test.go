package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/probe"
)

// §5.4.4's floor and §5.4.5's ceiling over every severity §6.1 names, on every
// result §5.4.3 can produce.
//
// The two bounds point opposite ways and apply in different places, so the
// table drives both directions of each. A supported failed gap probe carries
// severity at least `high` — nothing in §5.4.4 lowers that once it applies —
// and a record resting on any of §5.4.5's five carries at most `medium`.
//
// The rows that carry the most are the ones with no bound at all. An
// unsupported `failed` probe has neither: §5.4.4's floor is written "only when
// the probe supports the finding" and §5.4.5 does not name `failed`, so the
// severity is the agent's and §6.2 leaves the record argued anyway. Inventing a
// bound there would be cr forming an opinion §5.4 does not state.
//
// `error` is in the table for round 9's reason. A runner killed by a signal is
// `error` on §5.4.3's second rung, so a crashed process cannot reach §5.4.4's
// floor and is capped at `medium` instead — which is the whole point of that
// rung existing.
func TestBothGapSeverityBoundsAreEnforced(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   probe.Result
		supports bool
		severity Severity
		refused  bool
	}{
		{name: "supported failed at critical", result: "failed", supports: true, severity: SeverityCritical},
		{name: "supported failed at high", result: "failed", supports: true, severity: SeverityHigh},
		{
			name: "supported failed at medium", result: "failed", supports: true,
			severity: SeverityMedium, refused: true,
		},
		{
			name: "supported failed at low", result: "failed", supports: true,
			severity: SeverityLow, refused: true,
		},
		{name: "unsupported failed at critical", result: "failed", severity: SeverityCritical},
		{name: "unsupported failed at low", result: "failed", severity: SeverityLow},
		{name: "passed at medium", result: "passed", severity: SeverityMedium},
		{name: "passed at low", result: "passed", severity: SeverityLow},
		{name: "passed at high", result: "passed", severity: SeverityHigh, refused: true},
		{name: "passed at critical", result: "passed", severity: SeverityCritical, refused: true},
		{name: "timeout at high", result: "timeout", severity: SeverityHigh, refused: true},
		{name: "error at high", result: "error", severity: SeverityHigh, refused: true},
		{
			name: "no tests selected at critical", result: "no-tests-selected",
			severity: SeverityCritical, refused: true,
		},
		{name: "inconclusive at high", result: "inconclusive", severity: SeverityHigh, refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := &Finding{ID: "f1", Severity: tc.severity}
			gap := &probe.Record{ID: "p1", Kind: probe.Gap, Result: tc.result}

			err := CheckGapSeverity(ActorRecord, record, gap, tc.supports)

			if !tc.refused {
				assert.NoError(t, err, "§5.4 leaves this severity where the agent wrote it")
				return
			}
			var refused *GapSeverityError
			require.ErrorAs(t, err, &refused, "§5.4's bounds are refused through one type")
			assert.Equal(t, "f1", refused.Record, "the refusal names the record")
			assert.Equal(t, "p1", refused.Probe, "and the probe it rests on")
			assert.Equal(t, string(tc.result), refused.Result, "and the result the bound was read from")
			assert.Contains(t, refused.Error(), "f1")
			assert.Contains(t, refused.Error(), "p1")
			assert.Contains(t, refused.Error(), string(tc.result))
			// And what to write instead. A refusal that named only
			// what is wrong leaves the agent to work the bound out
			// of the section, which is the step a message exists to
			// save — and §12.4 has every error name the next step.
			assert.Contains(t, refused.Error(), allowedBy[tc.supports],
				"the refusal names the severities the bound leaves")
		})
	}
}

// allowedBy is what each bound leaves the record, said the way the refusal says
// it: §5.4.4's floor when the probe supports the finding, §5.4.5's ceiling when
// it does not.
var allowedBy = map[bool]string{
	true:  "high or critical",
	false: "medium or low",
}

// §5.4's bounds bind a record resting on a gap probe, and nothing else.
//
// A record with no probe that grades this round reaches the same function and
// leaves it untouched — which is what lets `cr record` call the check over
// every record it stores rather than over a subset it worked out first, and
// keeps §5.4 from reaching records whose evidence is §6.2's business.
func TestNoGapProbeMeansNoSeverityBound(t *testing.T) {
	loudest := &Finding{ID: "f1", Severity: SeverityCritical}

	assert.NoError(t, CheckGapSeverity(ActorRecord, loudest, nil, false))
	assert.NoError(t, CheckGapSeverity(ActorPostConfirm, loudest, nil, false))
}

// A severity §6.1 does not name satisfies neither bound.
//
// §6.1.3 checks the field's presence alone, so an unrecognised value does reach
// here — and the safe answer is the one that refuses. A bound implemented as a
// comparison would have to rank the unknown value into one half or the other,
// and whichever half it chose would let some record through a MUST on the
// strength of a word cr does not understand.
func TestAnUnrecognisedSeverityMeetsNeitherBound(t *testing.T) {
	record := &Finding{ID: "f1", Severity: Severity("blocker")}

	assert.Error(t, CheckGapSeverity(
		ActorRecord, record, &probe.Record{ID: "p1", Kind: probe.Gap, Result: "failed"}, true),
		"§5.4.4's floor is not met by a severity cr cannot place")
	assert.Error(t, CheckGapSeverity(
		ActorRecord, record, &probe.Record{ID: "p1", Kind: probe.Gap, Result: "passed"}, false),
		"and neither is §5.4.5's ceiling")
}

// The refusal names the command that gave it, whichever of §9.1's actors asks.
//
// §7.2.2's shape has the bound asked at record time and again after triage, by
// one function taking the moment as an argument, so the two answers differ in
// the moment alone. That both commands actually ask is internal/cli's
// TestTheSameBoundIsCheckedAtRecordTimeAndAtPostTime, which drives them; this is
// the half a caller cannot observe without a refusal to read.
func TestTheRefusalNamesTheMomentItCameFrom(t *testing.T) {
	record := &Finding{ID: "f1", Severity: SeverityCritical}
	gap := &probe.Record{ID: "p1", Kind: probe.Gap, Result: "passed"}

	for _, by := range []Actor{ActorRecord, ActorPostConfirm, ActorPostReconcile} {
		t.Run(by.String(), func(t *testing.T) {
			err := CheckGapSeverity(by, record, gap, false)

			var refused *GapSeverityError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, by, refused.By, "the refusal names the moment it came from")
			assert.Contains(t, refused.Error(), by.String())
		})
	}
}
