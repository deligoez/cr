package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The pull request `cr draft` is exercised against, and the round it stands in.
// The round is past its first for the reason `cr record`'s fixture gives:
// §9.3.5 scopes a command to the current round, and a fixture in round 1 could
// not tell records drawn from the round apart from records drawn from the file.
const (
	draftOwner = "acme"
	draftRepo  = "web"
	draftSlug  = draftOwner + "/" + draftRepo
	draftPR    = "7"
	draftPRNum = 7
	draftRound = 2
	draftHead  = "4f3e2d1c0b9a87766554433221100ffeeddccbba"
)

// draftedHome puts a state root behind CR_HOME holding one pull request in
// round draftRound, with the records passed to it already in findings.ndjson.
//
// The records are written straight to the file rather than through `cr record`,
// because the states this command has to tell apart are states `cr record` can
// only reach two of. §9.1's table brings a record into `draft`, `duplicate` or
// `suppressed`; `discarded`, `posted` and `stale` are written by commands that
// do not exist yet, and the draft has to leave all of them out today.
func draftedHome(t *testing.T, records ...*finding.Finding) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(draftOwner, draftRepo, draftPRNum))

	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum,
		Round: draftRound, Head: draftHead,
	}))
	require.NoError(t, state.ReplaceStamped(
		held, state.FileFindings,
		state.Stamp{Head: draftHead, Round: draftRound}, records,
	))
	require.NoError(t, held.Unlock())
	standInDiff(t, draftDiff)
	return layout
}

// aStoredRecord is one record as findings.ndjson holds it: the §6.1 fields cr
// wrote as well as the ones the agent supplied, since this fixture stands in
// for a round `cr record` already accepted.
func aStoredRecord(id string, at finding.State) *finding.Finding {
	return &finding.Finding{
		ID:       id,
		Kind:     finding.KindFinding,
		Axis:     "correctness",
		Role:     "correctness",
		Class:    "unchecked-error",
		Severity: finding.SeverityHigh,
		Grade:    finding.GradeCited,
		Unit:     "u1",
		Anchor: finding.Anchor{
			Path: "internal/api/handler.go", Side: "RIGHT",
			StartLine: 42, Line: 44, ContentHash: "0123456789abcdef",
		},
		Summary:  "The error Decode returns is dropped, " + id + ".",
		Evidence: "The call's second result is assigned to the blank identifier.",
		State:    at,
	}
}

// runDraft runs `cr draft` with args against whatever CR_HOME points at, and
// returns what it printed and what it refused.
func runDraft(t *testing.T, args ...string) (printed string, err error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"draft"}, args...))
	err = cmd.Execute()
	return out.String(), err
}

// readDraft reads the current round's draft.md off disk.
func readDraft(t *testing.T, layout state.Layout) string {
	t.Helper()
	body, err := os.ReadFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft))
	require.NoError(t, err)
	return string(body)
}

// markersIn returns the record id of every §7.1.1 marker in a rendered draft,
// in the order they appear, so a test can say what the file holds without
// depending on the rest of the grammar.
func markersIn(rendered string) []string {
	ids := make([]string, 0)
	for line := range strings.SplitSeq(rendered, "\n") {
		if !strings.HasPrefix(line, "<!-- cr:record ") {
			continue
		}
		id, _, _ := strings.Cut(strings.TrimPrefix(line, `<!-- cr:record id="`), `"`)
		ids = append(ids, id)
	}
	return ids
}

// §7.1 through the command: every queued record becomes exactly one block in
// one Markdown file at rounds/<n>/draft.md, and no record in any other state
// does.
//
// The fixture holds one record in each of §9.1's seven states, which is what
// makes the second half of that sentence provable rather than asserted about
// the two states a fixture usually carries. Six of the seven must not appear,
// and each is left out for its own reason: a duplicate and a thread-suppressed
// record are already spoken for, a discard wrote a waiver, a posted record is
// on GitHub, and a stale record belongs to a head that moved. Only `draft` is
// rendered, and it is rendered because §9.1's one row into `queued` is the one
// this command owns.
func TestDraftRendersEveryQueuedRecordAndNothingElse(t *testing.T) {
	open := aStoredRecord("f1", finding.StateDraft)
	second := aStoredRecord("f2", finding.StateDraft)
	layout := draftedHome(t,
		open,
		aStoredRecord("f3", finding.StateDuplicate),
		aStoredRecord("f4", finding.StateSuppressed),
		second,
		aStoredRecord("f5", finding.StateDiscarded),
		aStoredRecord("f6", finding.StatePosted),
		aStoredRecord("f7", finding.StateStale),
	)

	printed, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	rendered := readDraft(t, layout)
	assert.Equal(t, []string{"f1", "f2"}, markersIn(rendered),
		"§7.1: the queued records are the draft, in the order findings.ndjson holds them")
	assert.Equal(t, 1, strings.Count(rendered, `id="f1"`),
		"one queued record is exactly one block, never two")
	assert.Contains(t, rendered, open.Summary, "§7.1.2: each block carries a body")
	assert.Contains(t, printed, `"queued": 2`)
	assert.Contains(t, printed, state.FileDraft,
		"the path is reported, since §2.2 puts it somewhere the caller cannot derive")
}

// §9.1's `draft` → `queued` row is stamped by the same run that rendered the
// block, so the state and the file agree.
//
// The two are asserted together on purpose. A record queued without being
// rendered is a record §10.2.4 will hold the round open for and no reviewer can
// see; a record rendered without being queued is a comment that would be drawn
// again by the next run. Both are wrong in the same way — the file and the
// state disagreeing about what is in front of the human — and only a check that
// reads both can tell.
func TestDraftQueuesExactlyTheRecordsItRendered(t *testing.T) {
	layout := draftedHome(t,
		aStoredRecord("f1", finding.StateDraft),
		aStoredRecord("f2", finding.StateDuplicate),
	)

	_, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 2, "the round's records are replaced, never appended to")
	assert.Equal(t, finding.StateQueued, stored[0].State,
		"§9.1: cr draft is the actor that moves a record from draft to queued")
	assert.Equal(t, finding.StateDuplicate, stored[1].State,
		"§9.1 gives cr draft no row out of duplicate, so it leaves one alone")

	assert.Equal(t, []string{"f1"}, markersIn(readDraft(t, layout)))
}

// §9.3.5: the draft is the current round's, and an earlier round's records are
// history rather than material.
//
// Round 1's record is left in `draft` rather than `stale`, which is the shape
// this has to survive: §9.3.4 moves the open records of a round the head
// outran, but a round that ended some other way leaves them where they are, and
// a command that filtered on the state alone would render them into round 2's
// draft as if they had just been recorded.
func TestDraftReadsOnlyTheCurrentRoundsRecords(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f2", finding.StateDraft))

	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, state.AppendStamped(
		held, state.FileFindings,
		state.Stamp{Head: "0000000000000000000000000000000000000000", Round: draftRound - 1},
		[]*finding.Finding{aStoredRecord("f1", finding.StateDraft)},
	))
	require.NoError(t, held.Unlock())

	_, err = runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	assert.Equal(t, []string{"f2"}, markersIn(readDraft(t, layout)),
		"§9.3.5: earlier rounds are history, and this round's draft is not the place for them")

	stored, err := state.ReadRecords[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 2, "and the earlier round's line survives the replacement")
	for _, record := range stored {
		if record.Round == draftRound-1 {
			assert.Equal(t, finding.StateDraft, record.State,
				"§9.3.5: a command leaves earlier rounds intact, states included")
		}
	}
}

// A round with nothing queued still gets a file. §7.1.6 makes the draft
// regenerable, so the artefact the reviewer opens has to say what this round
// holds — and a run that wrote nothing would leave whatever the round before
// wrote there, which reads as a draft rather than as an absence.
func TestDraftWritesTheFileEvenWithNothingQueued(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f1", finding.StateDuplicate))

	printed, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	assert.Empty(t, markersIn(readDraft(t, layout)))
	assert.Contains(t, printed, `"queued": 0`)
}

// A pull request no round has been opened on has no rounds/<n>/ to write into,
// and §11.2 codes that 4 rather than answering with round 0.
func TestDraftRefusesAPullRequestWithNoRound(t *testing.T) {
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(draftOwner, draftRepo, draftPRNum))

	_, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.Error(t, err)
	assert.Equal(t, ExitState, exitCodeFor(err), "§11.2 codes a state conflict 4")

	var unbriefed *state.NotBriefedError
	assert.ErrorAs(t, err, &unbriefed, "the command raises no refusal of its own")
}

// §12.1's other shape for this command. A terminal reader is told the count and
// the file to open, because the blocks are in the file rather than on stdout.
func TestATerminalDraftNamesTheCountAndTheFile(t *testing.T) {
	draftedHome(t,
		aStoredRecord("f1", finding.StateDraft),
		aStoredRecord("f2", finding.StateDraft))

	out := throughATerminal(t, "draft", draftPR, "--repo", draftSlug)

	assert.Contains(t, out, "drafted ")
	assert.Contains(t, out, "\x1b[36m2\x1b[0m",
		"the count is accented, as every terminal rendering accents its answer")
	assert.Contains(t, out, state.FileDraft)
}
