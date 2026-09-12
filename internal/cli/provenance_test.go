package cli

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/note"
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

// The same region when the rule-origin citation is the only citation the round
// queues — one confirmed hit and nothing else, the plainest round a rule
// produces. The case above queues an agent-origin citation beside it, so it
// cannot tell a corpus read because a citation is rule-origin from one read
// because some citation is not; draftProvenances promises the first, and only a
// round with no other citation in it asks.
func TestARoundCitingOnlyARuleHitStillQuotesTheRulesRationale(t *testing.T) {
	layout := detectedHome(t)
	runRulesCheck(t)
	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", confirming("f1", 4)),
		"--repo", fixtureSlug)
	require.NoError(t, err)

	assert.Contains(t, blockOf(t, draftedFixture(t, layout), "f1"), "<!-- cr:provenance -->\n"+
		"rule: no-panic\n"+
		"rationale: A panic in a library takes down every caller's process.\n"+
		"<!-- cr:/provenance -->")
}

// §8.1.6's second trigger through the commands: a record resting on a claim with
// `source: note` reaches the draft with a provenance region naming the claim,
// the note it came from, and that note's §3.6.3 source — read out of the round's
// claims.ndjson and the issue's context store, where §3.3.2 and §3.6.1 put them.
func TestAClaimRestingOnANoteReachesTheDraftNamingTheNoteAndItsSource(t *testing.T) {
	layout := detectedHome(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.IssueKey = fixtureIssue
	recorded, err := note.Append(layout, fixtureIssue, "Load may panic during start-up only.",
		note.SourceMeeting, fixturePRNumber, time.Now())
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Write(state.FileClaims, []byte(`{"id":"`+fixtureIssue+`#c1",`+
		`"text":"Load may panic during start-up.","source":"note",`+
		`"span":"Load may panic during start-up only.","note_id":"`+recorded.ID+`",`+
		`"head":"`+meta.Head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())

	rests := confirming("f1", 4)
	delete(rests, "rule")
	delete(rests, "citations")
	rests["claim"] = fixtureIssue + "#c1"
	_, err = runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", rests), "--repo", fixtureSlug)
	require.NoError(t, err)

	assert.Contains(t, blockOf(t, draftedFixture(t, layout), "f1"), "<!-- cr:provenance -->\n"+
		"claim: "+fixtureIssue+"#c1 (source: note)\n"+
		"note: "+recorded.ID+" (source: meeting)\n"+
		"<!-- cr:/provenance -->")
}

// §2.6 item 4 when the rule file is removed between `cr record` and `cr draft`:
// the record's citation still carries `origin: rule`, the corpus no longer
// holds its rationale, and `cr draft` refuses with §11.2's code 3 naming the
// record and the rule instead of drawing `rule: no-panic` with no rationale
// line — and writes no draft.
func TestADraftWhoseRuleFileWasRemovedRefusesNamingTheRecordAndRule(t *testing.T) {
	layout := detectedHome(t)
	runRulesCheck(t)
	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", confirming("f1", 4)),
		"--repo", fixtureSlug)
	require.NoError(t, err)
	require.NoError(t, os.Remove(layout.Rule("no-panic")))

	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)

	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Contains(t, err.Error(), "f1")
	assert.Contains(t, err.Error(), `"no-panic"`)
	assert.Contains(t, hintFor(err), "cr rules list")
	written, readErr := os.ReadFile(layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft))
	require.NoError(t, readErr)
	assert.Empty(t, string(written), "a refused draft leaves the round's draft.md as §2.3 started it")
}
