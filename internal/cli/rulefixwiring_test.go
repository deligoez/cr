package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// panicFixRule is panicRule carrying §2.6.2.1's fix block, which rewrites the
// line the detector matched into the error return the rule exists to ask for.
const panicFixRule = `{"id":"no-panic","title":"A library returns errors rather than panicking.",` +
	`"rationale":"A panic in a library takes down every caller's process.",` +
	`"class":"panic-in-library","kind":"finding","severity":"critical",` +
	`"detect":{"mode":"regex","pattern":"panic\\("},` +
	`"fix":{"replace":"panic\\((.*)\\)","with":"return fmt.Errorf($1)"}}`

// fixingHome is reviewedHome with the fix block added to its rule, so the
// round's detection produces hits a `fix` can rewrite. It takes reviewedHome's
// unit, the hunk's whole head-side range, because `cr record` binds a record's
// anchor to its unit and a LEFT anchor's lines reach head coordinates through
// that range.
func fixingHome(t *testing.T) state.Layout {
	t.Helper()
	layout := reviewedHome(t)
	require.NoError(t, os.WriteFile(layout.Rule("no-panic"), []byte(panicFixRule), 0o600))
	return layout
}

// draftOfFixture reads the fixture pull request's round 1 draft off disk.
func draftOfFixture(t *testing.T, layout state.Layout) string {
	t.Helper()
	body, err := os.ReadFile(
		layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft))
	require.NoError(t, err)
	return string(body)
}

// §2.6.2 end to end: a real rule with a fix block, a hit the agent confirms,
// and the generated replacement reaching the draft labelled as machine
// generated.
//
// It is driven through the commands rather than through Matcher.Suggest,
// because what closed with the generator was the generator alone: every part of
// it was written and tested while no command called it, so a rule's fix
// produced nothing on any real run. The assertion that matters here is not that
// the text is right — fix_test.go settles that — but that the path from
// `cr rules check` through `cr record` to `cr draft` exists at all.
func TestARuleFixReachesTheDraftAsAMachineGeneratedSuggestion(t *testing.T) {
	layout := fixingHome(t)
	runRulesCheck(t)

	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", confirming("f1", 4)), "--repo", fixtureSlug)
	require.NoError(t, err)

	stored := storedFindings(t, layout)
	require.Len(t, stored, 1)
	assert.Equal(t, "\treturn fmt.Errorf(\"one\")", stored[0].Suggestion,
		"§2.6.2.1: the matched line is rewritten by the rule's own fix")
	assert.Equal(t, finding.OriginRule, stored[0].SuggestionOrigin,
		"§2.6.2.4: a suggestion a fix produced carries suggestion_origin: rule")

	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	drafted := draftOfFixture(t, layout)
	assert.Contains(t, drafted, "```suggestion\n\treturn fmt.Errorf(\"one\")\n```",
		"§7.1.3: the generated replacement is rendered as a suggestion block")
	assert.Contains(t, drafted, "Machine generated from the rule's fix",
		"§2.6.2.4: the block is labelled as machine generated")
}

// §2.6.2.2's other half through the command: a suggestion §8.2 refuses is
// dropped and the record it belonged to is stored anyway.
//
// The record cites the hit it confirms and anchors on the LEFT side, where §9.2
// numbers lines in the merge base — a tree a replacement cannot be offered
// against. The anchor is the base's line 3, `func Load() {}`, which the change
// removed; the merge base has no line 4. The finding still reaches the draft,
// because the record says a rule's standard was broken at a place and the
// suggestion only says what to write instead.
func TestASuggestionSection82RefusesIsDroppedAndItsRecordIsNot(t *testing.T) {
	layout := fixingHome(t)
	runRulesCheck(t)
	record := confirming("f1", 4)
	record["anchor"].(map[string]any)["side"] = "LEFT"
	record["anchor"].(map[string]any)["start_line"] = 3
	record["anchor"].(map[string]any)["line"] = 3

	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", record), "--repo", fixtureSlug)
	require.NoError(t, err)

	stored := storedFindings(t, layout)
	require.Len(t, stored, 1)
	assert.Empty(t, stored[0].Suggestion, "§2.6.2.2: the suggestion is dropped")
	assert.Empty(t, stored[0].SuggestionOrigin)

	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	drafted := draftOfFixture(t, layout)
	assert.Contains(t, drafted, `id="f1"`, "§2.6.2.2: the record survives its dropped suggestion")
	assert.NotContains(t, drafted, "```suggestion")
}
