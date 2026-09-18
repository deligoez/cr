package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// recordAtFirstID runs `cr record` over one question at the first id the
// prompt names, and returns the report it printed.
func recordAtFirstID(t *testing.T, prompt *review.Prompt) review.NotesAfterPrompts {
	t.Helper()
	line, err := json.Marshal(map[string]any{
		"id": prompt.FirstID, "kind": "question", "role": prompt.Role, "class": "unchecked-error",
		"severity": "low", "unit": prompt.Unit,
		"anchor":  map[string]any{"path": "lib.go", "side": "RIGHT", "start_line": 1, "line": 1},
		"summary": "Is the error Load returns dropped?", "evidence": "Nothing reads the result.",
	})
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "records.ndjson")
	require.NoError(t, os.WriteFile(file, append(line, '\n'), 0o600))
	printed, err := runCLIPrinting(t, "record", fixturePR, file, "--repo", fixtureSlug)
	require.NoError(t, err)
	var result struct {
		NotesAfterPrompts review.NotesAfterPrompts `json:"notes_after_prompts"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &result))
	return result.NotesAfterPrompts
}

// field-feedback 1.5 end to end: a note recorded after `cr review` emitted its
// prompts is reported by `cr note` against the pass that did not carry it, by
// `cr record` against the record written from such a prompt, counted by
// `cr status`, and shown in the draft header — while a record written from a
// prompt emitted again after the note, and a note typed with no repository, are
// reported against nothing.
func TestANoteRecordedAfterThePromptsIsReportedFromNoteToDraft(t *testing.T) {
	statusHome(t)
	layout, err := state.Default()
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	// statusHome files a pass cell on u1 from every role, which §4.5.6 has
	// `cr record` refuse a record against.
	require.NoError(t, held.Write(state.FileCoverage, []byte{}))
	require.NoError(t, held.Unlock())

	first := fanoutOf(t)
	require.Len(t, first.Prompts, 6)

	printed, err := runCLIPrinting(t, "note", fixtureIssue, "Load's error is logged upstream.",
		"--source", "chat", "--pr", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var noted struct {
		Note      note.Note          `json:"note"`
		Postdates []review.Postdated `json:"postdates"`
		Honesty   []string           `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &noted))
	emitted, err := review.ReadEmissions(layout, fixtureOwner, fixtureProject, fixturePRNumber, 1)
	require.NoError(t, err)
	require.Len(t, emitted, 6, "one line per prompt of the one run")
	assert.Equal(t, fixtureIssue+"#n2", noted.Note.ID)
	assert.Equal(t, []review.Postdated{{
		Round: 1, Head: first.Head, Pass: review.PassAll, EmittedAt: emitted[0].EmittedAt,
		Roles: []string{"convention", "correctness", "intent-coverage"}, Prompts: 6,
	}}, noted.Postdates)
	assert.Equal(t, []string{"§9.3.1: round 1 was opened at head " + first.Head + ", which is still the pull request's current head"},
		noted.Honesty)

	stale := recordAtFirstID(t, promptFor(t, &first, "correctness", "u1"))
	assert.Equal(t, review.NotesAfterPrompts{
		Count:        1,
		Records:      []review.RecordBeforeNotes{{Record: promptFor(t, &first, "correctness", "u1").FirstID, Notes: []string{noted.Note.ID}}},
		Unattributed: []string{},
	}, stale)

	again := fanoutOf(t)
	fresh := recordAtFirstID(t, promptFor(t, &again, "convention", "u2"))
	assert.Equal(t, review.NotesAfterPrompts{Records: []review.RecordBeforeNotes{}, Unattributed: []string{}}, fresh,
		"a record written after the prompt was emitted again carrying the note is reported against nothing")

	printed, err = runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var status struct {
		NotesAfterPrompts review.NotesAfterPrompts `json:"notes_after_prompts"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &status))
	assert.Equal(t, stale, status.NotesAfterPrompts, "cr status counts the round's records the same way")
	terminal := strings.Split(strings.ReplaceAll(throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug), "\r", ""), "\n")
	assert.Contains(t, terminal, notesAfterLines("", "", stale), "and says so at a terminal")

	// Twice: the first draft moves both records to `queued`, and the
	// regenerated header still dates each record by when `cr record` stored
	// it rather than by that later move. Then once more after the reviewer
	// deleted the reported record's block: the header reports the records the
	// draft holds, so it reports none.
	draftPath := layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft)
	headerOf := func() []string {
		t.Helper()
		_, err := runCLIPrinting(t, "draft", fixturePR, "--repo", fixtureSlug)
		require.NoError(t, err)
		body, err := os.ReadFile(draftPath)
		require.NoError(t, err)
		closing := bytes.Index(body, []byte("\n-->\n"))
		require.GreaterOrEqual(t, closing, 0, "the draft opens with its header")
		reported := make([]string, 0)
		for line := range strings.SplitSeq(string(body[:closing]), "\n") {
			if strings.HasPrefix(line, "notes after prompts: ") {
				reported = append(reported, line)
			}
		}
		return reported
	}
	for range 2 {
		assert.Equal(t, []string{
			"notes after prompts: 1 record(s) came from a prompt emitted before a standing note on their claim or unit; " +
				"read the note before keeping the block: " + stale.Records[0].Record + " (" + noted.Note.ID + ")",
		}, headerOf(), "the record written after the note was carried is in no line")
	}
	body, err := os.ReadFile(draftPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(draftPath, []byte(deleteBlock(t, string(body), stale.Records[0].Record)), 0o600))
	assert.Equal(t, []string{}, headerOf(), "a record the draft no longer holds is not reported in its header")

	printed, err = runCLIPrinting(t, "note", fixtureIssue, "A fact typed anywhere.", "--source", "chat", "--pr", fixturePR)
	require.NoError(t, err)
	var unnamed struct {
		Postdates []review.Postdated `json:"postdates"`
		Honesty   []string           `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &unnamed))
	assert.Equal(t, []review.Postdated{}, unnamed.Postdates, "with no repository there is no round to read")
	assert.Equal(t, []string{}, unnamed.Honesty)
}

// The terminal lines of field-feedback 1.5's report name each record with its
// notes and each record with no emission to compare, and are empty for a report
// holding neither.
func TestTheNotesAfterPromptsLinesNameTheRecordsAndTheirNotes(t *testing.T) {
	report := review.NotesAfterPrompts{
		Count: 2,
		Records: []review.RecordBeforeNotes{
			{Record: "f3", Notes: []string{"CR-7#n2"}},
			{Record: "f4", Notes: []string{"CR-7#n2", "CR-7#n3"}},
		},
		Unattributed: []string{"f5"},
	}
	assert.Equal(t,
		"<2 record(s) came from a prompt emitted before a standing note on their claim or unit: "+
			"f3 (CR-7#n2), f4 (CR-7#n2, CR-7#n3)>"+
			"<1 record(s) follow no recorded prompt emission of their role and unit, so whether a note "+
			"postdates their prompt is unknown: f5>",
		notesAfterLines("<", ">", report))
	assert.Empty(t, notesAfterLines("<", ">",
		review.NotesAfterPrompts{Records: []review.RecordBeforeNotes{}, Unattributed: []string{}}))

	var printed bytes.Buffer
	require.NoError(t, (&writer{out: &printed, mode: ModeText}).emit(
		&recordResult{Recorded: []*finding.Finding{}, NotesAfterPrompts: report}))
	assert.Equal(t, "recorded 0 in state draft"+notesAfterLines("\n", "", report),
		strings.TrimRight(printed.String(), "\n"), "cr record prints them under its first line")
}

// A terminal `cr note` names every pass the note postdates with the command that
// emits it again, `--axis` included for an axis pass, and then §9.3.1's line.
func TestATerminalNoteNamesThePassesItPostdatesAndHowToEmitThemAgain(t *testing.T) {
	result := &noteResult{
		Note: note.Note{ID: "CR-7#n2", Source: note.SourceChat, PR: 7},
		Postdates: []review.Postdated{
			{Round: 1, Head: "abc", Pass: review.PassAll, EmittedAt: time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC),
				Roles: []string{"convention", "correctness"}, Prompts: 4},
			{Round: 1, Head: "abc", Pass: "intent", EmittedAt: time.Date(2026, 9, 16, 11, 0, 0, 0, time.UTC),
				Roles: []string{"intent-coverage"}, Prompts: 2},
		},
		Honesty: []string{"§9.3.1: round 1 is at head abc, which is the pull request's current head"},
		repo:    "octocat/hello",
	}
	var printed bytes.Buffer
	require.NoError(t, (&writer{out: &printed, mode: ModeText}).emit(result))
	assert.Equal(t, strings.Join([]string{
		"recorded CR-7#n2 from chat",
		"CR-7#n2 postdates pass all of round 1 emitted at 2026-09-16T10:00:00Z: 4 prompt(s) for convention, " +
			"correctness did not carry it; `cr review 7 --repo octocat/hello` emits them again with it",
		"CR-7#n2 postdates pass intent of round 1 emitted at 2026-09-16T11:00:00Z: 2 prompt(s) for intent-coverage " +
			"did not carry it; `cr review 7 --repo octocat/hello --axis intent` emits them again with it",
		"§9.3.1: round 1 is at head abc, which is the pull request's current head",
	}, "\n"), strings.TrimRight(printed.String(), "\n"))
}
