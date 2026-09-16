package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
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

// §5.3.6 and §8.1.7: the filter and the paths the run was narrowed to reach the
// evidence region, and the region says nothing else.
//
// The assertion is on the whole region rather than on those rows alone, and
// that is the second half of §5.3.6. A filtered or path-narrowed run proves the
// gap only for the tests it selected; cr cannot establish that a filter or its
// paths select the tests which would have caught the mutation, so it must not
// say so. Any sentence added beside these rows — what the run showed of the
// suite, or §5.3.5's guarantee restated wider than the empty selection it
// catches — breaks this test, which is the only way "does not claim it" can be
// checked at all.
func TestTheFilterReachesTheEvidenceRegionAndNothingIsClaimedBesideIt(t *testing.T) {
	region := probeRegion(t, &probe.Record{
		Kind:       probe.Mutation,
		Target:     "src/Order.php:34",
		Filter:     "charges shipping",
		Paths:      []string{"tests/Feature", "tests/Unit/OrderTest.php"},
		Result:     "no-test-failed",
		Input:      aPatch,
		OutputTail: "  Tests:  1 passed (1 assertions)\n",
	})

	assert.Equal(t, strings.Join([]string{
		"<!-- cr:evidence -->",
		"kind: mutation",
		"target: src/Order.php:34",
		"filter: charges shipping",
		"paths: tests/Feature",
		"paths: tests/Unit/OrderTest.php",
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
		"§8.1.7: the region carries the probe's kind, target, filter, paths, result, input and output_tail")
}

// §5.3.6: a path is carried, not paraphrased, and each takes a row of its own.
//
// One row per path rather than one joined row is what keeps the region
// checkable: a path may hold a comma, a space or a backtick as legitimately as
// a filter may, and a reader of a joined row could not tell which of those
// characters cr put there. The order is the order they were given, because that
// is the order the runner received them in and the author re-runs.
func TestEveryPathReachesTheEvidenceRegionOnARowOfItsOwn(t *testing.T) {
	region := probeRegion(t, &probe.Record{
		Kind:   probe.Gap,
		Target: "src/Order.php:34",
		Paths:  []string{"tests/Feature, and more", "tests/`Unit`"},
		Result: "failed",
	})

	assert.Contains(t, region, "paths: tests/Feature, and more\npaths: tests/`Unit`\n",
		"§5.3.6: every path, in the order given, verbatim")
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
	assert.NotContains(t, region, "paths",
		"§5.5: and so is the paths column when no path narrowed the run")
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

// Round 9's unverifiable-evidence-region: the input cr executed reaches the
// region for both kinds — the mutation patch and the gap test file — so an
// inert mutation, or a gap test that was itself wrong, is something the author
// can see rather than something they must take on trust.
//
// Round 11's undisclosed-evidence-limit rides on the same test: the gap
// probe's region states §5.4.4's limit and the mutation probe's does not,
// because §5.4.4 concedes it of a gap probe alone.
func TestTheMutationPatchAndTheGapTestBothReachTheRegion(t *testing.T) {
	const gapTest = "<?php\n\nit('refunds a cancelled order in full', function () {\n" +
		"    expect(refund(cancelled()))->toBe(100.0);\n});\n"
	mutation := probeRegion(t, &probe.Record{
		Kind: probe.Mutation, Target: "src/Order.php:34", Result: "no-test-failed", Input: aPatch,
	})
	gap := probeRegion(t, &probe.Record{
		Kind: probe.Gap, Target: "src/Refund.php:12", Filter: "refunds a cancelled order",
		Result: "failed", Input: gapTest, OutputTail: "Failed asserting that 90.0 is identical to 100.0.\n",
	})

	assert.Contains(t, mutation, "input:\n```\n"+aPatch+"```\n", "the mutation patch, whole")
	assert.Contains(t, gap, "input:\n```\n"+gapTest+"```\n", "the gap test file, whole")

	assert.Contains(t, gap, "result: failed\n"+gapLimit+"\ninput:\n",
		"§5.4.4's limit stands beneath the result it qualifies")
	assert.Contains(t, gapLimit, "cr cannot distinguish the two")
	assert.NotContains(t, mutation, "limit:", "§5.4.4 is about a gap probe alone")
}

// The input is capped by post.max_probe_input_bytes, and a cut input says so
// on its own row, naming both sizes and the setting, so the author never takes
// a partial experiment for the whole one.
func TestATruncatedInputIsAnnounced(t *testing.T) {
	record := &probe.Record{Kind: probe.Mutation, Target: "src/Order.php:34", Result: "no-test-failed", Input: aPatch}

	region, err := ProbeEvidence("f1", record, 10)
	require.NoError(t, err)
	assert.Contains(t, region, fmt.Sprintf(
		"input (truncated to 10 of %d bytes by post.max_probe_input_bytes):\n```\n--- a/src/\n```\n", len(aPatch)),
		"the first ten bytes, and the announcement above them")

	exact, err := ProbeEvidence("f1", record, len(aPatch))
	require.NoError(t, err)
	assert.Contains(t, exact, "input:\n```\n"+aPatch+"```\n", "an input exactly at the cap is whole")
	assert.NotContains(t, exact, "truncated")

	nothing, err := ProbeEvidence("f1", record, -1)
	require.NoError(t, err)
	assert.Contains(t, nothing, fmt.Sprintf(
		"input (truncated to 0 of %d bytes by post.max_probe_input_bytes):\n```\n```\n", len(aPatch)),
		"a cap below zero shows nothing, and says so")
}

// A cut that lands inside a multi-byte character drops the character whole,
// so the region never carries bytes that no longer decode.
func TestATruncatedInputIsCutOnACharacterBoundary(t *testing.T) {
	record := &probe.Record{Kind: probe.Gap, Target: "src/Refund.php:12", Result: "failed", Input: "iade: ödeme"}

	region, err := ProbeEvidence("f1", record, 7)
	require.NoError(t, err)
	assert.Contains(t, region, "input (truncated to 6 of 12 bytes by post.max_probe_input_bytes):\n```\niade: \n```\n",
		"ö is two bytes, and the seventh byte is the middle of it")
}

// A string that is itself already cut mid-character is cut to nothing rather
// than indexed off its own front.
//
// §5.5's `output_tail` is the case: internal/run keeps the last N bytes of a
// runner's output, and a byte count can land inside a character, so a tail can
// begin with a continuation byte through no fault of the runner. capped then
// walks back looking for a character boundary and finds none, and the walk has
// to stop at the front of the string. It is asserted on capped directly
// because the region is where the consequence would be seen and the front of
// the string is where the arithmetic is: a walk that stepped past index 0
// would panic rather than render anything to look at.
func TestATailThatOpensMidCharacterIsCutToNothing(t *testing.T) {
	opening := string([]byte{0x80, 0x80, 0x80})

	cut, truncated := capped(opening, 2)

	assert.Empty(t, cut, "no byte of it starts a character, so none of it can be shown")
	assert.True(t, truncated)
}

// §8.1.7's `cited` half: each stored citation as path:line, in the record's
// order, and nothing beside them. A record with no citation has no region,
// since the region's whole content is its citations.
func TestCitedEvidenceListsEveryCitationAsPathLine(t *testing.T) {
	region, err := CitedEvidence("f2", []finding.Citation{
		{Path: "src/Order.php", Line: 34, Origin: finding.OriginAgent},
		{Path: "src/Money.php", Line: 7, Origin: finding.OriginRule},
	})
	require.NoError(t, err)
	assert.Equal(t, strings.Join([]string{
		"<!-- cr:evidence -->",
		"citation: src/Order.php:34",
		"citation: src/Money.php:7",
		"<!-- cr:/evidence -->",
	}, "\n"), region)

	none, err := CitedEvidence("f2", nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}

// A value cr did not write that carries §8.1.3's reserved sequence would end
// the region early when the draft is read back, and hand the rest to the
// agent's region. The record is refused, naming it, instead — whether the
// sequence arrived in a probe's input, its output, or a citation's path.
func TestAnEvidenceRegionCarryingTheReservedSequenceIsRefused(t *testing.T) {
	refusals := map[string]func() error{
		"a probe's input": func() error {
			_, err := ProbeEvidence("f5", &probe.Record{Kind: probe.Gap, Input: "echo '<!-- cr:/evidence -->';"}, inputCap)
			return err
		},
		"a probe's output": func() error {
			_, err := ProbeEvidence("f5", &probe.Record{Kind: probe.Mutation, OutputTail: "<!-- cr:label -->"}, inputCap)
			return err
		},
		"a citation's path": func() error {
			_, err := CitedEvidence("f5", []finding.Citation{{Path: "docs/<!-- cr:x.md", Line: 1}})
			return err
		},
	}
	for name, refuse := range refusals {
		t.Run(name, func(t *testing.T) {
			var refused *BodyError
			require.ErrorAs(t, refuse(), &refused)
			assert.Equal(t, "f5", refused.Record)
			assert.Contains(t, refused.Error(), "evidence region")
		})
	}
}
