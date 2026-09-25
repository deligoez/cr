package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/render"
)

// aProbe is the mutation probe p1, as probes.ndjson would hold it.
func aProbe() *probe.Record {
	return &probe.Record{
		ID: "p1", Kind: probe.Mutation, Target: "internal/api/handler.go:43",
		Input:  "--- a/internal/api/handler.go\n+++ b/internal/api/handler.go\n@@ -43 +43 @@\n-\tif err != nil {\n+\tif false {\n",
		Result: "no-test-failed", OutputTail: "ok  \tgithub.com/acme/api\t0.4s\n",
	}
}

// withProbe is the sources a draft holding a record graded on p1 is rendered
// with, under post.max_probe_input_bytes' default.
func withProbe() *Provenances {
	return &Provenances{Probes: map[string]*probe.Record{"p1": aProbe()}, MaxProbeInput: 4096}
}

// assertingRecords are one record of each grade: f1 cited on a location
// outside its unit, f2 probed on p1, and f3 argued, which §6.3 makes a
// question.
func assertingRecords() []*finding.Finding {
	cited := aRecord("f1")
	cited.Citations = []finding.Citation{{Path: "internal/api/decode.go", Line: 12}}
	probed := aRecord("f2")
	probed.Grade, probed.Probe = finding.GradeProbed, "p1"
	argued := aRecord("f3")
	argued.Kind, argued.Grade = finding.KindQuestion, finding.GradeArgued
	argued.Summary = "Does the caller ever see the error Decode returns?"
	return []*finding.Finding{cited, probed, argued}
}

// §8.1.7 through the draft: every record whose grade asserts carries the
// evidence region beneath its agent body, and a record whose grade does not
// carries none. The region is the one render builds from the record, so the
// block is compared whole: marker, body, and region, in §8.1.3's order.
func TestEveryAssertingGradeCarriesItsEvidenceBeneathTheBody(t *testing.T) {
	records := assertingRecords()
	rendered, err := Render(records, render.LangEN, withProbe(), nil)
	require.NoError(t, err)

	cited, err := render.CitedEvidence("f1", records[0].Citations, nil)
	require.NoError(t, err)
	probed, err := render.ProbeEvidence("f2", aProbe(), 4096, "", nil)
	require.NoError(t, err)

	assert.Contains(t, rendered, markerOf(records[0]).String()+"\n\n"+body(records[0])+"\n\n"+cited+"\n",
		"cited: the citations, beneath the body")
	assert.Contains(t, rendered, markerOf(records[1]).String()+"\n\n"+body(records[1])+"\n\n"+probed+"\n",
		"probed: the probe, input included, beneath the body")
	assert.Contains(t, probed, aProbe().Input, "the patch cr executed reaches the draft")
	_, argued, found := strings.Cut(rendered, markerOf(records[2]).String())
	require.True(t, found)
	assert.NotContains(t, argued, "<!-- cr:evidence", "argued asserts nothing, so it has nothing to show")
}

// A `probed` record whose probe the sources do not hold stops the draft,
// naming the record and the probe, rather than reaching the author in the
// assertion register with nothing beneath it to check.
func TestAProbedRecordWhoseProbeIsNotHeldStopsTheDraft(t *testing.T) {
	probed := aRecord("f2")
	probed.Grade, probed.Probe = finding.GradeProbed, "p9"

	for name, sources := range map[string]*Provenances{"no sources": nil, "another probe": withProbe()} {
		t.Run(name, func(t *testing.T) {
			rendered, err := Render([]*finding.Finding{probed}, render.LangEN, sources, nil)

			var missing *MissingProbeError
			require.ErrorAs(t, err, &missing)
			assert.Equal(t, MissingProbeError{Record: "f2", Probe: "p9"}, *missing)
			assert.Contains(t, err.Error(), "f2")
			assert.Contains(t, err.Error(), "p9")
			assert.Empty(t, rendered)
		})
	}
}

// §7.1.5 and §7.1.6 with the evidence region in place: rendered.json holds the
// agent region alone, so an untouched draft reads back with nothing to
// preserve — the byte-exact comparison still compares like with like — and a
// reviewer's typing inside the evidence region is no edit of the body, since
// §8.1.3 has cr regenerate the region and discard what was typed there.
func TestTheEvidenceRegionLeavesTheByteExactEditDetectionIntact(t *testing.T) {
	records := assertingRecords()
	file, err := File(records, render.LangEN, withProbe(), nil, headerFacts)
	require.NoError(t, err)
	entries, err := Rendered(records, render.LangEN, withProbe())
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(file, "<!-- cr:evidence -->"), "the cited and the probed record")

	for id, entry := range entries {
		assert.NotContains(t, entry, render.Reserved, "%s: rendered.json holds no cr-owned region", id)
	}
	untouched, err := ingested(records, file, entries)
	require.NoError(t, err)
	assert.Empty(t, untouched.Preserved, "a draft nobody edited has no body to keep")

	typed := strings.ReplaceAll(file, "<!-- cr:evidence -->\n", "<!-- cr:evidence -->\nI checked this myself.\n")
	edited, err := ingested(records, typed, entries)
	require.NoError(t, err)
	assert.Empty(t, edited.Preserved, "the typing sat inside a region cr owns")

	regenerated, err := File(records, render.LangEN, withProbe(), edited.Preserved, headerFacts)
	require.NoError(t, err)
	assert.Equal(t, file, regenerated, "and the region is regenerated exactly as cr built it")
}
