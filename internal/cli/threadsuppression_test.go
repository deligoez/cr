package cli

import (
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// storedRound is findings.ndjson as `id/state` pairs, which is the whole of
// what §3.5.4 and §9.1 can be right or wrong about here.
func storedRound(t *testing.T, l state.Layout) []string {
	t.Helper()
	stored, err := state.ReadRecords[finding.Finding](
		l, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	rendered := make([]string, 0, len(stored))
	for i := range stored {
		rendered = append(rendered, stored[i].ID+"/"+stored[i].State.String())
	}
	return rendered
}

// draftedRound reads the round's draft.md off disk after `cr draft` wrote it.
func draftedRound(t *testing.T, l state.Layout) string {
	t.Helper()
	body, err := os.ReadFile(
		l.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FileDraft))
	require.NoError(t, err)
	return string(body)
}

// A record naming an ingested thread in `suppressed_by` is stored in state
// `suppressed` and never reaches draft.md, per §3.5.4.
//
// Both halves are asserted through the two commands rather than against a
// hand-written state, because §3.5.4's promise is about what a reviewer sees
// and the only way to reach that is `cr record` then `cr draft`. A fixture that
// set the state itself would pass on a build where `cr record` never stamps it
// — which is exactly the build this test was written against.
//
// The stamp is what §9.1's second row allows and what §6.4.3 already uses for
// its own reason, so the record stays in the file: a suppressed finding is
// accountable in the same way a suppressed duplicate is, and §10.1.2's reader
// can say what the round decided not to raise.
//
// Three records with one suppressed, for the reason the duplicate case gives:
// a fixture with one of each reads the same whichever state is being counted.
func TestARecordNamingAThreadIsStoredSuppressedAndIsNeverDrafted(t *testing.T) {
	layout := recordedHome(t)
	covered := aRecord("f2", "u2")
	covered["suppressed_by"] = "PRRT_kwDOA1b2c3"
	file := writeRecordFile(t, "merged.ndjson",
		aRecord("f1", "u1"), covered, aRecord("f3", "u2"))

	printed, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	assert.Equal(t, []string{"f1/draft", "f2/suppressed", "f3/draft"}, storedRound(t, layout),
		"§3.5.4 and §9.1's second row: cr record moves a thread-covered record on to suppressed")
	assert.Contains(t, printed, `"suppressed": 1`,
		"the count reaches the caller as a number, not only inside the records")

	_, err = runDraft(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)

	assert.Equal(t, []string{"f1", "f3"}, markersIn(draftedRound(t, layout)),
		"§3.5.4: a suppressed record is not drafted, because the thread already covers it")
}

// §12.1's terminal shape for the same run names the suppressed record apart from
// the drafts, and names no state that holds nothing.
//
// gremlins found every clause of recordResult.states unasserted for this state:
// the terminal tests retire a record as a duplicate, never by a thread, so the
// suppressed count could be left out, added to the drafts instead of taken from
// them, or joined by a `0 in state duplicate` the line exists not to print, and
// no test said so.
func TestATerminalRecordNamesTheSuppressedApartFromTheDrafts(t *testing.T) {
	recordedHome(t)
	covered := aRecord("f2", "u2")
	covered["suppressed_by"] = "PRRT_kwDOA1b2c3"
	file := writeRecordFile(t, "merged.ndjson",
		aRecord("f1", "u1"), covered, aRecord("f3", "u2"))

	out := throughATerminal(t, "record", recordPR, file, "--repo", recordSlug)

	assert.Contains(t, out, "\x1b[36m3\x1b[0m: 2 in state draft, 1 in state suppressed",
		"three stored, one of them retired by the thread the agent named")
	assert.NotContains(t, out, "in state duplicate", "a state with no records in it is left out")
}

// §10.3's `suppressed_by_thread` is `cr record`'s to write, counted over the
// round's stored records.
//
// The summary tests run a round in which nothing is suppressed by a thread, so
// they read the count at zero, and gremlins found that counting down instead of
// up went unnoticed.
func TestRecordCountsTheThreadSuppressedRecordsInTheRoundSummary(t *testing.T) {
	layout := recordedHome(t)
	covered := aRecord("f2", "u2")
	covered["suppressed_by"] = "PRRT_kwDOA1b2c3"
	file := writeRecordFile(t, "merged.ndjson",
		aRecord("f1", "u1"), covered, aRecord("f3", "u2"))

	_, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	body, err := layout.ReadRound(recordOwner, recordRepo, recordPRNum, recordRound, state.FileSummary)
	require.NoError(t, err)
	document := assertSummaryShape(t, body, ownerRecord)
	assert.JSONEq(t, "1", string(document["suppressed_by_thread"]))
	assert.JSONEq(t, "0", string(document["deduplicated"]))
}

// The thread id the agent supplied is stored as it arrived, and cr writes none
// of it: §6.1.4 does not reserve `suppressed_by`, because §3.5.3 forbids cr to
// decide suppression at all.
func TestTheThreadIdIsStoredAsTheAgentSuppliedIt(t *testing.T) {
	layout := recordedHome(t)
	covered := aRecord("f1", "u1")
	covered["suppressed_by"] = "PRRT_kwDOA1b2c3"
	file := writeRecordFile(t, "merged.ndjson", covered)

	_, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 1) //nolint:testifylint // one record is the fixture
	assert.Equal(t, "PRRT_kwDOA1b2c3", stored[0].SuppressedBy,
		"§3.5.4's field holds the agent's judgement, and cr stores it rather than forming it")
}

// A finding whose summary repeats an ingested thread word for word is drafted
// like any other when the agent did not name that thread.
//
// This is §3.5.3's second sentence made observable: cr MUST NOT decide
// suppression itself. The decision has one door — `suppressed_by` on the record
// the agent hands in — and a cr that read the threads file and matched text
// would suppress this record on the strongest evidence text matching can offer,
// an exact copy of the thread's body. §4.1.5 refuses the same shortcut for
// notes and unmapped units, and for the same reason: the agent decides whether
// a thread explains a finding, and no string comparison stands in for that.
func TestAFindingRepeatingAThreadWordForWordIsStillDrafted(t *testing.T) {
	layout := recordedHome(t)
	repeated := aRecord("f1", "u1")
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileThreads, []byte(
		`{"id":"PRRT_kwDOA1b2c3","resolved":false,"outdated":false,`+
			`"author_type":"human","anchor":{"path":"internal/api/handler.go",`+
			`"side":"RIGHT","start_line":42,"line":44},`+
			`"comment":{"body":`+strconv.Quote(repeated["summary"].(string))+`},`+
			`"head":"`+recordHead+`","round":`+strconv.Itoa(recordRound)+`}`+"\n")))
	require.NoError(t, held.Unlock())

	_, err = runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson", repeated), "--repo", recordSlug)
	require.NoError(t, err)

	assert.Equal(t, []string{"f1/draft"}, storedRound(t, layout),
		"§3.5.3: cr does not decide suppression, however exactly the text agrees")

	_, err = runDraft(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)

	assert.Equal(t, []string{"f1"}, markersIn(draftedRound(t, layout)),
		"the record the agent did not suppress reaches the reviewer")
}
