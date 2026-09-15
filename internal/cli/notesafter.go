package cli

import (
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// notesAfterLines renders field-feedback 1.5's report for a terminal, each line
// between before and after, and nothing when the report holds no record.
//
// The records a note postdates are named with the notes, because the reader's
// next step is to open those records and read those notes. The records with no
// emission to compare are named too: a report that stayed silent about them
// would read as a round no note postdates.
func notesAfterLines(before, after string, report review.NotesAfterPrompts) string {
	var out strings.Builder
	if report.Count > 0 {
		named := make([]string, 0, len(report.Records))
		for _, entry := range report.Records {
			named = append(named, entry.Record+" ("+strings.Join(entry.Notes, ", ")+")")
		}
		out.WriteString(before + strconv.Itoa(report.Count) +
			" record(s) came from a prompt emitted before a standing note on their claim or unit: " +
			strings.Join(named, ", ") + after)
	}
	if len(report.Unattributed) > 0 {
		out.WriteString(before + strconv.Itoa(len(report.Unattributed)) +
			" record(s) follow no recorded prompt emission of their role and unit, so whether a note " +
			"postdates their prompt is unknown: " + strings.Join(report.Unattributed, ", ") + after)
	}
	return out.String()
}

// roundNotesAfter is field-feedback 1.5's report over every record of the
// round, as `cr status` counts it.
func roundNotesAfter(l state.Layout, round *state.Meta) (review.NotesAfterPrompts, error) {
	stored, err := roundFindingsOf(l, round.Owner, round.Repo, round.PR, round.Round)
	if err != nil {
		return review.NotesAfterPrompts{}, err
	}
	return review.NotesAfterOf(l, round, stored)
}

// withPromptNotes is the draft file with field-feedback 1.5's report over its
// queued records added to its header. The report is computed over the round's
// records and narrowed to the queued ones, so a note answering a record the
// draft no longer holds still ties that note to its unit.
func withPromptNotes(l state.Layout, round *state.Meta, queued []*finding.Finding, file string) (string, error) {
	report, err := roundNotesAfter(l, round)
	if err != nil {
		return "", err
	}
	ids := make([]string, 0, len(queued))
	for _, record := range queued {
		ids = append(ids, record.ID)
	}
	report = report.Only(ids)
	notes := make([]draft.PromptNote, 0, report.Count)
	for _, entry := range report.Records {
		notes = append(notes, draft.PromptNote{Record: entry.Record, Notes: entry.Notes})
	}
	return draft.WithPromptNotes(file, notes, report.Unattributed), nil
}
