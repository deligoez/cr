package draft

import (
	"fmt"
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

// ingested reads one draft back for the records it was rendered from, with no
// tree behind it. §7.2's location row is the only row that opens one, so a test
// that moves an anchor calls Ingest itself with the trees it wants read.
func ingested(records []*finding.Finding, file string, entries map[string]string) (Triage, error) {
	return Ingest(records, &Draft{Name: "draft.md", Body: file, Rendered: entries})
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

	triage, err := ingested(records, file, entries)
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
	triage, err := ingested([]*finding.Finding{question}, edited, entries)
	require.NoError(t, err)

	assert.Empty(t, triage.Preserved, "the typing sat inside a region cr owns")
	assert.Empty(t, triage.Deleted)
}

// A block with no rendered.json entry keeps its body. With nothing to compare
// against, cr cannot show the block is untouched, and keeping a body that
// happens to equal cr's own costs nothing where overwriting an edited one would
// destroy the reviewer's prose.
func TestABlockWithNoEntryKeepsItsBody(t *testing.T) {
	record := aRecord("f1")
	file, _ := renderedRound(t, record)

	triage, err := ingested([]*finding.Finding{record}, file, map[string]string{})
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"f1": body(record)}, triage.Preserved)
}

// drift counts the location, severity and grade fields of a marker that read
// otherwise than the record's own, one per field, so a record's own marker
// drifts by none.
func TestDriftCountsEachFieldThatDiffers(t *testing.T) {
	record := aRecord("f1")
	own := markerOf(record)
	assert.Zero(t, drift(&own, record))
	for field, edit := range map[string]func(*Marker){
		"path":       func(m *Marker) { m.Path = "other.go" },
		"side":       func(m *Marker) { m.Side = "LEFT" },
		"start_line": func(m *Marker) { m.StartLine++ },
		"line":       func(m *Marker) { m.Line++ },
		"severity":   func(m *Marker) { m.Severity = string(finding.SeverityLow) },
		"grade":      func(m *Marker) { m.Grade = string(finding.GradeArgued) },
	} {
		edited := own
		edit(&edited)
		assert.Equal(t, 1, drift(&edited, record), field)
	}
}

// lineOf is the one-based line of a draft on which needle first appears.
func lineOf(t *testing.T, file, needle string) int {
	t.Helper()
	at := strings.Index(file, needle)
	require.GreaterOrEqual(t, at, 0)
	return strings.Count(file[:at], "\n") + 1
}

// A marker the reviewer mistyped stops the ingest, naming its line, rather than
// being read as prose — which would take the record behind it for deleted and
// discard it with a waiver nobody asked for.
func TestAMalformedMarkerStopsTheIngestNamingItsLine(t *testing.T) {
	record := aRecord("f1")
	file, entries := renderedRound(t, record)
	broken := strings.Replace(file, `kind="finding"`, `kind=finding`, 1)

	triage, err := ingested([]*finding.Finding{record}, broken, entries)

	var malformed *MalformedMarkerError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, lineOf(t, broken, "<!-- cr:record "), malformed.At)
	assert.Empty(t, triage.Deleted, "nothing is discarded on the strength of a draft cr could not read")
}

// A second block for one record stops the ingest, naming the second marker's
// line and the first's. One record is one block, so two bodies under one id
// leave no single answer to what the reviewer wrote, and cr does not pick one.
func TestASecondBlockForOneRecordStopsTheIngest(t *testing.T) {
	record := aRecord("f1")
	file, entries := renderedRound(t, record)
	first := lineOf(t, file, "<!-- cr:record ")
	marker := markerOf(record).String()
	doubled := file + "\n" + marker + "\n\nA pasted copy of the block.\n"

	_, err := ingested([]*finding.Finding{record}, doubled, entries)

	var malformed *MalformedMarkerError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, strings.Count(file, "\n")+2, malformed.At, "the second marker is the one named")
	assert.Contains(t, malformed.Problem, fmt.Sprintf("line %d", first), "and the first is named beside it")
}

// §7.2's `id` row: a block naming a record the draft was not rendered for
// aborts, naming the id, rather than being adopted.
//
// One refusal answers two obligations. It keeps a deleted block from being
// resurrected by pasting it back, since a discarded record is no longer among
// the queued; and it is §7.2.3's own, which gives v0.3 no manual-comment
// channel because a hand-written block carries no role, axis or grade cr
// computed.
func TestABlockOutsideTheQueuedRecordsAborts(t *testing.T) {
	record, discarded := aRecord("f1"), aRecord("f9")
	file, entries := renderedRound(t, record)
	pasted := file + "\n" + markerOf(discarded).String() + "\n\n" + body(discarded) + "\n"

	_, err := ingested([]*finding.Finding{record}, pasted, entries)

	var refused *MarkerEditError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "f9", refused.ID, "the abort names the id the round does not hold")
	assert.Equal(t, "id", refused.Field)
	assert.Contains(t, refused.Error(), "f9")
}

// Of two blocks naming records the round does not hold, the one nearer the top
// is named, whatever order the blocks are read back in. §2.1.1 has one draft
// give one refusal, and the earlier block is the one a reviewer working down
// the draft meets first. f9 is placed above f8 so the answer is not the ids'
// own order either.
func TestTheEarliestUnknownBlockIsTheOneNamed(t *testing.T) {
	record, upper, lower := aRecord("f1"), aRecord("f9"), aRecord("f8")
	file, entries := renderedRound(t, record)
	pasted := file + "\n" + markerOf(upper).String() + "\n\n" + body(upper) + "\n" +
		markerOf(lower).String() + "\n\n" + body(lower) + "\n"

	_, err := ingested([]*finding.Finding{record}, pasted, entries)

	var refused *MarkerEditError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "f9", refused.ID, "the upper of the two unknown blocks is named")
	assert.Equal(t, lineOf(t, pasted, markerOf(upper).String()), refused.At)
}

// §7.2: `disposition=wrong` in the marker discards the record as a false
// positive whether or not the body remains. f1 keeps its body untouched, so the
// reviewer did not have to delete text to say it; f2's body is gone; f3 was
// also softened, and the discard is what the record comes to. None of the three
// has a body kept for it, since none is rendered again.
func TestAWrongMarkerDiscardsWhetherOrNotTheBodyRemains(t *testing.T) {
	records := fourRecords()[:3]
	file, entries := renderedRound(t, records...)
	for _, record := range records {
		marker := markerOf(record)
		marked := marker
		marked.Disposition = string(finding.DispositionWrong)
		if record.ID == "f3" {
			marked.Kind = string(finding.KindQuestion)
		}
		file = strings.Replace(file, marker.String(), marked.String(), 1)
	}
	file = strings.Replace(file, entries["f2"], "", 1)

	triage, err := ingested(records, file, entries)
	require.NoError(t, err)

	assert.Equal(t, records, triage.Wrong)
	assert.Empty(t, triage.Softened, "a wrong discard is what the record comes to")
	assert.Empty(t, triage.Preserved)
	for _, record := range records {
		assert.Equal(t, finding.OutcomeDiscardedWrong, triage.Outcome(record), record.ID)
	}
}

// §7.2: changing `kind=finding` to `kind=question` softens the record. The same
// marker on a record that already was a question asks for nothing and is kept,
// so only a change the reviewer made is read as one.
func TestAQuestionMarkerOnAFindingSoftensIt(t *testing.T) {
	asserted, asked := aRecord("f1"), aRecord("f2")
	asked.Kind, asked.Grade = finding.KindQuestion, finding.GradeArgued
	asked.Summary = "Does the caller ever see the error Decode returns?"
	file, entries := renderedRound(t, asserted, asked)
	marker := markerOf(asserted)
	softened := marker
	softened.Kind = string(finding.KindQuestion)
	file = strings.Replace(file, marker.String(), softened.String(), 1)

	triage, err := ingested([]*finding.Finding{asserted, asked}, file, entries)
	require.NoError(t, err)

	assert.Equal(t, []*finding.Finding{asserted}, triage.Softened)
	assert.Equal(t, finding.OutcomeSoftened, triage.Outcome(asserted))
	assert.Equal(t, finding.OutcomeKept, triage.Outcome(asked))
	assert.Empty(t, triage.Preserved, "a marker edit alone is no edit of the body")
}
