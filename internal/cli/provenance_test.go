package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// draftedFixture runs `cr draft` against the fixture's pull request and returns
// the draft.md it wrote for round 1.
func draftedFixture(t *testing.T, layout state.Layout) string {
	t.Helper()
	_, err := runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	written, err := os.ReadFile(layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft))
	require.NoError(t, err)
	return string(written)
}

// blockOf cuts one record's block out of a draft, from its marker to the next.
func blockOf(t *testing.T, drafted, id string) string {
	t.Helper()
	_, block, found := strings.Cut(drafted, `<!-- cr:record id="`+id+`" `)
	require.True(t, found, "the draft holds no block for %s", id)
	block, _, _ = strings.Cut(block, "<!-- cr:record ")
	return block
}

// §8.1.6's third trigger through the commands: a record whose citation §6.2.5
// stamped `origin: rule` against a real hit reaches the draft with a provenance
// region naming the rule and quoting its rationale, per §2.6 item 4. Beside it,
// a record citing a line no rule hit — `origin: agent`, and so no weak
// provenance to disclose — carries no region.
//
// The hit's record is the one that buys the assertion register mechanically:
// its citation lies inside its own unit, so only `origin: rule` made it
// `cited`, and the region is how the author learns that a detector rather than
// a reviewer located the line.
func TestARuleOriginCitationReachesTheDraftNamingTheRuleAndItsRationale(t *testing.T) {
	layout := detectedHome(t)
	runRulesCheck(t)
	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson",
		confirming("f1", 4), confirming("f2", 3)), "--repo", fixtureSlug)
	require.NoError(t, err)

	drafted := draftedFixture(t, layout)

	assert.Contains(t, blockOf(t, drafted, "f1"), "<!-- cr:provenance -->\n"+
		"rule: no-panic\n"+
		"rationale: A panic in a library takes down every caller's process.\n"+
		"<!-- cr:/provenance -->")
	assert.NotContains(t, blockOf(t, drafted, "f2"), "<!-- cr:provenance -->",
		"an agent-origin citation is no weak provenance")
}
