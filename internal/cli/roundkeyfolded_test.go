package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// §9.3.5 scopes a read to the round a stored line carries, and every read of
// the line decodes it into a struct, which binds `"Round"` exactly as it binds
// `"round"`. So the round a round-scoped read filters by is read under the
// same folding: a line keyed `Round` is a record of its round, not of round 0.
//
// Measured before stored-round-key-folded: `cr status` counted 1 record, the
// Round-keyed line reading as round 0 and dropping out of the round.
func TestStatusCountsAStoredLineWhoseRoundKeyIsSpelledInAnotherCase(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(
		`{"id":"f1","state":"draft","head":"`+meta.Head+`","round":1}`+"\n\n"+
			`{"id":"f2","state":"queued","Round":1}`+"\n"+
			`{"id":"f3","state":"queued","ROUND":2}`+"\n")))
	require.NoError(t, held.Unlock())

	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)

	require.NoError(t, err)
	var report struct {
		Records recordReport `json:"records"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	byState := make([]tally, 0)
	for _, name := range stateNames() {
		count := 0
		if name == finding.StateDraft.String() || name == finding.StateQueued.String() {
			count = 1
		}
		byState = append(byState, tally{Name: name, Count: count})
	}
	assert.Equal(t, 2, report.Records.Total, "f1 and f2 are round 1's; f3 is round 2's")
	assert.Equal(t, byState, report.Records.ByState)
}

// A line giving round twice under that folding has no one round a read binds,
// so the round-scoped read refuses it as cr's own file it cannot use: exit 3,
// state.UnusableHint, the path and the line, counting the blank one.
//
// Measured before stored-round-key-folded: `cr status` exited 0, filtering the
// line by its lowercase key alone.
func TestStatusRefusesAStoredLineGivingRoundTwiceUnderCaseFolding(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(
		`{"id":"f1","state":"draft","head":"`+meta.Head+`","round":1}`+"\n\n"+
			`{"id":"f2","state":"queued","round":1,"Round":2}`+"\n")))
	require.NoError(t, held.Unlock())

	_, err = runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)

	require.Error(t, err)
	path := layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	assert.Equal(t, path+" line 3: cannot use findings.ndjson: round is given more than once", err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2 codes a file cr cannot use 3")
	assert.Equal(t, state.UnusableHint, hintFor(err))
}

// storedFindingLines are findings.ndjson's lines as bytes, the trailing newline
// dropped.
func storedFindingLines(t *testing.T, layout state.Layout) []string {
	t.Helper()
	body, err := os.ReadFile(layout.PRFile(draftOwner, draftRepo, draftPRNum, state.FileFindings))
	require.NoError(t, err)
	return strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
}

// rekeyedRound rewrites the round key of the second stored findings line from
// `"round":2` to with, and puts a round-1 line keyed `Round` in front of both.
func rekeyedRound(t *testing.T, layout state.Layout, with string) (earlier string) {
	t.Helper()
	lines := storedFindingLines(t, layout)
	require.Len(t, lines, 2)
	require.Equal(t, 1, strings.Count(lines[1], `"round":2`))
	lines[1] = strings.Replace(lines[1], `"round":2`, with, 1)
	earlier = `{"id":"f0","state":"queued","Round":1}`
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings,
		[]byte(earlier+"\n"+strings.Join(lines, "\n")+"\n")))
	require.NoError(t, held.Unlock())
	return earlier
}

// The same reading through a command that reads the round and then replaces
// it. `cr draft` renders the round's records and writes them back through
// §9.3.5's round-scoped replacement, and the two have to agree about a line
// keyed `Round`: the read seeing it and the writer keeping it would store the
// record twice, and the writer dropping a line the read never saw would delete
// it.
//
// The round-1 line keyed `Round` is history, and survives byte for byte.
//
// Measured before stored-round-key-folded: the draft rendered f1 alone, and
// findings.ndjson kept f2 in draft beside the replaced round.
func TestDraftRendersAndReplacesAStoredLineWhoseRoundKeyIsSpelledInAnotherCase(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f1", finding.StateDraft), aStoredRecord("f2", finding.StateDraft))
	earlier := rekeyedRound(t, layout, `"Round":2`)

	_, err := runDraft(t, draftPR, "--repo", draftSlug)

	require.NoError(t, err)
	assert.Equal(t, []string{"f1", "f2"}, markersIn(readDraft(t, layout)))
	stored, err := state.ReadStamped[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
	require.NoError(t, err)
	type held struct {
		ID    string
		State finding.State
	}
	records := make([]held, 0, len(stored))
	for _, record := range stored {
		records = append(records, held{record.ID, record.State})
	}
	assert.Equal(t, []held{{"f1", finding.StateQueued}, {"f2", finding.StateQueued}}, records)
	lines := storedFindingLines(t, layout)
	require.Len(t, lines, 3, "the earlier round's line and the round's two records")
	assert.Equal(t, earlier, lines[0])
}

// A line giving round twice under folding refuses `cr draft` before it writes
// anything, as it refuses `cr status`.
//
// Measured before stored-round-key-folded: the draft exited 0.
func TestDraftRefusesAStoredLineGivingRoundTwiceUnderCaseFolding(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f1", finding.StateDraft), aStoredRecord("f2", finding.StateDraft))
	rekeyedRound(t, layout, `"round":2,"Round":2`)
	before := stateFiles(t, layout)

	_, err := runDraft(t, draftPR, "--repo", draftSlug)

	require.Error(t, err)
	path := layout.PRFile(draftOwner, draftRepo, draftPRNum, state.FileFindings)
	assert.Equal(t, path+" line 3: cannot use findings.ndjson: round is given more than once", err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2 codes a file cr cannot use 3")
	assert.Equal(t, state.UnusableHint, hintFor(err))
	assert.Equal(t, before, stateFiles(t, layout), "a refused draft writes nothing")
}
