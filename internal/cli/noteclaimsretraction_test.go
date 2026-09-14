package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// writeOutside writes body to a file outside every state tree and repository,
// the way an agent hands cr its own files, and returns the path.
func writeOutside(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// §3.3.3 through `cr brief`: after the issue text moves, a claim drawn from a
// note is checked against that note's body and still occurs, while a tracker
// claim whose span the new text no longer holds is reported gone.
//
// Measured on release QA before the fix (D-S03-3): the note-sourced claim was
// reported `span_occurs: false`, and the terminal said its span no longer
// occurs in the issue text, although that span never came from the issue.
func TestTheDriftReportChecksANoteClaimAgainstItsNote(t *testing.T) {
	lensParityHome(t, `{"profile":"generic"}`)
	body := "Orders over 50 ship free only in TR."
	_, err := runCLIPrinting(t, "note", fixtureIssue, body, "--source", "chat", "--pr", fixturePR)
	require.NoError(t, err)
	issue := writeOutside(t, "issue.txt", fixtureIssue+": format money amounts.\n")
	claims := writeOutside(t, "claims.ndjson",
		`{"id":"`+fixtureIssue+`#c1","text":"Format amounts.","source":"acceptance","span":"format money amounts."}`+"\n"+
			`{"id":"`+fixtureIssue+`#c2","text":"Free shipping in TR.","source":"note","note_id":"`+
			fixtureIssue+`#n1","span":"`+body+`"}`+"\n")
	_, err = runCLIPrinting(t, "claims", "record", fixturePR, claims, "--repo", fixtureSlug, "--intent-file", issue)
	require.NoError(t, err)

	moved := writeOutside(t, "issue.txt", fixtureIssue+": format money amounts in EUR.\n")
	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", moved)
	require.NoError(t, err)
	var briefed struct {
		Drift struct {
			Drifted bool `json:"drifted"`
			Claims  []struct {
				ID         string `json:"id"`
				SpanOccurs bool   `json:"span_occurs"`
			} `json:"claims"`
		} `json:"drift"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	require.True(t, briefed.Drift.Drifted, "the issue text moved")
	occurs := make(map[string]bool, len(briefed.Drift.Claims))
	for _, claim := range briefed.Drift.Claims {
		occurs[claim.ID] = claim.SpanOccurs
	}
	assert.Equal(t, map[string]bool{fixtureIssue + "#c1": false, fixtureIssue + "#c2": true}, occurs,
		"the tracker span went with the rewording; the note claim's span is still its note's body")

	// QA D-V1a-7: the terminal said the note claim's span still occurs in the
	// issue text, which never held it.
	spans := make([]string, 0, 2)
	for line := range strings.SplitSeq(throughATerminal(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", moved, "--no-color"), "\n") {
		if strings.Contains(line, "  span ") {
			spans = append(spans, line)
		}
	}
	assert.Equal(t, []string{
		"  " + fixtureIssue + "#c1  span no longer occurs in the issue text",
		"  " + fixtureIssue + "#c2  span still occurs in note " + fixtureIssue + "#n1",
	}, spans, "each line names the text its span was checked in, as span_occurs does")
}

// A §4.1.8 set-aside resting on a note §3.6.6 retracts is reported by
// `cr status` as needing re-evaluation and blocks completeness again.
//
// Measured on release QA before the fix (D-S03-5): after `cr note --remove`,
// `cr status` still counted the set-aside, listed no unstanding note, and its
// completeness reasons no longer named the claim.
func TestASetAsideOnARetractedNoteBlocksCompletenessAgain(t *testing.T) {
	statusHome(t)
	blocking := func() retractionStatus {
		t.Helper()
		printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
		require.NoError(t, err)
		var report retractionStatus
		require.NoError(t, json.Unmarshal([]byte(printed), &report))
		return report
	}
	standing := blocking()
	require.Empty(t, standing.Unstanding, "the control: the note #c2 rests on stands")
	assert.Contains(t, standing.Completeness.Reasons,
		"§10.2.3: 1 claim(s) are mapped to no unit and not set aside: "+fixtureIssue+"#c3")

	_, err := runCLIPrinting(t, "note", "--remove", fixtureIssue+"#n1")
	require.NoError(t, err)

	retracted := blocking()
	assert.Equal(t, []unstandingNote{{
		Note: fixtureIssue + "#n1", Standing: note.StandingRetracted,
		Cells: []string{}, Records: []string{}, SetAsides: []string{fixtureIssue + "#c2"},
	}}, retracted.Unstanding)
	assert.Contains(t, retracted.Completeness.Reasons,
		"§10.2.3: 2 claim(s) are mapped to no unit and not set aside: "+fixtureIssue+"#c2, "+fixtureIssue+"#c3")

	printed := throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color")
	assert.Contains(t, printed, "  "+fixtureIssue+"#n1 retracted: 0 cell(s), 0 record(s), "+
		"1 set-aside(s) need re-evaluation ("+fixtureIssue+"#c2)")
}

// retractionStatus is the part of `cr status`'s document the retraction test reads.
type retractionStatus struct {
	Unstanding   []unstandingNote `json:"unstanding_notes"`
	Completeness struct {
		Reasons []string `json:"reasons"`
	} `json:"completeness"`
}

// `cr claims set-aside` refuses a note §3.6.6 has already retracted, with exit
// code 1, and stamps nothing.
//
// Measured on release QA before the fix (D-S03-5): the set-aside exited 0 and
// took the claim out of completeness on a retracted note.
func TestSetAsideRefusesARetractedNote(t *testing.T) {
	layout := briefedForMapping(t)
	require.NoError(t, recordMapping(t, `{"claim":"`+mapIssue+`#c1","unit":"u1"}`))
	recorded, err := note.Append(layout, mapIssue, "the tax table ships in its own change",
		note.SourceChat, mapPR, time.Now())
	require.NoError(t, err)
	_, err = note.Retract(layout, recorded.ID, time.Now())
	require.NoError(t, err)

	err = runCLI(t, "claims", "set-aside", strconv.Itoa(mapPR), mapIssue+"#c2",
		"--note", recorded.ID, "--repo", mapSlug)
	var refused *RetractedSetAsideNoteError
	require.True(t, errors.As(err, &refused), "got %v", err)
	assert.Equal(t, RetractedSetAsideNoteError{Claim: mapIssue + "#c2", Note: recorded.ID}, *refused)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, []string{mapIssue + "#c2/", mapIssue + "#c3/"}, notesOf(t, layout),
		"nothing was stamped")
}

// `cr claims record` reports the §4.1.8 set-asides it clears, in the shape and
// the terminal wording `cr map record` reports the ones it drops, and an empty
// array when it cleared none.
//
// Measured on release QA before the fix (D-S03-7): claims record printed only
// the claims and the round, and the next `cr map record` reported
// `dropped_set_asides: []` while the stamp was gone.
func TestClaimsRecordReportsTheSetAsidesItClears(t *testing.T) {
	layout := briefedForMapping(t)
	require.NoError(t, recordMapping(t, `{"claim":"`+mapIssue+`#c1","unit":"u1"}`))
	recorded, err := note.Append(layout, mapIssue, "the tax table ships in its own change",
		note.SourceChat, mapPR, time.Now())
	require.NoError(t, err)
	setAside(t, mapIssue+"#c3", recorded.ID)

	issue := writeOutside(t, "issue.txt", "Retry the upload.\n")
	claims := writeOutside(t, "claims.ndjson",
		`{"id":"`+mapIssue+`#c1","text":"Retry.","source":"acceptance","span":"Retry the upload."}`+"\n"+
			`{"id":"`+mapIssue+`#c2","text":"Upload.","source":"acceptance","span":"upload"}`+"\n")
	record := func() map[string]json.RawMessage {
		t.Helper()
		printed, err := runCLIPrinting(t, "claims", "record", strconv.Itoa(mapPR), claims,
			"--repo", mapSlug, "--intent-file", issue)
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(printed), &fields))
		return fields
	}

	cleared := record()
	var dropped []mapping.DroppedSetAside
	require.NoError(t, json.Unmarshal(cleared["dropped_set_asides"], &dropped))
	assert.Equal(t, []mapping.DroppedSetAside{{Claim: mapIssue + "#c3", SetAsideNote: recorded.ID}}, dropped)
	gaps, err := state.ReadStamped[mapping.Gap](layout, mapOwner, mapRepo, mapPR, state.FileIntentGaps, 2)
	require.NoError(t, err)
	assert.Empty(t, gaps, "§3.3.1 cleared the round's entries, which is the loss reported")

	assert.Equal(t, "[]", string(record()["dropped_set_asides"]), "a run that cleared no stamp")

	// §12.1's terminal shape: the cleared claim is named, and a run that
	// cleared none says nothing about set-asides.
	require.NoError(t, recordMapping(t, `{"claim":"`+mapIssue+`#c1","unit":"u1"}`))
	setAside(t, mapIssue+"#c2", recorded.ID)
	named := throughATerminal(t, "claims", "record", strconv.Itoa(mapPR), claims,
		"--repo", mapSlug, "--intent-file", issue, "--no-color")
	assert.Contains(t, named, "; mapping cleared; dropped the set-aside of "+mapIssue+"#c2")
	quiet := throughATerminal(t, "claims", "record", strconv.Itoa(mapPR), claims,
		"--repo", mapSlug, "--intent-file", issue, "--no-color")
	assert.Contains(t, quiet, "; mapping cleared")
	assert.NotContains(t, quiet, "dropped the set-aside")
}
