package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/probe"
)

// aFullComment is a comment carrying every region §8.1.3 names, each owned
// one built the way its constructor builds it.
func aFullComment(t *testing.T) Comment {
	t.Helper()
	return Comment{
		Label:      ownedRegions[0].wrap("**Question** — evidence grade: argued"),
		Provenance: ownedRegions[1].wrap("Rests on note n3, source: the author."),
		Body:       "Does Decode's error reach the caller?\n\nThe second result is discarded.",
		Evidence: probeRegion(t, &probe.Record{
			Kind: probe.Mutation, Target: "internal/api/handler.go:42",
			Result: "no-test-failed", OutputTail: "ok  \tgithub.com/acme/api\t0.4s\n",
		}),
	}
}

// §8.1.3: a posted comment is a fixed sequence of regions — the label, the
// provenance block when one applies, the agent body, and the evidence block
// when one applies — and a region that does not apply leaves no trace.
func TestACommentIsTheFixedSequenceOfRegions(t *testing.T) {
	full := aFullComment(t)
	assert.Equal(t,
		full.Label+"\n\n"+full.Provenance+"\n\n"+full.Body+"\n\n"+full.Evidence,
		full.String(), "§8.1.3's order, a blank line between each two regions")

	bodyOnly := Comment{Body: full.Body}
	assert.Equal(t, full.Body, bodyOnly.String(),
		"a comment no owned region applies to is the agent body and nothing else")

	noProvenance := full
	noProvenance.Provenance = ""
	assert.Equal(t, full.Label+"\n\n"+full.Body+"\n\n"+full.Evidence, noProvenance.String(),
		"an absent region leaves no separator behind")
}

// §8.1.3: each cr-owned region is delimited by its own named pair, spelled as
// the section spells it, so recovery never depends on counting.
func TestEveryOwnedRegionIsDelimitedByItsOwnNamedPair(t *testing.T) {
	assert.Equal(t, []ownedRegion{
		{open: "<!-- cr:label -->", close: "<!-- cr:/label -->"},
		{open: "<!-- cr:provenance -->", close: "<!-- cr:/provenance -->"},
		{open: "<!-- cr:evidence -->", close: "<!-- cr:/evidence -->"},
	}, ownedRegions, "§8.1.3's three pairs, in the order the sequence places them")

	seen := make(map[string]bool, 2*len(ownedRegions))
	for _, region := range ownedRegions {
		for _, marker := range []string{region.open, region.close} {
			assert.True(t, strings.HasPrefix(marker, Reserved),
				"%s opens with the sequence §8.1.3 keeps out of a body", marker)
			assert.False(t, seen[marker], "%s delimits one region only", marker)
			seen[marker] = true
		}
		assert.Equal(t, region.open+"\nx\n"+region.close, region.wrap("x"),
			"each marker stands on a line of its own")
	}
}

// Every combination of owned regions round-trips: a comment nobody edited
// recovers exactly the agent body it was built from. That is what lets
// §7.1.5's `rendered.json` tell an edited body from an untouched one.
func TestAnUneditedCommentRecoversItsBody(t *testing.T) {
	full := aFullComment(t)
	for mask := range 8 {
		comment := Comment{Body: full.Body}
		if mask&1 != 0 {
			comment.Label = full.Label
		}
		if mask&2 != 0 {
			comment.Provenance = full.Provenance
		}
		if mask&4 != 0 {
			comment.Evidence = full.Evidence
		}
		assert.Equal(t, full.Body, AgentRegion(comment.String()), "regions present: %03b", mask)
	}
}

// §8.1.3: cr regenerates every owned region and discards edits inside them,
// and §8.1.5, §7.1.5 and §7.1.6 apply to the agent region alone.
//
// The same edit is made in two places — inside each owned region and inside
// the agent region — so the test can only pass by treating the two places
// differently: an owned region is taken out whole and regenerated from the
// record, and the agent region is what is left. A recovery that dropped every
// edit, or kept every edit, fails one half.
func TestAnEditInsideAnOwnedRegionIsDiscardedWhileTheSameEditInTheAgentRegionSurvives(t *testing.T) {
	const edit = "EDITED BY THE REVIEWER"
	regenerated := aFullComment(t)
	written := regenerated.String()

	edited := written
	for _, region := range ownedRegions {
		edited = strings.Replace(edited, region.open+"\n", region.open+"\n"+edit+"\n", 1)
	}
	edited = strings.Replace(edited, regenerated.Body, regenerated.Body+"\n"+edit, 1)
	require.Equal(t, len(ownedRegions)+1, strings.Count(edited, edit),
		"the edit reached every owned region and the agent region")

	regenerated.Body = AgentRegion(edited)
	posted := regenerated.String()

	assert.Equal(t, 1, strings.Count(posted, edit), "only the agent region's copy survives")
	assert.Contains(t, regenerated.Body, edit, "the edit in the agent region survives")
	assert.Equal(t, strings.Replace(written, aFullComment(t).Body, regenerated.Body, 1), posted,
		"every owned region is exactly as cr generated it")
}

// A region is found by its pair wherever it sits. A reviewer who moved the
// evidence above the body and the label beneath it still gets back the body
// they wrote, and nothing of cr's travels with it.
func TestRecoveryFindsARegionByItsPairNotItsPosition(t *testing.T) {
	full := aFullComment(t)
	rearranged := full.Evidence + "\n\n" + full.Body + "\n\n" + full.Label + "\n\n" + full.Provenance

	assert.Equal(t, full.Body, AgentRegion(rearranged))
}

// A marker without its partner is not a region, and recovery does not guess
// where the region was meant to end. It is left in the agent region, which
// §8.1.3 then refuses naming the record, so a half-deleted region costs the
// reviewer one edit rather than costing the author a slice of cr's text
// presented as the reviewer's.
func TestAHalfDeletedRegionIsRefusedRatherThanGuessed(t *testing.T) {
	full := aFullComment(t)
	for _, region := range ownedRegions {
		for _, missing := range []string{region.open, region.close} {
			damaged := strings.Replace(full.String(), missing, "", 1)
			err := ValidateBody("f7", AgentRegion(damaged))

			var refused *BodyError
			require.ErrorAsf(t, err, &refused, "removing %s leaves a delimiter in the body", missing)
			assert.Equal(t, "f7", refused.Record)
		}
	}
}

// §8.1.3: a body is non-empty and never carries the marker comment — nor any
// `<!-- cr:` sequence, which covers every owned delimiter too — and the
// refusal names the record.
func TestABodyIsNonEmptyAndCarriesNoReservedSequence(t *testing.T) {
	for name, body := range map[string]string{
		"an empty body":            "",
		"a body of whitespace":     " \n\t\n ",
		"a record marker":          `<!-- cr:record id="f1" kind="finding" -->`,
		"an owned delimiter":       "Does this hold?\n<!-- cr:/label -->",
		"the sequence mid-line":    "See <!-- cr: here.",
		"the sequence at the end":  "Is this reachable? <!-- cr:",
		"a closing delimiter only": "<!-- cr:/evidence -->",
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateBody("f3", body)

			var refused *BodyError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, "f3", refused.Record)
			assert.Contains(t, err.Error(), "f3", "§8.1.3: the refusal names the record")
		})
	}

	for name, body := range map[string]string{
		"a sentence":                 "Does Decode's error reach the caller?",
		"an HTML comment of its own": "<!-- note to self --> Is this reachable?",
		"a similar prefix":           "<!-- crx: not reserved -->",
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, ValidateBody("f3", body))
		})
	}
}
