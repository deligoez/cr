package draft

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/finding"
)

// MarkerEditError reports a marker edit §7.2's table does not admit.
//
// §7.2 gives all of them one answer — abort with exit code 1, naming the record
// id — so they share one type, exactly as §6.1.3's record rejections do. The
// draft line is carried beside the id because the reviewer's next step is to
// open the file at it: the id says which record cr refused, and the line says
// where the sentence they typed is.
//
// It is a separate type from MalformedMarkerError, which the same file raises,
// because the two say different things to the same reader. A malformed marker
// is a line that is not a marker at all, and the answer is the grammar; an edit
// refused here is a well-formed marker asking for something §7.2 does not
// admit, and the answer is the row.
type MarkerEditError struct {
	// ID is the record whose marker carries the edit.
	ID string
	// At is the one-based line the marker sits on in the draft.
	At int
	// Field is the marker field at fault, named as §7.1.1 names it.
	Field string
	// Problem completes the sentence saying why §7.2 does not admit it,
	// and ends by naming the reviewer's way of saying what they meant.
	Problem string
}

func (e *MarkerEditError) Error() string {
	return fmt.Sprintf("draft line %d, record %s: %s %s", e.At, e.ID, e.Field, e.Problem)
}

// markerEdit is one block's marker read against the record cr rendered it from:
// everything §7.2's table needs to judge what the reviewer typed.
//
// `was` is markerOf(record) rather than the line cr last wrote, and the
// difference reaches exactly one field. A softened record stays a finding in
// findings.ndjson and is rendered as a question, for the reason ingestDraft
// gives, so `was.Kind` is the stored kind and the softening is read back out of
// the file on every run rather than once. Every other field is rendered from
// the record unchanged, so the two readings agree.
type markerEdit struct {
	record *finding.Finding
	was    Marker
	now    Marker
	at     int
}

// refuse reports one row of §7.2's table declining what the marker asks.
func (e *markerEdit) refuse(field, problem string) error {
	return &MarkerEditError{ID: e.record.ID, At: e.at, Field: field, Problem: problem}
}

// disposition is §7.2's `disposition` row.
//
// `wrong` discards the record as a false positive whatever its body holds.
// `not-here` is refused: §7.2 has cr write it when a block is deleted, and a
// reviewer who could type it would be writing cr's own word for a verb the file
// already has — deletion — while the two carry waivers of different scope and
// count differently in §7.3.4.
//
// Any other value names no verb at all, which is §7.2's "any other marker edit
// MUST abort".
func (e *markerEdit) disposition() (wrong bool, err error) {
	switch finding.Disposition(e.now.Disposition) {
	case finding.Disposition(e.was.Disposition):
		return false, nil
	case finding.DispositionWrong:
		return true, nil
	case finding.DispositionNotHere:
		return false, e.refuse("disposition", fmt.Sprintf(
			"reads %q, which cr writes itself when a block is deleted; delete the block to say it",
			finding.DispositionNotHere))
	}
	return false, e.refuse("disposition", fmt.Sprintf(
		"reads %q, and the one disposition §7.2 admits by hand is %q; delete the block for the other",
		e.now.Disposition, finding.DispositionWrong))
}

// kind is §7.2's `kind` row, and with it its `grade` row.
//
// The two rows are one branch because they ask one question. §7.2 makes `grade`
// informational — cr recomputes it per §6.2 and ignores whatever the marker
// holds — and leaves it exactly one abort, when the marker's kind contradicts
// the recomputed grade per §6.3.3. A kind can only contradict a grade by
// asserting, and a block cr rendered is already in the register §6.3 left it
// in, so the contradiction is reachable only through this row: the marker's own
// grade is never read, and cr's is read once.
//
// Softening needs no grade. §6.3 forces down and never up, and a reviewer
// asking a question of a record cr would have let assert is making the
// judgement §2.1.3 leaves to them.
func (e *markerEdit) kind() (asked finding.Kind, retyped bool, err error) {
	asked = finding.Kind(e.now.Kind)
	if asked != finding.KindFinding && asked != finding.KindQuestion {
		return "", false, e.refuse("kind", fmt.Sprintf(
			"reads %q, and §6.1's register is %q or %q",
			e.now.Kind, finding.KindFinding, finding.KindQuestion))
	}
	if asked == finding.Kind(e.was.Kind) {
		return asked, false, nil
	}
	if asked == finding.KindFinding && !asserts(e.record.Grade) {
		return "", false, e.refuse("kind", fmt.Sprintf(
			"asks for %q on a record cr graded %q, and §6.3.3 admits that register only on %q or %q; "+
				"leave it a %q or give the record an experiment",
			finding.KindFinding, e.record.Grade,
			finding.GradeProbed, finding.GradeCited, finding.KindQuestion))
	}
	return asked, true, nil
}

// asserts reports whether §6.2's grade lets a record reach the author as an
// assertion, which §6.3 leaves to `probed` and `cited` alone. A record cr never
// graded is neither, and is treated as the weakest thing it could be rather
// than as an exemption.
func asserts(grade finding.Grade) bool {
	return grade == finding.GradeProbed || grade == finding.GradeCited
}

// severity is §7.2's `severity` row: freely editable, within §6.1's four.
func (e *markerEdit) severity() (finding.Severity, error) {
	asked := finding.Severity(e.now.Severity)
	if asked == finding.Severity(e.was.Severity) {
		return "", nil
	}
	if !slices.Contains(finding.Severities(), asked) {
		return "", e.refuse("severity", fmt.Sprintf(
			"reads %q, and §6.1's four are %s", e.now.Severity, listedSeverities()))
	}
	return asked, nil
}

// listedSeverities names §6.1's four in the order finding.Severities holds
// them, so a refusal tells the reviewer what to write rather than only what is
// wrong.
func listedSeverities() string {
	names := make([]string, 0, len(finding.Severities()))
	for _, severity := range finding.Severities() {
		names = append(names, string(severity))
	}
	return strings.Join(names, ", ")
}

// anchor is §7.2's `path`, `start_line` and `line` row: re-validated per
// §6.1.2, aborting when the anchor no longer resolves.
//
// It returns nil when the marker leaves the location where cr wrote it, which
// is what keeps `cr draft` from opening the repository — or, for a LEFT anchor,
// the pull request — on a run where nothing moved.
func (e *markerEdit) anchor(trees finding.Trees, file string) (*finding.Anchor, error) {
	if e.now.Path == e.was.Path && e.now.StartLine == e.was.StartLine && e.now.Line == e.was.Line {
		return nil, nil
	}
	moved := e.record.Anchor
	moved.Path, moved.StartLine, moved.Line = e.now.Path, e.now.StartLine, e.now.Line
	if err := finding.StampAnchor(trees, file, e.at, &moved); err != nil {
		// A location the tree cannot answer for is this row's abort and
		// carries the record id with it. A git that refuses is not: it
		// is §3.1.3's external command failure, and rewriting it here
		// would code an unreadable repository 1 and tell the reviewer
		// to edit a marker that is fine.
		var rejected *finding.RejectedRecordError
		if errors.As(err, &rejected) {
			return nil, e.refuse("anchor", rejected.Problem)
		}
		return nil, err
	}
	return &moved, nil
}
