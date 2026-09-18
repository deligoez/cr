package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// anchoredFile is the file the fixture's anchor points into, and the lines the
// head holds it as.
var (
	anchoredPath  = aRecord("f1").Anchor.Path
	anchoredLines = []string{"package api", "func Handle() {", "}"}
)

// headHolding is a tree holding that one file, for §7.2's location row — the
// only row that opens one. Both sides read the same file, so a test does not
// have to choose a side to exercise the row.
func headHolding() finding.Trees {
	read := func(asked string) ([]string, bool, error) {
		if asked != anchoredPath {
			return nil, false, nil
		}
		return anchoredLines, true, nil
	}
	return finding.Trees{Head: read, MergeBase: read}
}

// markerMoved renders one record's block and returns the draft with one marker
// field moved, the way a reviewer types into it, beside its rendered.json.
func markerMoved(
	t *testing.T, record *finding.Finding, move func(*Marker),
) (file string, entries map[string]string) {
	t.Helper()
	file, entries = renderedRound(t, record)
	was := markerOf(record)
	now := was
	move(&now)
	edited := strings.Replace(file, was.String(), now.String(), 1)
	require.NotEqual(t, file, edited, "the fixture's marker is the line the edit lands on")
	return edited, entries
}

// readBack ingests a draft with the one tree §7.2's location row resolves
// against.
func readBack(records []*finding.Finding, file string, entries map[string]string) (Triage, error) {
	return Ingest(records, &Draft{
		Name: "draft.md", Body: file, Rendered: entries, Trees: headHolding(),
	})
}

// §7.2's edit-semantics table, one accept path per row.
//
// The `id` row's accept is the marker left as cr wrote it, which is the only
// thing that row admits; its abort is in the table below. `disposition` and the
// softening half of `kind` have accept paths of their own in ingest_test.go,
// where the verbs they belong to are read; what is asserted here is that each
// row leaves the record where §7.2 says it leaves it.
func TestEveryRowOfTheMarkerEditTableAcceptsWhatItAdmits(t *testing.T) {
	t.Run("id as cr wrote it asks for nothing", func(t *testing.T) {
		record := aRecord("f1")
		file, entries := renderedRound(t, record)

		triage, err := readBack([]*finding.Finding{record}, file, entries)
		require.NoError(t, err)

		assert.Empty(t, triage.Retriaged)
		assert.Empty(t, triage.Softened)
		assert.Empty(t, triage.Hardened)
		assert.Equal(t, finding.OutcomeKept, triage.Outcome(record))
	})

	t.Run("kind hardens a question on a cited grade", func(t *testing.T) {
		asked := aRecord("f1")
		asked.Kind = finding.KindQuestion
		asked.Summary = "Does the caller ever see the error Decode returns?"
		file, entries := markerMoved(t, asked, func(m *Marker) { m.Kind = string(finding.KindFinding) })

		triage, err := readBack([]*finding.Finding{asked}, file, entries)
		require.NoError(t, err)

		assert.Equal(t, []*finding.Finding{asked}, triage.Hardened)
		assert.Equal(t, finding.OutcomeKept, triage.Outcome(asked),
			"§7.3.1 closes the outcome vocabulary at five, and hardening is not one of them")
	})

	t.Run("the location is re-validated and the content hash recomputed", func(t *testing.T) {
		record := aRecord("f1")
		file, entries := markerMoved(t, record, func(m *Marker) { m.StartLine, m.Line = 2, 3 })

		triage, err := readBack([]*finding.Finding{record}, file, entries)
		require.NoError(t, err)

		require.Len(t, triage.Retriaged, 1)
		moved := triage.Retriaged[0].Anchor
		require.NotNil(t, moved)
		assert.Equal(t, 2, moved.StartLine)
		assert.Equal(t, 3, moved.Line)
		hash, err := finding.AnchorContentHash(anchoredLines[1:])
		require.NoError(t, err)
		assert.Equal(t, hash, moved.ContentHash,
			"round 12: §9.2's hash is recomputed over the lines the anchor now names")
		assert.NotEqual(t, record.Anchor.ContentHash, moved.ContentHash,
			"and is not the hash of the lines it left, which §7.4.1 would key a waiver on")
	})

	t.Run("severity is freely editable", func(t *testing.T) {
		record := aRecord("f1")
		file, entries := markerMoved(t, record, func(m *Marker) { m.Severity = string(finding.SeverityLow) })

		triage, err := readBack([]*finding.Finding{record}, file, entries)
		require.NoError(t, err)

		require.Len(t, triage.Retriaged, 1)
		assert.Equal(t, finding.SeverityLow, triage.Retriaged[0].Severity)
		assert.Nil(t, triage.Retriaged[0].Anchor, "one row moved, not two")
	})

	t.Run("disposition wrong discards", func(t *testing.T) {
		record := aRecord("f1")
		file, entries := markerMoved(t, record,
			func(m *Marker) { m.Disposition = string(finding.DispositionWrong) })

		triage, err := readBack([]*finding.Finding{record}, file, entries)
		require.NoError(t, err)

		assert.Equal(t, []*finding.Finding{record}, triage.Wrong)
		assert.Equal(t, finding.OutcomeDiscardedWrong, triage.Outcome(record))
	})

	t.Run("grade is informational and ignored", func(t *testing.T) {
		record := aRecord("f1")
		file, entries := markerMoved(t, record, func(m *Marker) { m.Grade = "certain" })

		triage, err := readBack([]*finding.Finding{record}, file, entries)
		require.NoError(t, err)

		assert.Empty(t, triage.Retriaged,
			"§7.2: cr recomputes the grade per §6.2 and ignores whatever the marker holds")
		assert.Equal(t, finding.GradeCited, record.Grade, "the record keeps the grade cr computed")
	})
}

// §7.2's edit-semantics table, one abort path per row: exit code 1, naming the
// record id.
//
// The `id` row's abort is TestABlockOutsideTheQueuedRecordsAborts, where the
// block that carries it has no record to be read against.
func TestEveryRowOfTheMarkerEditTableAbortsOnWhatItRefuses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		record func() *finding.Finding
		move   func(*Marker)
		field  string
		says   string
	}{
		{
			name:  "kind names no register",
			move:  func(m *Marker) { m.Kind = "bug" },
			field: "kind", says: `"bug"`,
		},
		{
			name: "a question is hardened on an argued grade",
			record: func() *finding.Finding {
				argued := aRecord("f1")
				argued.Kind, argued.Grade = finding.KindQuestion, finding.GradeArgued
				return argued
			},
			move:  func(m *Marker) { m.Kind = string(finding.KindFinding) },
			field: "kind", says: "§6.3.3",
		},
		{
			name:  "the anchor no longer resolves",
			move:  func(m *Marker) { m.StartLine, m.Line = 90, 91 },
			field: "anchor", says: "holds 3 lines",
		},
		{
			name:  "the anchor names a file the tree does not hold",
			move:  func(m *Marker) { m.Path = "internal/api/gone.go" },
			field: "anchor", says: "gone.go",
		},
		{
			name:  "severity names no severity",
			move:  func(m *Marker) { m.Severity = "urgent" },
			field: "severity", says: "critical, high, medium, low",
		},
		{
			name:  "not-here is set by hand",
			move:  func(m *Marker) { m.Disposition = string(finding.DispositionNotHere) },
			field: "disposition", says: "delete the block to say it",
		},
		{
			name:  "disposition names no verb",
			move:  func(m *Marker) { m.Disposition = "maybe" },
			field: "disposition", says: `"maybe"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := aRecord("f1")
			if tc.record != nil {
				record = tc.record()
			}
			file, entries := markerMoved(t, record, tc.move)

			_, err := readBack([]*finding.Finding{record}, file, entries)

			var refused *MarkerEditError
			require.ErrorAs(t, err, &refused, "§7.2 admits no such edit")
			assert.Equal(t, tc.field, refused.Field)
			assert.Equal(t, record.ID, refused.ID, "§7.2: the abort names the record id")
			assert.Contains(t, refused.Error(), tc.says)
			assert.Contains(t, refused.Error(), record.ID)
		})
	}
}

// A marker asking for one thing §7.2 admits and one it does not is refused
// whole. The severity would have stood on its own; the disposition is cr's own
// word, so nothing the marker asks for is applied.
func TestOneRefusedRowRefusesTheWholeMarker(t *testing.T) {
	record := aRecord("f1")
	file, entries := markerMoved(t, record, func(m *Marker) {
		m.Severity = string(finding.SeverityLow)
		m.Disposition = string(finding.DispositionNotHere)
	})

	triage, err := readBack([]*finding.Finding{record}, file, entries)

	var refused *MarkerEditError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "disposition", refused.Field)
	assert.Empty(t, triage.Retriaged, "the severity beside it is not applied")
	assert.Equal(t, finding.SeverityHigh, record.Severity)
}
