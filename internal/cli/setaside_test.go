package cli

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// gapsWithANote is the round a set-aside runs against: three claims, one of
// them mapped, so §4.1.3 raises entries for the other two — and one note in the
// §3.6 store for the pull request's issue key.
//
// The entries are derived by `cr map record` rather than written here, because
// §4.1.7 makes that command the one that raises them and §4.1.8 stamps what it
// raised. A fixture writing its own entries would let the stamp land on a file
// no derivation produced.
func gapsWithANote(t *testing.T) (layout state.Layout, noteID string) {
	t.Helper()
	layout = briefedForMapping(t)
	require.NoError(t, recordMapping(t, `{"claim":"`+mapIssue+`#c1","unit":"u1"}`))
	recorded, err := note.Append(layout, mapIssue,
		"the tax table ships in its own change", note.SourceChat, mapPR, time.Now())
	require.NoError(t, err)
	return layout, recorded.ID
}

// setAsideRun runs `cr claims set-aside` and returns what it printed alongside
// what it refused, so a test can assert on either.
func setAsideRun(t *testing.T, claim, noteID string) (claimsSetAsideResult, error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"claims", "set-aside", strconv.Itoa(mapPR), claim, "--note", noteID, "--repo", mapSlug,
	})
	err := cmd.Execute()
	var printed claimsSetAsideResult
	if err == nil {
		require.NoError(t, json.Unmarshal(out.Bytes(), &printed))
	}
	return printed, err
}

// storedGaps is intent-gaps.ndjson as `claim@round/note`, which is the whole of
// what a set-aside can be right or wrong about.
func storedGaps(t *testing.T, l state.Layout) []string {
	t.Helper()
	stored, err := state.ReadRecords[mapping.Gap](l, mapOwner, mapRepo, mapPR, state.FileIntentGaps)
	require.NoError(t, err)
	rendered := make([]string, 0, len(stored))
	for i := range stored {
		rendered = append(rendered,
			stored[i].Claim+"@"+strconv.Itoa(stored[i].Round)+"/"+stored[i].SetAsideNote)
	}
	return rendered
}

// §4.1.8: the note id is stamped onto that claim's entry, and onto nothing
// else.
//
// The neighbouring entry is what makes the assertion worth making. The write is
// a whole-round replacement — state.ReplaceStamped takes what it is handed and
// keeps nothing it is not — so an implementation that returned only the entry it
// changed would delete c3's, and §10.2.3 would stop blocking on an
// unimplemented claim nobody set aside. Nothing in the printed result would say
// so.
func TestASetAsideStampsTheNoteOnThatClaimsEntryAndNoOther(t *testing.T) {
	layout, noteID := gapsWithANote(t)

	printed, err := setAsideRun(t, mapIssue+"#c2", noteID)
	require.NoError(t, err)

	assert.Equal(t, mapIssue+"#c2", printed.Claim)
	assert.Equal(t, noteID, printed.Note)
	assert.Equal(t, 2, printed.Round, "§9.3.5: the entry stamped is the current round's")

	assert.Equal(t, []string{
		mapIssue + "#c2@2/" + noteID,
		mapIssue + "#c3@2/",
	}, storedGaps(t, layout),
		"§4.1.8 stamps one entry; every other entry of the round stays as it was")
}

// A set-aside naming a note the store does not hold is rejected with exit code
// 1, and stamps nothing.
//
// This is the criterion's own case, and the refusal is only half of it. §4.1.8
// makes the note the whole of what a set-aside rests on, so a run that refused
// and had already written would leave a stamp behind whose note nobody can
// open — and §10.2.3 would read that entry as one that no longer blocks.
func TestASetAsideNamingNoNoteIsRefusedAndWritesNothing(t *testing.T) {
	layout, _ := gapsWithANote(t)
	before := storedGaps(t, layout)

	_, err := setAsideRun(t, mapIssue+"#c2", mapIssue+"#n9")

	require.Error(t, err)
	var unknown *note.UnknownNoteError
	require.ErrorAs(t, err, &unknown,
		"§4.1.8 checks the note exists for the PR's issue key, so that is what refuses")
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§11.2 codes a bad note id 1")
	assert.Equal(t, mapIssue, unknown.IssueKey,
		"the store searched is the pull request's own, per §4.1.8")
	assert.Contains(t, err.Error(), "cr context "+mapIssue,
		"§12.4: the refusal names the next actionable step")

	assert.Equal(t, before, storedGaps(t, layout),
		"§4.1.8 checks before it stamps, so a refused run leaves the entries as they were")
}

// A set-aside naming a claim the round raises no entry for is rejected with
// exit code 1, and stamps nothing.
//
// c1 is mapped to a unit, so §4.1.3 raises nothing for it and §4.1.8 has no
// entry to stamp. It is the sharper half of the two refusals: an id the round
// has never heard of is obviously wrong, while this one is a real claim of the
// round, and an implementation that appended an entry instead would invent an
// unimplemented-claim record for a claim that is implemented.
func TestASetAsideOfAMappedClaimIsRefusedAndWritesNothing(t *testing.T) {
	layout, noteID := gapsWithANote(t)
	before := storedGaps(t, layout)

	_, err := setAsideRun(t, mapIssue+"#c1", noteID)

	require.Error(t, err)
	var noGap *mapping.NoGapError
	require.ErrorAs(t, err, &noGap)
	assert.Equal(t, ExitValidation, exitCodeFor(err),
		"§11.2 codes a claim id the round raises no entry for 1")
	assert.Equal(t, mapIssue+"#c1", noGap.Claim)
	assert.Equal(t, 2, noGap.Round, "§9.3.5 makes the answer the round's")
	assert.Contains(t, err.Error(), "cr status",
		"§12.4: the refusal names the next actionable step")

	assert.Equal(t, before, storedGaps(t, layout),
		"a refused set-aside adds no entry and changes none")
}

// The stamp is the note id the caller gave, and cr judges nothing about it.
//
// §4.1.8 reserves the judgement for the agent, which means two things a test
// can see. Re-running with a different note replaces the reference rather than
// refusing, because correcting which note a decision cites is the agent's to
// do; and the note's body reaches the entry not at all, because a cr that read
// it would be deciding whether it justifies the set-aside.
func TestASetAsideRecordsTheAgentsNoteAndFormsNoOpinionOfIt(t *testing.T) {
	layout, first := gapsWithANote(t)
	second, err := note.Append(layout, mapIssue,
		"the tax table is tracked separately", note.SourceChat, mapPR, time.Now())
	require.NoError(t, err)
	require.NotEqual(t, first, second.ID)

	_, err = setAsideRun(t, mapIssue+"#c2", first)
	require.NoError(t, err)
	_, err = setAsideRun(t, mapIssue+"#c2", second.ID)
	require.NoError(t, err)

	stored, err := state.ReadRecords[mapping.Gap](
		layout, mapOwner, mapRepo, mapPR, state.FileIntentGaps)
	require.NoError(t, err)
	require.Len(t, stored, 2, "a second set-aside of one claim adds no entry")

	for i := range stored {
		if stored[i].Claim != mapIssue+"#c2" {
			continue
		}
		assert.Equal(t, second.ID, stored[i].SetAsideNote,
			"§4.1.8 gives the field no history, so the later reference stands")
		assert.NotContains(t, stored[i].SetAsideNote, "tracked separately",
			"the entry carries the note id and never the note's text")
	}
}

// The set-aside is scoped to the round, and leaves an earlier round's entry
// byte for byte, per §9.3.5.
//
// The fixture's earlier round carries a stamp of its own, which is the case
// that separates a round-scoped write from a whole-file one: a writer that
// replaced the file would take round 1's judgement out along with its entry,
// and §9.3.5 calls that history.
func TestASetAsideLeavesAnEarlierRoundsEntryAlone(t *testing.T) {
	layout, noteID := gapsWithANote(t)
	held, err := layout.LockPR(mapOwner, mapRepo, mapPR)
	require.NoError(t, err)
	earlier := `{"claim":"` + mapIssue + `#c9","head":"0f1e2d3","round":1,` +
		`"set_aside_note":"` + mapIssue + `#n1"}` + "\n"
	body, err := layout.ReadPR(mapOwner, mapRepo, mapPR, state.FileIntentGaps)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileIntentGaps, append([]byte(earlier), body...)))
	require.NoError(t, held.Unlock())

	_, err = setAsideRun(t, mapIssue+"#c2", noteID)
	require.NoError(t, err)

	assert.Equal(t, []string{
		mapIssue + "#c9@1/" + mapIssue + "#n1",
		mapIssue + "#c2@2/" + noteID,
		mapIssue + "#c3@2/",
	}, storedGaps(t, layout),
		"§9.3.5: round 1's entry is history and keeps the stamp it carried")
}
