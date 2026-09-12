package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// seedTriage raises one class `raised` times on one pull request and settles
// each raise with the outcome its position names, so a fixture is written as
// the distribution it is meant to be rather than as a loop of calls.
//
// Every record gets its own round, which is what makes the ledger hold them
// all: §7.3.1 keys an event by `(record, pr, round)` and a second write under
// one key overwrites rather than appends.
func seedTriage(
	t *testing.T, l state.Layout, pr int, class string, outcomes []finding.Outcome,
) {
	t.Helper()
	for i, outcome := range outcomes {
		record := aTriagedRecord("f"+strconv.Itoa(i), class, "")
		seedRaised(t, l, pr, i+1, record)
		seedOutcome(t, l, pr, i+1, record, outcome)
	}
}

// repeated is one outcome, n times over, so a distribution reads as the
// sentence §7.3.4 states rather than as a literal of twelve entries.
func repeated(n int, outcome finding.Outcome) []finding.Outcome {
	out := make([]finding.Outcome, 0, n)
	for range n {
		out = append(out, outcome)
	}
	return out
}

// §7.3.4's numerator excludes `not-here`, and the exclusion is what decides
// the candidacy rather than a rounding away from it.
//
// Two classes are raised ten times each, past §7.3.4's default minimum of
// eight. `imprecise` is discarded `wrong` seven times, a rate of 0.7 over the
// default threshold of 0.6, and is a candidate. `unwelcome` is discarded
// `not-here` seven times and kept three, which is the same shape of history
// and the opposite fact about the class: §7.2 makes `not-here` a true finding
// not worth saying here, so its rate is 0 and it is no candidate at all.
//
// That pairing is the assertion. A numerator that simply counted discards
// would put both classes on the list at 0.7, and the second one is a class
// that has never once been wrong.
func TestTheDemotionRateExcludesNotHereFromItsNumerator(t *testing.T) {
	layout := statsHome(t)

	wrong := append(repeated(7, finding.OutcomeDiscardedWrong), repeated(3, finding.OutcomeKept)...)
	seedTriage(t, layout, statsFirst, "imprecise", wrong)
	notHere := append(
		repeated(7, finding.OutcomeDiscardedNotHere), repeated(3, finding.OutcomeKept)...)
	seedTriage(t, layout, statsLater, "unwelcome", notHere)

	report := statsReport(t)

	assert.Equal(t, 0.6, report.Threshold, "§7.3.4's default threshold")
	assert.Equal(t, 8, report.MinSamples, "§7.3.4's default minimum")
	assert.Equal(t, []finding.DemotionCandidate{
		{Class: "imprecise", Raised: 10, RateLowerBound: 0.7},
	}, report.Demotion,
		"§7.3.4: `wrong` and `softened` count, and `not-here` never does")
}

// A softened record counts against its class exactly as a `wrong` one does,
// and the two together are what carry a class over the threshold.
//
// §7.3.4 names both in the numerator, and a class softened every other time is
// a class whose findings cr could not stand behind — which is the same signal a
// false positive gives, arrived at more politely.
func TestSofteningCountsTowardTheDemotionRateBesideWrong(t *testing.T) {
	layout := statsHome(t)

	mixed := append(repeated(4, finding.OutcomeDiscardedWrong), repeated(3, finding.OutcomeSoftened)...)
	mixed = append(mixed, repeated(3, finding.OutcomeKept)...)
	seedTriage(t, layout, statsFirst, "overreaching", mixed)

	report := statsReport(t)
	require.Len(t, report.Demotion, 1)
	assert.Equal(t, finding.DemotionCandidate{
		Class: "overreaching", Raised: 10, RateLowerBound: 0.7,
	}, report.Demotion[0], "§7.3.4: (4 wrong + 3 softened) / 10 raised")
}

// A class under §7.3.4's minimum is not a candidate however bad its rate, and
// a class on the threshold is not one either.
//
// The minimum is what stops a class raised twice and wrong twice from being
// demoted on a sample of two, which is the reading that would make the whole
// statistic worse than nothing. The threshold is read strictly because §7.3.4
// words the two bounds differently — "exceeds" the threshold, "at least" the
// minimum — and a class sitting exactly on 0.6 is the case where the two
// readings differ.
func TestAThinSampleAndAnExactThresholdAreNotCandidates(t *testing.T) {
	layout := statsHome(t)

	seedTriage(t, layout, statsFirst, "rare", repeated(7, finding.OutcomeDiscardedWrong))
	onTheLine := append(repeated(6, finding.OutcomeDiscardedWrong), repeated(4, finding.OutcomeKept)...)
	seedTriage(t, layout, statsLater, "borderline", onTheLine)

	report := statsReport(t)
	assert.Empty(t, report.Demotion,
		"§7.3.4: 7 raises is under the minimum, and 0.6 does not exceed 0.6")

	byClass := map[string]finding.ClassTriage{}
	for _, class := range report.Classes {
		byClass[class.Class] = class
	}
	assert.InDelta(t, 1.0, byClass["rare"].DemotionRate(), 0.0001,
		"the thin class's rate is computed and simply not acted on")
	assert.InDelta(t, 0.6, byClass["borderline"].DemotionRate(), 0.0001)
}

// §7.3.5: the rate is reported as a lower bound and never as a measured
// precision.
//
// It is asserted on the document rather than on the terminal rendering alone,
// because an agent reads the document and §7.3.5 binds what the number may be
// presented as to any reader. The key carries the caveat and so does the
// sentence beside it, which is deliberate: a reader who copies one number out
// of the document still cannot lose it.
func TestTheDemotionRateIsReportedAsALowerBoundAndNotAsPrecision(t *testing.T) {
	layout := statsHome(t)
	wrong := append(repeated(7, finding.OutcomeDiscardedWrong), repeated(3, finding.OutcomeKept)...)
	seedTriage(t, layout, statsFirst, "imprecise", wrong)

	printed, err := runCLIPrinting(t, "stats", "--repo", statsSlug)
	require.NoError(t, err)
	assert.Contains(t, printed, `"rate_lower_bound": 0.7`,
		"§7.3.5: the rate's own key says what the number is")
	assert.Contains(t, printed, "lower bound on imprecision, not a measured precision",
		"§7.3.5: and the document carries the sentence too")
	assert.NotContains(t, strings.ToLower(printed), `"precision"`,
		"§7.3.5: no field offers the number as a measured precision")

	var report statsResult
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	assert.Equal(t, finding.DemotionBound, report.Bound)
}

// The threshold and the minimum are §2.7 settings rather than constants, and a
// repository that sets them is the one the candidacy is decided by.
//
// A project whose reviewers mark false positives diligently and one where the
// `wrong` marker is rarely used are §7.3.5's two ends of the same spectrum, and
// the second is the ordinary case — so the numbers have to move.
func TestTheSampleComesFromTheResolvedConfiguration(t *testing.T) {
	layout := statsHome(t)
	wrong := append(repeated(3, finding.OutcomeDiscardedWrong), repeated(2, finding.OutcomeKept)...)
	seedTriage(t, layout, statsFirst, "imprecise", wrong)

	assert.Empty(t, statsReport(t).Demotion, "5 raises is under the default minimum of 8")

	require.NoError(t, os.WriteFile(
		filepath.Join(layout.RepoDir(statsOwner, statsRepo), "config.json"),
		[]byte(`{"stats.min_samples": 4, "stats.demote_threshold": 0.5}`+"\n"), 0o600))

	report := statsReport(t)
	assert.Equal(t, 0.5, report.Threshold)
	assert.Equal(t, 4, report.MinSamples)
	require.Len(t, report.Demotion, 1)
	assert.Equal(t, "imprecise", report.Demotion[0].Class)
}
