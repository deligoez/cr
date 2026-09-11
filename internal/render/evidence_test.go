package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/probe"
)

// inputCap is post.max_probe_input_bytes' default, the cap every region here
// is rendered under unless a test is about the cap itself.
const inputCap = 4096

// probeRegion is ProbeEvidence for record f1 under inputCap, for a test whose
// probe §8.1.3 accepts.
func probeRegion(t *testing.T, p *probe.Record) string {
	t.Helper()
	region, err := ProbeEvidence("f1", p, inputCap)
	require.NoError(t, err)
	return region
}

// aPatch is a mutation probe's input: the one-hunk patch §5.3.1 has cr apply.
const aPatch = "--- a/src/Order.php\n+++ b/src/Order.php\n@@ -34 +34 @@\n" +
	"-        return $this->subtotal + $this->shipping;\n" +
	"+        return $this->subtotal;\n"

// §5.3.6 and §8.1.7: the filter the run was narrowed to reaches the evidence
// region, and the region says nothing else.
//
// The assertion is on the whole region rather than on the filter row alone, and
// that is the second half of §5.3.6. A filtered run proves the gap only for the
// tests it selected; cr cannot establish that a filter selects the tests which
// would have caught the mutation, so it must not say so. Any sentence added
// beside these rows — what the run showed of the suite, or §5.3.5's guarantee
// restated wider than the empty selection it catches — breaks this test, which
// is the only way "does not claim it" can be checked at all.
func TestTheFilterReachesTheEvidenceRegionAndNothingIsClaimedBesideIt(t *testing.T) {
	region := probeRegion(t, &probe.Record{
		Kind:       probe.Mutation,
		Target:     "src/Order.php:34",
		Filter:     "charges shipping",
		Result:     "no-test-failed",
		Input:      aPatch,
		OutputTail: "  Tests:  1 passed (1 assertions)\n",
	})

	assert.Equal(t, strings.Join([]string{
		"<!-- cr:evidence -->",
		"kind: mutation",
		"target: src/Order.php:34",
		"filter: charges shipping",
		"result: no-test-failed",
		"input:",
		"```",
		strings.TrimSuffix(aPatch, "\n"),
		"```",
		"output_tail:",
		"```",
		"  Tests:  1 passed (1 assertions)",
		"```",
		"<!-- cr:/evidence -->",
	}, "\n"), region,
		"§8.1.7: the region carries the probe's kind, target, filter, result, input and output_tail")
}

// §5.3.6: the filter is carried, not paraphrased.
//
// The expressions here are the ones a real filter is made of — a runner's own
// quoting, an alternation, a backtick — and each has to survive into the region
// byte for byte, because the author checks the comment by re-running the filter
// cr names. A filter cr tidied would be a filter the author cannot re-run.
func TestAFilterReachesTheRegionVerbatim(t *testing.T) {
	for _, filter := range []string{
		"charges shipping",
		"Order::shipping",
		"shipping|totals",
		"it('charges shipping', ...)",
		"tests/OrderTest.php::*",
		"`shipping`",
		"  leading and trailing  ",
	} {
		t.Run(filter, func(t *testing.T) {
			region := probeRegion(t, &probe.Record{
				Kind:   probe.Mutation,
				Target: "src/Order.php:34",
				Filter: filter,
				Result: "no-test-failed",
			})
			assert.Contains(t, region, "filter: "+filter+"\n",
				"§5.3.6: the filter the run was narrowed to, as it was given")
		})
	}
}

// §5.5 makes `filter` optional and the record omits it for a run of the whole
// suite, so the region has no row for one.
//
// The absence is asserted together with the silence around it: the alternative
// to an omitted row is a value cr invented to stand for "no filter", and the
// evidence region is the one place in a posted comment where every word is
// supposed to be checkable against the record.
func TestAnUnfilteredRunCarriesNoFilterRow(t *testing.T) {
	region := probeRegion(t, &probe.Record{
		Kind:       probe.Mutation,
		Target:     "src/Order.php:34",
		Result:     "no-test-failed",
		Input:      aPatch,
		OutputTail: "  Tests:  4 passed\n",
	})

	assert.NotContains(t, region, "filter",
		"§5.5: the column is absent when the whole suite ran")
	assert.Equal(t, strings.Join([]string{
		"<!-- cr:evidence -->",
		"kind: mutation",
		"target: src/Order.php:34",
		"result: no-test-failed",
		"input:",
		"```",
		strings.TrimSuffix(aPatch, "\n"),
		"```",
		"output_tail:",
		"```",
		"  Tests:  4 passed",
		"```",
		"<!-- cr:/evidence -->",
	}, "\n"), region)
}

// The runner's output is whatever the runner printed, and a suite that printed
// a code fence would otherwise close the block early — leaving the rest of its
// own output to be read as the comment's prose, which in this region means read
// as something cr asserted.
func TestAnOutputTailHoldingAFenceIsStillContained(t *testing.T) {
	region := probeRegion(t, &probe.Record{
		Kind:       probe.Mutation,
		Target:     "src/Order.php:34",
		Result:     "failed",
		OutputTail: "expected:\n```\n0.0\n```\nbut got 9.99",
	})

	assert.Contains(t, region, "````\nexpected:\n```\n0.0\n```\nbut got 9.99\n````\n",
		"the fence is one backtick longer than the longest run the tail holds")
	assert.True(t, strings.HasSuffix(region, evidenceClose),
		"§8.1.3: the region ends at its own closing marker")
}
