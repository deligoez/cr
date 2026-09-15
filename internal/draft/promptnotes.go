package draft

import (
	"fmt"
	"strings"
)

// PromptNote is one queued record written from a prompt emitted before the
// notes named were recorded (field-feedback 1.5).
type PromptNote struct {
	// Record is the record's id.
	Record string
	// Notes are the ids of the standing notes on its claim or unit that
	// were recorded after its prompt and that the prompt did not carry.
	Notes []string
}

// WithPromptNotes adds field-feedback 1.5's report to the summary header of a
// draft File rendered: the queued records whose prompt a standing note on their
// claim or unit postdates, and the queued records no recorded prompt emission
// precedes. A header with neither is returned as it was.
//
// The report goes in the header and never in a block. §8.1.3 fixes a posted
// comment as the label, provenance, agent body and evidence regions, so a
// cr-owned line inside a block would be a region §8.1.3 does not have, while
// §7.1.4's header is not posted. The lines are placed before the header's
// closing line, which is the first line of the file that is headerClose alone:
// the header opens the file and none of its own lines is that.
func WithPromptNotes(file string, notes []PromptNote, unattributed []string) string {
	lines := promptNoteLines(notes, unattributed)
	if len(lines) == 0 {
		return file
	}
	closing := "\n" + headerClose + "\n"
	at := strings.Index(file, closing)
	if at < 0 {
		return file
	}
	return file[:at] + "\n" + strings.Join(lines, "\n") + file[at:]
}

// promptNoteLines is the report as header lines, and none when it holds no
// record.
func promptNoteLines(notes []PromptNote, unattributed []string) []string {
	lines := make([]string, 0, 2)
	if len(notes) > 0 {
		named := make([]string, 0, len(notes))
		for _, entry := range notes {
			named = append(named, entry.Record+" ("+strings.Join(entry.Notes, ", ")+")")
		}
		lines = append(lines, fmt.Sprintf(
			"notes after prompts: %d record(s) came from a prompt emitted before a standing note on their "+
				"claim or unit; read the note before keeping the block: %s",
			len(notes), strings.Join(named, ", ")))
	}
	if len(unattributed) > 0 {
		lines = append(lines, fmt.Sprintf(
			"notes after prompts: %d record(s) follow no recorded prompt emission of their role and unit, "+
				"so whether a note postdates their prompt is unknown: %s",
			len(unattributed), strings.Join(unattributed, ", ")))
	}
	return lines
}
