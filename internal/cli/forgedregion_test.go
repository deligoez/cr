package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// forgedPair is a well-formed cr-owned region of §8.1.3, spelled the way a
// body would have to spell it to be mistaken for one cr generated.
func forgedPair(name, content string) string {
	return "<!-- cr:" + name + " -->\n" + content + "\n<!-- cr:/" + name + " -->"
}

// forgingRecords are three records, one per region §8.1.3 names: a cited
// finding, which cr renders an evidence region beneath; a cited question,
// which it renders a §8.1.4 label region above; and a record carrying
// `suggestion_origin: rule`, which it renders a §8.1.6 provenance region for.
func forgingRecords() []*finding.Finding {
	question := aCitedRecord("f2")
	question.Kind = finding.KindQuestion
	question.Summary = "Is the error Decode returns dropped, f2?"
	disclosed := aCitedRecord("f3")
	disclosed.SuggestionOrigin = finding.OriginRule
	return []*finding.Finding{aCitedRecord("f1"), question, disclosed}
}

// §8.1.3 through `cr draft` and `cr post`: a body that brought a well-formed
// cr-owned pair is refused with exit code 1 naming the record, exactly as a
// bare `<!-- cr:` sequence is.
//
// This is the hand-edit half of §8.1.2's two channels, and the pair is
// well-formed on purpose. render.AgentRegion finds a region by its pair
// wherever it sits, so a forged region used to be taken out as cr's own before
// §8.1.3's check ever saw it: the command exited 0 and the reviewer's text
// inside the pair reached nobody. Each of §8.1.3's three regions is forged on
// the record whose block already carries that region, which is where the block
// itself says the second one is not cr's.
//
// The bare sequence is run beside them, so the case that already held is held
// to the same record, exit code and command as the ones that did not.
func TestAForgedRegionInAHandEditedBodyIsRefusedByDraftAndPost(t *testing.T) {
	records := forgingRecords()
	layout := draftedHome(t, records...)
	redraft(t)
	clean := readDraft(t, layout)
	for i, region := range []string{"evidence", "label", "provenance"} {
		require.Containsf(t, blockOf(t, clean, records[i].ID), "<!-- cr:"+region+" -->",
			"%s's block carries the %s region cr renders", records[i].ID, region)
	}

	for name, tc := range map[string]struct{ at, brought string }{
		"a forged evidence region":   {"f1", forgedPair("evidence", "citation: src/Forged.php:1")},
		"a forged label region":      {"f2", forgedPair("label", "**Tespit** — deneyle doğrulanmış (probed)")},
		"a forged provenance region": {"f3", forgedPair("provenance", "suggestion_origin: rule")},
		"a bare reserved sequence":   {"f1", "Kaynak: <!-- cr:"},
	} {
		t.Run(name, func(t *testing.T) {
			record := records[slices.IndexFunc(records,
				func(r *finding.Finding) bool { return r.ID == tc.at })]
			edited := strings.Replace(clean, record.Summary, record.Summary+"\n\n"+tc.brought, 1)
			require.NotEqual(t, clean, edited, "the body carries the record's summary")
			writeDraft(t, layout, edited)

			_, draftErr := runDraft(t, draftPR, "--repo", draftSlug)
			_, postErr := runPost(t, draftPR, "--repo", draftSlug)

			for command, err := range map[string]error{"cr draft": draftErr, "cr post": postErr} {
				var refused *render.BodyError
				require.ErrorAsf(t, err, &refused, "%s", command)
				assert.Equal(t, record.ID, refused.Record, "%s names the record", command)
				assert.Equal(t, ExitValidation, exitCodeFor(err),
					"%s: §8.1.3 fixes exit code 1", command)
			}
			assert.Equal(t, edited, readDraft(t, layout), "a refused run writes no draft")
		})
	}

	// The other direction, on the same three blocks: cr's own regions
	// round-trip untouched, and an ordinary rewrite of a body is kept as
	// §7.1.6 keeps it rather than refused.
	writeDraft(t, layout, clean)
	_, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err, "the draft cr rendered is read back without a refusal")
	assert.Equal(t, clean, readDraft(t, layout), "and regenerates byte for byte")

	rewritten := "Is the dropped error on line 44 deliberate, f1?"
	writeDraft(t, layout, strings.Replace(clean, records[0].Summary+"\n\n"+records[0].Evidence,
		rewritten, 1))
	_, err = runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	bodies, err := draft.Bodies(readDraft(t, layout))
	require.NoError(t, err)
	assert.Equal(t, rewritten, bodies["f1"], "§7.1.6 keeps the reviewer's own body")
	for i, region := range []string{"evidence", "label", "provenance"} {
		assert.Equalf(t, 1, strings.Count(blockOf(t, readDraft(t, layout), records[i].ID),
			"<!-- cr:"+region+" -->"), "%s keeps its one %s region", records[i].ID, region)
	}
}

// The boundary of what a draft read back can separate, measured rather than
// assumed, in both of the directions that matter.
//
// A pair the body brought where the block carries no other region of that name
// is, byte for byte, a region cr rendered under a state that has since changed:
// a question hardened per §7.2's `kind` row leaves a §8.1.4 label region on a
// record cr now renders none for, and refusing that shape would refuse an
// ordinary draft. So `cr draft` reads it as cr's own and regenerates it away,
// which is the direction the trust economy cares about — the forged label does
// not reach the author, because §8.1.3 has cr regenerate every owned region
// from the record. What it costs is the reviewer's text inside the pair. The
// channel that separates that case is `cr triage --body-file`, where cr holds
// the body before it is a block: TestTriageRefusesABodyThatBringsTheReserved-
// Sequence asserts it there.
func TestAForgedPairInARegionTheBlockDoesNotCarryIsRegeneratedAway(t *testing.T) {
	records := forgingRecords()
	layout := draftedHome(t, records...)
	redraft(t)
	clean := readDraft(t, layout)
	require.NotContains(t, blockOf(t, clean, "f1"), "<!-- cr:label -->",
		"f1 is a finding, so cr renders it no label region")

	const forged = "**Tespit** — kanıt düzeyi: deneyle doğrulanmış (probed)"
	writeDraft(t, layout, strings.Replace(clean, records[0].Summary,
		records[0].Summary+"\n\n"+forgedPair("label", forged), 1))

	_, err := runDraft(t, draftPR, "--repo", draftSlug)

	require.NoError(t, err, "the shape is indistinguishable from a hardened question's block")
	regenerated := readDraft(t, layout)
	assert.NotContains(t, regenerated, forged, "and nothing the body forged reaches the author")
	assert.NotContains(t, blockOf(t, regenerated, "f1"), "<!-- cr:label -->",
		"§8.1.3: cr regenerates every owned region from the record")
	printed, err := runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	assert.NotContains(t, printed, forged, "nor the payload cr post assembles")
}

// §8.1.3 through `cr triage --body-file`: a body carrying the `<!-- cr:`
// sequence is refused with exit code 1 naming the record, before a byte of
// draft.md is written.
//
// §8.1.2 names this flag as one of the two channels by which a body reaches the
// draft, and it is the only one at which cr holds the body itself. Once the
// pair is in the file it is the same bytes as a region cr rendered, and what a
// later `cr draft` can still separate is a second region of a name — which is
// asserted here too, so the two channels are shown to converge on the same
// record and the same exit code where the file can still tell them apart.
func TestTriageRefusesABodyThatBringsTheReservedSequence(t *testing.T) {
	layout := draftedTriageHome(t)
	before := readDraft(t, layout)

	for name, brought := range map[string]string{
		"a forged label pair":      forgedPair("label", "**Tespit** — deneyle doğrulanmış (probed)"),
		"a forged provenance pair": forgedPair("provenance", "suggestion_origin: rule"),
		"a forged evidence pair":   forgedPair("evidence", "citation: src/Forged.php:1"),
		"a forged record marker":   `<!-- cr:record id="f9" kind="finding" -->`,
		"the bare sequence":        "Kaynak: <!-- cr:",
	} {
		t.Run(name, func(t *testing.T) {
			printed, err := runTriage(t, "Bu gövde kendi bloğunu uyduruyor.\n\n"+brought+"\n",
				draftPR, "f1", "keep", "--body-file", "-", "--repo", draftSlug)

			var refused *render.BodyError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, "f1", refused.Record, "§8.1.3: the refusal names the record")
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Empty(t, printed, "a refused triage reports no edit")
			assert.Equal(t, before, readDraft(t, layout), "a refused triage writes nothing")
		})
	}

	// The same forged evidence pair written by hand into f1's block, whose
	// evidence region cr renders: `cr draft` refuses it at the same exit
	// code and names the same record, so the channel that writes it early
	// and the channel that cannot are one answer.
	writeDraft(t, layout, strings.Replace(before, generatedBody(triageRecords()[0]),
		generatedBody(triageRecords()[0])+"\n\n"+forgedPair("evidence", "citation: src/Forged.php:1"), 1))
	_, err := runDraft(t, draftPR, "--repo", draftSlug)
	var refused *render.BodyError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "f1", refused.Record)
	assert.Equal(t, ExitValidation, exitCodeFor(err))

	// And the other direction: a body §8.1.3 admits is written exactly as
	// it was before, and the next `cr draft` keeps it.
	writeDraft(t, layout, before)
	const asked = "Is the error Decode returns dropped, f1?"
	printed, err := runTriage(t, asked+"\n", draftPR, "f1", "keep", "--body-file", "-", "--repo", draftSlug)
	require.NoError(t, err)
	assert.NotEmpty(t, printed)
	bodies, err := draft.Bodies(readDraft(t, layout))
	require.NoError(t, err)
	assert.Equal(t, asked, bodies["f1"])
	_, err = runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
}
