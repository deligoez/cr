package draft

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// field-feedback 1.5's lines go into the header, before its closing line and
// after every line already there, and a report holding no record leaves the
// file exactly as it was.
func TestPromptNotesGoIntoTheHeaderBeforeItsClosingLine(t *testing.T) {
	file := headerOpen + "\nrecords: 2 queued\n" + headerClose + "\n\n<!-- cr:record id=f3 -->\nbody\n-->\n"

	assert.Equal(t, headerOpen+"\nrecords: 2 queued\n"+
		"notes after prompts: 2 record(s) came from a prompt emitted before a standing note on their claim or "+
		"unit; read the note before keeping the block: f3 (CR-7#n2), f4 (CR-7#n2, CR-7#n3)\n"+
		"notes after prompts: 1 record(s) follow no recorded prompt emission of their role and unit, so "+
		"whether a note postdates their prompt is unknown: f5\n"+
		headerClose+"\n\n<!-- cr:record id=f3 -->\nbody\n-->\n",
		WithPromptNotes(file,
			[]PromptNote{{Record: "f3", Notes: []string{"CR-7#n2"}}, {Record: "f4", Notes: []string{"CR-7#n2", "CR-7#n3"}}},
			[]string{"f5"}))

	assert.Equal(t, file, WithPromptNotes(file, []PromptNote{}, []string{}))
}
