package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// renderedRound is a first rendering of records: the draft.md cr wrote and the
// rendered.json entries beside it, which is what Ingest reads back.
func renderedRound(t *testing.T, records ...*finding.Finding) (file string, entries map[string]string) {
	t.Helper()
	file, err := File(records, render.LangEN, nil, nil, headerFacts)
	require.NoError(t, err)
	entries, err = Rendered(records, render.LangEN, nil)
	require.NoError(t, err)
	return file, entries
}

// fourRecords are four queued records whose bodies differ, so an edit to one
// cannot land in another's block by accident.
func fourRecords() []*finding.Finding {
	records := make([]*finding.Finding, 0, 4)
	for _, id := range []string{"f1", "f2", "f3", "f4"} {
		record := aRecord(id)
		record.Summary = "The error Decode returns is dropped in " + id + "."
		records = append(records, record)
	}
	return records
}

// §7.1.6 over the four cases the criterion names, read by Ingest alone: an
// untouched block, a whitespace-only edit, a substantive edit, and a deletion.
//
// The comparison is byte-exact, so the whitespace-only edit — two trailing
// spaces, which Markdown reads as a line break — is an edit like any other and
// its body is kept exactly as typed, spaces included. Under a normalising
// comparison it would read as untouched and the next rendering would take the
// spaces away, and which reading is right is the reviewer's to say rather than
// cr's.
func TestIngestReadsTheFourCasesOfSection716(t *testing.T) {
	records := fourRecords()
	file, entries := renderedRound(t, records...)

	whitespace := strings.Replace(entries["f2"], "f2.", "f2.  ", 1)
	substantive := "The reviewer's own wording: Decode's error never reaches the caller."
	file = strings.Replace(file, entries["f2"], whitespace, 1)
	file = strings.Replace(file, entries["f3"], substantive, 1)
	file = withoutBlock(t, file, "f4")

	triage, err := Ingest(records, file, entries)
	require.NoError(t, err)

	assert.Equal(t, []*finding.Finding{records[3]}, triage.Deleted, "the deleted block is the one discard")
	assert.Equal(t, map[string]string{"f2": whitespace, "f3": substantive}, triage.Preserved,
		"both edits are kept exactly as typed, and the untouched block is not")
}

// withoutBlock deletes one record's block from a rendered draft, marker and
// body together, the way a reviewer deletes it.
func withoutBlock(t *testing.T, file, id string) string {
	t.Helper()
	start := strings.Index(file, `<!-- cr:record id="`+id+`"`)
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(file[start+1:], "<!-- cr:record ")
	if end < 0 {
		return file[:start]
	}
	return file[:start] + file[start+1+end:]
}

// §8.1.3: cr regenerates every owned region and discards edits inside them, so
// a reviewer who types into a question's label has not edited the body. Only
// the agent region is compared, and it is untouched here.
func TestAnEditInsideAnOwnedRegionIsNoEditOfTheBody(t *testing.T) {
	question := aRecord("f1")
	question.Kind, question.Grade = finding.KindQuestion, finding.GradeArgued
	question.Summary = "Does the caller ever see the error Decode returns?"
	file, entries := renderedRound(t, question)
	label, err := render.QuestionLabelRegion(render.LangEN, finding.GradeArgued)
	require.NoError(t, err)
	require.Contains(t, file, label)

	edited := strings.Replace(file, label,
		strings.Replace(label, "\n", "\nI would rather this said something else.\n", 1), 1)
	triage, err := Ingest([]*finding.Finding{question}, edited, entries)
	require.NoError(t, err)

	assert.Empty(t, triage.Preserved, "the typing sat inside a region cr owns")
	assert.Empty(t, triage.Deleted)
}
