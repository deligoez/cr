package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// reindentingRule is panicRule with a fix whose replacement is indented with
// four spaces where the line it replaces is indented with a tab, which is
// §8.2.3's case exactly.
const reindentingRule = `{"id":"no-panic","title":"A library returns errors rather than panicking.",` +
	`"rationale":"A panic in a library takes down every caller's process.",` +
	`"class":"panic-in-library","kind":"finding","severity":"critical",` +
	`"detect":{"mode":"regex","pattern":"panic\\("},` +
	`"fix":{"replace":"\\tpanic\\((.*)\\)","with":"    return fmt.Errorf($1)"}}`

// §8.2.3 through the commands: the draft warns, shows both lines, and the
// block is still in the file the reviewer is about to read.
//
// The two halves are one claim. §8.2.3 says cr must warn *and* must still post
// when the user leaves the block in place, so a run that warned and dropped the
// suggestion would satisfy half of it while removing the thing the reviewer was
// being asked to decide about. The suggestion is therefore read back out of
// draft.md and out of the stored record, beside the warning.
func TestAReindentingSuggestionIsWarnedAboutAndStillDrafted(t *testing.T) {
	layout := detectedHome(t)
	require.NoError(t, os.WriteFile(layout.Rule("no-panic"), []byte(reindentingRule), 0o600))
	runRulesCheck(t)
	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", confirming("f1", 4)), "--repo", fixtureSlug)
	require.NoError(t, err)

	printed, err := runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err, "§8.2.3 warns; it does not refuse")

	assert.Contains(t, printed, "indented differently", "§8.2.3: the run warns")
	assert.Contains(t, printed, "f1", "the warning names the record it is about")
	assert.Contains(t, printed, "suggestion: ", "§8.2.3: it shows the suggestion's first line")
	assert.Contains(t, printed, "return fmt.Errorf")
	assert.Contains(t, printed, "replaces:", "§8.2.3: and the line it replaces")
	assert.Contains(t, printed, "panic(")

	drafted := draftOfFixture(t, layout)
	assert.Contains(t, drafted, "```suggestion\n    return fmt.Errorf(\"one\")\n```",
		"§8.2.3: the block is left in place for the reviewer to keep or edit")

	stored := storedFindings(t, layout)
	require.Len(t, stored, 1)
	assert.Equal(t, finding.StateQueued, stored[0].State)
	assert.Equal(t, "    return fmt.Errorf(\"one\")", stored[0].Suggestion,
		"nothing about the warning changes what would be posted")
}

// A suggestion indented like the line it replaces is not warned about, so the
// warning means something when it appears. A run that warned about every
// suggestion would be a run whose warning the reviewer learns to skip.
func TestASuggestionIndentedLikeItsLineIsNotWarnedAbout(t *testing.T) {
	layout := fixingHome(t)
	runRulesCheck(t)
	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", confirming("f1", 4)), "--repo", fixtureSlug)
	require.NoError(t, err)

	printed, err := runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	assert.NotContains(t, printed, "indented differently")
	assert.Contains(t, printed, `"warnings": []`, "§12.3: an empty list is printed as one")
	assert.Contains(t, draftOfFixture(t, layout), "```suggestion")
}

// A round whose records carry no suggestion reads no diff at all, which is why
// every other draft fixture needs no repository behind it: the warning is asked
// for only where there is something to warn about.
func TestADraftWithNoSuggestionAsksForNoDiff(t *testing.T) {
	draftedHome(t, aStoredRecord("f1", finding.StateDraft))

	printed, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	assert.Contains(t, printed, `"warnings": []`)
	assert.False(t, strings.Contains(printed, "indented differently"))
}
