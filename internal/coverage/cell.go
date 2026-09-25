// Package coverage models the coverage cell of spec/0.1.0.md §4.5.5.
//
// A cell is §1.3's "intersection of one unit and one active role", and it is
// what P6 rests on: coverage is proven when every unit times every active role
// is a filled cell, so a cell is the unit of evidence that a lens looked at a
// piece of code. §10.2.2 counts them and §10.1.1 reports the ones that are
// missing.
//
// Cells are the agent's judgement. §4.5.6 has them reach cr through
// `cr cells record` exactly as findings reach it through `cr record`, and
// invariant 1 leaves cr no way to form one: this package decodes, validates,
// and refuses, and nothing here fills in a verdict.
//
// That is why the type has two constructors and no more. state.DecodeStamped
// allocates every Cell out of a line an agent wrote, except the two derived.go
// builds from a fact other than cr's reading of the code — §4.6.7's kind and
// §4.6.8's twin — and TestNothingBuildsACoverageCellOutsideItsDecoder fences
// it: §4.5.6 forbids cr to invent a cell for a unit no role reported on.
package coverage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/testadequacy"
)

// The four values §4.5.5 closes a cell's verdict at.
//
// They are the whole of what a role may say about a unit, and the set is closed
// for the reason §1.5 closes the axis ids: §10.1.1 counts cells by what they
// say, and a fifth value would be counted by nothing.
const (
	// ResultPass is a role that looked and found nothing to raise.
	ResultPass = "pass"
	// ResultFinding is a role that raised a finding on the unit.
	ResultFinding = "finding"
	// ResultQuestion is a role that raised a question on it.
	ResultQuestion = "question"
	// ResultNA is a role that had nothing to say about this unit, which
	// §4.5.5 requires a reason for.
	ResultNA = "na"
)

// results is the closed set, in the order §4.5.5 names them, so a rejection
// always lists them the same way.
var results = []string{ResultPass, ResultFinding, ResultQuestion, ResultNA}

// Cell is one line of coverage.ndjson: §4.5.5's fields, and the head and round
// §2.3.3 stamps onto every record of that file.
//
// The head is not a field of its own here because §2.3.3 already owns it: "the
// head it was filled against" is the embedded Stamp's, written by the writer
// and refused when an agent supplies it. `unit_hash` sits beside it because
// §10.2.2 compares a cell against "that unit's current unit hash", which is a
// different question from which commit the round is on — a unit can be
// re-clustered under an unchanged head.
type Cell struct {
	// Unit is the unit id of §3.4.6 this cell sits at.
	Unit string `json:"unit"`
	// Role is the active role of §4.5.1 that filled it.
	Role string `json:"role"`
	// Result is one of the four values above.
	Result string `json:"result"`
	// Reason completes an `na`, and §4.5.5 requires it there and nowhere
	// else.
	Reason string `json:"reason,omitempty"`
	// UnitHash is the §3.4.6 hash the cell was filled for, which §10.2.2
	// compares against the unit's current one. cr writes it, and refuses a
	// cell that supplied it.
	UnitHash string `json:"unit_hash"`
	// NoteID is the note that explained an unmapped unit, which §4.1.5 has
	// the cell cite so a suppressed question rests on something.
	NoteID string `json:"note_id,omitempty"`
	// Coverage is §4.4.1's classification, carried by a cell a role on the
	// `test` axis filled and by no other.
	//
	// It is internal/testadequacy's type rather than one of this package's,
	// because that is where §4.4.1 already lives: Attach supplies the test
	// files a classification rests on and Coverage records the verdict, and
	// its classification field is unexported so no code outside a decoded
	// agent line can put a value there. A second shape here would be a
	// second answer to §2.1.3's "whether a unit is covered by tests is the
	// agent's judgement", and this one would be the answer with the
	// structural fence missing.
	Coverage *testadequacy.Coverage `json:"coverage,omitempty"`
	state.Stamp
}

// RejectedCellError reports a cell §4.5 refuses.
//
// It takes the shape §3.3.1 and §6.1.3 already give a refused line: name the
// line so the user can open it, and name the field so they know what to fix.
// The cli layer maps it onto §11.2's code 1 — the file was found, read, and
// parsed, and what is wrong is the agent's data inside it.
type RejectedCellError struct {
	// File is the NDJSON file the agent handed the command.
	File string
	// Line is the one-based line the cell sits on, counting blank lines.
	Line int
	// Field is the field at fault, named by its JSON key.
	Field string
	// Problem is what is wrong with that field.
	Problem string
}

func (e *RejectedCellError) Error() string {
	return fmt.Sprintf("%s line %d: %s %s", e.File, e.Line, e.Field, e.Problem)
}

// DecodeInRound reads the cells an agent hands `cr cells record`, holding every
// line to §4.5.5's fields, to §4.5.6's rejection of an inactive role, to the
// authorship of the fields cr computes, and to §4.6.5's round standing.
//
// active is the round's active roles of §4.5.1, each carrying the axis it sits
// on. One argument answers both questions the decoder asks, and that is
// deliberate: §4.5.6 rejects a cell naming an inactive role, and §4.5.5 asks
// for `coverage` when the role is on the `test` axis, so a decoder given the
// role ids alone could enforce one and not the other — and one given a second,
// separate list of test roles could be handed two lists that disagree.
//
// raised is the current round's records by the seat each was raised at, which
// a `pass` or an `na` cell is held to; see consistent. unmapped is the round
// when its remaining axes still wait for the intent pass, and nil when they do
// not; see gated. notes is the context store a cell's `note_id` is held to;
// see cited.
//
// The checks run inside the decode rather than after it because
// state.DecodeStamped is the one place that counts lines, blank ones included,
// and every refusal here has to name the line the user must open.
func DecodeInRound(
	file string, body []byte, units []string, active []role.Role, raised Raised,
	unmapped *Unmapped, notes *Notes,
) ([]*Cell, error) {
	against := cellChecker{
		file: file, units: units, active: active, raised: raised, unmapped: unmapped, notes: notes,
	}
	seen := make(map[Seat]int)
	return state.DecodeStamped[Cell](file, body,
		func(line int, supplied map[string]json.RawMessage, cell *Cell) error {
			if err := against.check(line, supplied, cell); err != nil {
				return err
			}
			return refuseRepeatedSeat(file, seen, line, cell)
		})
}

// refuseRepeatedSeat refuses a cell at a `(unit, role)` an earlier line of the
// same file already filled, naming both lines, as `cr claims record` refuses a
// repeated claim id. seen maps each seat met so far to its line.
//
// §4.5.6 replaces the one cell a seat holds, so a file naming a seat twice asks
// for two verdicts where one is kept. Storing both would leave the seat saying
// two things at once, and §10.2.2 would count the row complete over a verdict
// nobody can read; keeping either would discard the other without a word.
func refuseRepeatedSeat(file string, seen map[Seat]int, line int, cell *Cell) error {
	seat := Seat{Unit: cell.Unit, Role: cell.Role}
	if first, repeated := seen[seat]; repeated {
		return &RejectedCellError{
			File: file, Line: line, Field: "unit",
			Problem: fmt.Sprintf(
				"%q and role %q repeat the seat of line %d; §4.5.6 replaces the one cell a "+
					"(unit, role) holds, so give each seat one cell in the file",
				cell.Unit, cell.Role, first),
		}
	}
	seen[seat] = line
	return nil
}

// Unmapped is a round whose intent axis is active and which holds no mapping
// yet: §4.6.5 refuses the remaining axes until the intent pass has recorded
// one for the round and head.
type Unmapped struct {
	// Round and Head are the round that holds no mapping.
	Round int
	Head  string
}

// MappingRequiredError reports a cell for a role off the intent axis, recorded
// in a round §4.6.5 has not yet let past its intent pass.
//
// It is `cr review`'s refusal of the same round, reached from the other end:
// the prompts for those roles are not emitted until a mapping exists, so a cell
// they filled was filled without them. The cli layer maps it onto §11.2's code
// 4, as it does `cr review`'s: the file is well-formed, and what refuses is
// where the round stands, which recording the mapping undoes.
type MappingRequiredError struct {
	// File is the NDJSON file the agent handed the command.
	File string
	// Line is the one-based line the cell sits on, counting blank lines.
	Line int
	// Role and Axis are the role the cell names and the axis it is on.
	Role string
	Axis string
	// Round and Head are the round that holds no mapping.
	Round int
	Head  string
}

func (e *MappingRequiredError) Error() string {
	return fmt.Sprintf(
		"%s line %d: role %q is on the %s axis, and round %d at head %s has no mapping; §4.6.5 "+
			"refuses the remaining axes until the intent pass has recorded one, so record the "+
			"mapping with `cr map record` and record this cell again",
		e.File, e.Line, e.Role, e.Axis, e.Round, e.Head)
}

// Seat is the `(unit, role)` a cell sits at, and the unit and role a record was
// raised at.
type Seat struct {
	Unit string
	Role string
}

// Raised names the ids of the current round's records at each seat, in the
// order findings.ndjson holds them. A seat no record was raised at is absent.
type Raised map[Seat][]string

// cellChecker holds what one file's cells are checked against: the file they
// arrived in, the units and active roles of the round, and the records the
// round holds, and whether §4.6.5 still holds the round's remaining axes back.
type cellChecker struct {
	file     string
	units    []string
	active   []role.Role
	raised   Raised
	unmapped *Unmapped
	notes    *Notes
}

// Notes is the context store of §3.6 a cell's `note_id` is held to: the issue
// key the round resolved, and that key's notes whole, as note.StandingOf
// requires.
type Notes struct {
	IssueKey string
	Stored   []note.Note
}

// check holds one line to §4.5.5 and to §4.5.6's role half.
//
// The order is the order a reader can act on. Authorship comes first: a cell
// carrying a field cr computes is refused whatever else it says, because the
// answer is to drop the field rather than to correct it. `role` is settled
// early because every later question is asked of the role it names: an inactive
// role has no axis, so there is no rule under which to decide whether
// `coverage` belongs on the line. `result` follows, because `reason` is
// conditional on it. Presence is read off the wire rather than off the decoded
// cell, for the reason state.DecodeStamped gives about head and round — `""` is
// a value the agent chose exactly as much as a sentence is.
//
// A key the cell schema does not define is refused right after authorship, for
// the same reason: the answer is to drop it. §4.5.6's round standing is asked
// once the role is known, because it is the role's axis that decides it.
func (c *cellChecker) check(line int, supplied map[string]json.RawMessage, cell *Cell) error {
	if err := c.computed(line, supplied); err != nil {
		return err
	}
	if err := c.fields(line, supplied); err != nil {
		return err
	}
	if err := c.unit(line, supplied, cell); err != nil {
		return err
	}
	filled, err := c.role(line, supplied, cell)
	if err != nil {
		return err
	}
	if err := c.gated(line, &filled); err != nil {
		return err
	}
	if err := c.result(line, supplied, cell); err != nil {
		return err
	}
	if err := c.cited(line, cell); err != nil {
		return err
	}
	if err := c.consistent(line, cell); err != nil {
		return err
	}
	return c.coverage(line, &filled, supplied, cell)
}

// cellFields are the keys §4.5.5 has the agent write on a cell line, in the
// order a rejection lists them. unit_hash, head and round are the cell's too,
// and cr writes them; they are refused before this is asked.
var cellFields = []string{"unit", "role", "result", "reason", "note_id", "coverage"}

// fields refuses a key the cell schema does not define, naming the first in
// sorted order so the same line is always refused by the same key.
//
// encoding/json drops such a key without a word, so a line carrying one would
// be stored as something other than what its author wrote: a `"comment"` the
// agent meant as the explanation of a `pass` would vanish, and the cell would
// read as an unqualified pass. §12.4 has cr name what it did not accept rather
// than accept part of a line.
func (c *cellChecker) fields(line int, supplied map[string]json.RawMessage) error {
	unknown := make([]string, 0)
	for key := range supplied {
		if !slices.Contains(cellFields, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	return &RejectedCellError{
		File: c.file, Line: line, Field: unknown[0],
		Problem: "is not a cell field; §4.5.5 has the agent write exactly " + listed(cellFields) +
			", and cr writes unit_hash, head and round itself",
	}
}

// gated is §4.6.5's refusal of the remaining axes, held at the cell a role on
// one of them filled.
//
// `cr review` does not emit their prompts until the intent pass has recorded a
// mapping, so a cell for one of those roles in a round with none was filled
// without the prompt §4.6.1 builds for it — the claims mapped to the unit
// included. Accepting it would let §10.2.2 count a row, and a round reach
// complete, over lenses that never ran against the round's intent. The intent
// axis's own cells pass: that pass is what produces the mapping.
func (c *cellChecker) gated(line int, filled *role.Role) error {
	if c.unmapped == nil || filled.Axis == axis.Intent {
		return nil
	}
	return &MappingRequiredError{
		File: c.file, Line: line, Role: filled.ID, Axis: filled.Axis,
		Round: c.unmapped.Round, Head: c.unmapped.Head,
	}
}

// cellComputed are the fields of §4.5.5 that cr writes onto a cell itself, in
// the order a rejection names them.
//
// `head` is not among them because state.DecodeStamped already refuses it for
// every one of §2.3.3's nine files. `unit_hash` is a cell's alone, so it is
// refused here: §3.4.6 fixes the value, `cr cells record` takes it from
// units.ndjson for the unit the cell names, and no other record carries it.
var cellComputed = []string{"unit_hash"}

// computed gives §4.5.5's computed field the treatment §6.1.4 gives `grade`: a
// cell arriving with one is rejected with exit code 1 naming the line and the
// field.
//
// §10.2.2 asks whether a cell "was filled for that unit's current unit hash",
// which makes the hash the guard's input and the thing being guarded at once.
// An agent free to write it could echo the current value onto a cell filled
// against older code, and a stale cell would then be indistinguishable from a
// fresh one — P6's "coverage is proven" read as coverage asserted. Overwriting
// a supplied value silently would close the same hole, but it would also accept
// a file whose author believed they were recording something, and §12.4 has cr
// name what it refused instead.
//
// It reports through the same state.ReservedFieldError that carries §2.3.3's
// head and round, so a cell's computed fields have one wording and one exit
// code. Presence alone is the test, as it is there: `"unit_hash": ""` is a key
// the agent wrote, and the field has the same author whatever value sits under
// it.
func (c *cellChecker) computed(line int, supplied map[string]json.RawMessage) error {
	for _, field := range cellComputed {
		if _, ok := supplied[field]; ok {
			return &state.ReservedFieldError{File: c.file, Line: line, Field: field}
		}
	}
	return nil
}

// unit holds one cell to §4.5.6's first rejection: an unknown unit id.
//
// The set is the units of the current round, which §3.7 makes `cr brief`'s to
// write. §3.4.6 scopes a unit id to its round and forbids carrying it across
// one, so `u1` of round 2 is a different piece of code from `u1` of round 1 —
// a cell checked against the whole of units.ndjson would be accepted at a unit
// this round never formed, and §10.2.2 would count it towards a row it does not
// belong to.
func (c *cellChecker) unit(line int, supplied map[string]json.RawMessage, cell *Cell) error {
	switch {
	case !written(supplied["unit"]):
		return &RejectedCellError{
			File: c.file, Line: line, Field: "unit",
			Problem: "is required by §4.5.5, which has every cell name the unit it sits at",
		}
	case !slices.Contains(c.units, cell.Unit):
		return &RejectedCellError{
			File: c.file, Line: line, Field: "unit",
			Problem: fmt.Sprintf(
				"names %q, which is not a unit of this round; §4.5.6 rejects a cell naming an "+
					"unknown unit id, and §3.4.6 scopes a unit id to the round that formed it, "+
					"which formed %s",
				cell.Unit, listed(c.units),
			),
		}
	}
	return nil
}

// role holds one cell to §4.5.6's second rejection and returns the role it
// named.
//
// §4.5.6 refuses "a cell naming an unknown unit id or an inactive role", and
// the two halves are one sentence for one reason: §10.2.2 counts a complete row
// of cells for every active role, so a cell filled by a role outside that set
// is evidence about a lens the completeness check will never ask after. It
// would raise the number of filled cells without raising the number of proven
// ones, which is P6 read backwards.
func (c *cellChecker) role(
	line int, supplied map[string]json.RawMessage, cell *Cell,
) (role.Role, error) {
	if !written(supplied["role"]) {
		return role.Role{}, &RejectedCellError{
			File: c.file, Line: line, Field: "role",
			Problem: "is required by §4.5.5, which has every cell name the role it sits at",
		}
	}
	at := slices.IndexFunc(c.active, func(r role.Role) bool { return r.ID == cell.Role })
	if at < 0 {
		return role.Role{}, &RejectedCellError{
			File: c.file, Line: line, Field: "role",
			Problem: fmt.Sprintf(
				"names %q, which is not an active role of this round; §4.5.6 rejects a cell "+
					"an inactive role filled, and §4.5.1 makes the active set %s",
				cell.Role, listed(activeIDs(c.active)),
			),
		}
	}
	return c.active[at], nil
}

// result holds one cell to §4.5.5's verdict and to the reason an `na` owes.
//
// Both directions are checked. An `na` with no reason is the case §4.5.5 states
// outright, and it is the one that matters: `na` is the only verdict that takes
// a cell out of what §10.2.2 can read as a lens having looked, so a bare `na`
// would shrink the proven coverage while looking exactly like a filled cell. A
// reason on any other verdict is the same fault mirrored — it is the word
// §4.5.5 attaches to `na` alone, and a `pass` carrying one reads as a qualified
// pass that nothing downstream qualifies.
func (c *cellChecker) result(line int, supplied map[string]json.RawMessage, cell *Cell) error {
	switch {
	case !written(supplied["result"]):
		return &RejectedCellError{
			File: c.file, Line: line, Field: "result",
			Problem: "is required by §4.5.5, which closes it at " + listed(results),
		}
	case !slices.Contains(results, cell.Result):
		return &RejectedCellError{
			File: c.file, Line: line, Field: "result",
			Problem: fmt.Sprintf("is %q; §4.5.5 closes it at %s", cell.Result, listed(results)),
		}
	}
	held := written(supplied["reason"])
	switch {
	case cell.Result == ResultNA && !held:
		return &RejectedCellError{
			File: c.file, Line: line, Field: "reason",
			Problem: "is required by §4.5.5 when result is " + ResultNA +
				", because an unexplained na is a lens that did not look",
		}
	case cell.Result != ResultNA && held:
		return &RejectedCellError{
			File: c.file, Line: line, Field: "reason",
			Problem: fmt.Sprintf(
				"is the explanation §4.5.5 attaches to %s alone, and this cell is a %s",
				ResultNA, cell.Result),
		}
	}
	return nil
}

// cited refuses a `note_id` naming no note of the round's issue key, or one
// §3.6.6 has retracted.
//
// §4.5.5 has a cell cite the note that explained an unmapped unit per §4.1.5,
// so the id is the explanation's provenance. An id the store holds no note for
// is an explanation nobody recorded, and `cr status` would report it dangling
// only after the round had counted the cell; a retracted one is a note §3.6.6
// already has re-evaluated, which `cr claims set-aside` refuses for the same
// reason. An empty `note_id` cites nothing and is passed over.
func (c *cellChecker) cited(line int, cell *Cell) error {
	if c.notes == nil || cell.NoteID == "" {
		return nil
	}
	if c.notes.IssueKey == "" {
		return &RejectedCellError{
			File: c.file, Line: line, Field: "note_id",
			Problem: fmt.Sprintf("names %q, and this round resolved no issue key, so §3.6 "+
				"holds no note for a cell to cite; drop the field", cell.NoteID),
		}
	}
	switch note.StandingOf(c.notes.Stored, cell.NoteID) {
	case note.StandingDangling:
		return &RejectedCellError{
			File: c.file, Line: line, Field: "note_id",
			Problem: fmt.Sprintf("names %q, which is no note of issue %s; §4.5.5 cites a note "+
				"of §3.6 for the round's issue key, and `cr context %s` lists them",
				cell.NoteID, c.notes.IssueKey, c.notes.IssueKey),
		}
	case note.StandingRetracted:
		return &RejectedCellError{
			File: c.file, Line: line, Field: "note_id",
			Problem: fmt.Sprintf("names %q, which §3.6.6 has retracted, so it explains "+
				"nothing; cite a note that stands, or drop the field", cell.NoteID),
		}
	}
	return nil
}

// consistent refuses a `pass` or an `na` cell at a seat where the current round
// holds a record from that role on that unit, naming the cell's line, unit and
// role and the records beside it.
//
// Without it a role could file a record and a `pass` for the same unit in the
// same round, and `cr status` would count a complete row with a clean verdict
// over a unit the role had found something on. An `na` there is the same
// contradiction in the other register: it says the role had nothing to say
// about the unit, beside a record it said.
//
// This direction admits no legitimate exception. §6.4.4's waiver drop and
// §9.3.6's posted-index drop can only remove records, never create them, so a
// record standing at a seat is one the role raised there.
//
// The opposite direction is left unenforced deliberately: a `finding` or
// `question` cell at a seat holding no record is the expected outcome of those
// same two drops, which remove the record and leave the cell the role filled
// when it raised it.
func (c *cellChecker) consistent(line int, cell *Cell) error {
	ids := c.raised[Seat{Unit: cell.Unit, Role: cell.Role}]
	if (cell.Result != ResultPass && cell.Result != ResultNA) || len(ids) == 0 {
		return nil
	}
	said := "did not find nothing"
	if cell.Result == ResultNA {
		said = "had something to say about it"
	}
	return &RejectedCellError{
		File: c.file, Line: line, Field: "result",
		Problem: fmt.Sprintf(
			"is %s at unit %q and role %q, and this round holds record(s) %s from that role on "+
				"that unit; a role that raised a record there %s, so file "+
				"the cell as %s or %s",
			cell.Result, cell.Unit, cell.Role, listed(ids), said, ResultFinding, ResultQuestion),
	}
}

// coverage holds one cell to §4.5.5's conditional `coverage` object.
//
// §4.5.5 asks for it "when the role is on the `test` axis per §4.4.1", so the
// condition is the role's axis and not anything the line itself says. Both
// directions are checked for the reason §3.3's `note_id` is: a test-axis cell
// with no classification leaves §4.4.1's whole answer unrecorded, and a cell on
// another axis carrying one asserts a coverage judgement no role on that axis
// was asked to make and no probe backs.
//
// An `na` cell is the exception, and it is the one dogfooding found: §4.5.5
// gives `na` to every role, §4.4.1's classification is the answer a role
// reached about a unit, and `na` is the statement that it reached none. A
// test-adequacy role looking at the test file itself has nothing to classify —
// requiring the object there would make `na` unreachable on the test axis, and
// requiring a classification would have the role invent one to satisfy cr. So
// the reason §4.5.5 already demands of an `na` stands in its place, and the
// object is refused rather than merely optional: a classification beside a
// verdict that says no judgement was reached is a judgement with nothing behind
// it.
func (c *cellChecker) coverage(
	line int, filled *role.Role, supplied map[string]json.RawMessage, cell *Cell,
) error {
	classifies := filled.Axis == axis.Test && cell.Result != ResultNA
	switch {
	case classifies && cell.Coverage == nil:
		return &RejectedCellError{
			File: c.file, Line: line, Field: "coverage",
			Problem: fmt.Sprintf(
				"is required by §4.5.5 when the role is on the %s axis, and %q is; "+
					"§4.4.1 has the classification recorded with the test paths it rested on",
				axis.Test, filled.ID),
		}
	case !classifies && cell.Coverage != nil && cell.Result == ResultNA:
		return &RejectedCellError{
			File: c.file, Line: line, Field: "coverage",
			Problem: fmt.Sprintf(
				"is §4.4.1's classification, and this cell is an %s: the reason §4.5.5 "+
					"asks of an %s says what the object would have to stand for",
				ResultNA, ResultNA),
		}
	case !classifies && cell.Coverage != nil:
		return &RejectedCellError{
			File: c.file, Line: line, Field: "coverage",
			Problem: fmt.Sprintf(
				"is §4.4.1's answer for the %s axis, and %q is on the %s axis",
				axis.Test, filled.ID, filled.Axis),
		}
	case !classifies:
		return nil
	}
	// The classification itself is not checked here. §4.4.1's closed set
	// is testadequacy.Coverage's own, refused inside its UnmarshalJSON
	// before this ever runs. A second check would be a second reading of
	// the same three words, and the two could disagree.
	return c.testPaths(line, supplied["coverage"], cell.Coverage)
}

// coverageFields are the keys of §4.5.5's `coverage` object, in the order a
// rejection lists them.
var coverageFields = []string{"classification", "test_paths"}

// testPaths holds a test-axis cell's `coverage` object to its own keys and to
// §4.4.1's "together with the test paths it rested on".
//
// The object's keys are folded and fenced as the line's are, and for the same
// reasons: a key given twice is decoded from parts of both copies, and a key
// the object does not define is dropped by the decode without a word.
//
// `test_paths` itself is required. testadequacy.Coverage turns an absent list
// into [] per §12.3, so a cell that never said what its classification rested
// on would be stored saying it rested on nothing, and nobody would have said
// so. An empty list is refused beside `covered` and `partially-covered`, which
// are verdicts that tests exercise the unit and so rest on at least the tests
// that do. It stands beside `uncovered`: that verdict can rest on no test at
// all, when the pull request attached none, and refusing it would have the
// role name a test path it never read to satisfy cr.
func (c *cellChecker) testPaths(line int, object json.RawMessage, recorded *testadequacy.Coverage) error {
	keys, err := state.FoldedFields(object)
	if repeated := (*state.RepeatedKeyError)(nil); errors.As(err, &repeated) {
		return &state.RepeatedKeyError{File: c.file, Line: line, Key: "coverage." + repeated.Key}
	}
	if err != nil {
		return &state.MalformedLineError{File: c.file, Line: line, Err: err}
	}
	unknown := make([]string, 0)
	for key := range keys {
		if !slices.Contains(coverageFields, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return &RejectedCellError{
			File: c.file, Line: line, Field: "coverage." + unknown[0],
			Problem: "is not a field of §4.5.5's coverage object, which has exactly " +
				listed(coverageFields),
		}
	}
	switch {
	case !written(keys["test_paths"]):
		return &RejectedCellError{
			File: c.file, Line: line, Field: "coverage.test_paths",
			Problem: "is required by §4.5.5, and §4.4.1 records the classification together with " +
				"the test paths it rested on; write [] only when an uncovered verdict rested on none",
		}
	case len(recorded.TestPaths()) == 0 && recorded.Classification() != testadequacy.Uncovered:
		return &RejectedCellError{
			File: c.file, Line: line, Field: "coverage.test_paths",
			Problem: fmt.Sprintf(
				"is empty, and the classification is %s; §4.4.1 records the classification together "+
					"with the test paths it rested on, so name the tests that exercise the unit",
				recorded.Classification()),
		}
	}
	return nil
}

// activeIDs names the active roles, so a rejection tells the user which ones
// this round would have accepted.
func activeIDs(active []role.Role) []string {
	out := make([]string, 0, len(active))
	for i := range active {
		out = append(out, active[i].ID)
	}
	return out
}

// listed renders a closed set for a message, and says so when it is empty
// rather than trailing off after "at".
func listed(values []string) string {
	if len(values) == 0 {
		return "empty"
	}
	return strings.Join(values, ", ")
}

// The two values a JSON object can hold under a key and still supply nothing.
var (
	nullLiteral = []byte("null")
	emptyString = []byte(`""`)
)

// written reports whether a wire line supplied a field, reading null and the
// empty string as absent for the reason internal/intent's own written does: a
// cell whose `reason` is `""` explains nothing, and §4.5.5's requirement is
// about the explanation and not about the key.
func written(value json.RawMessage) bool {
	value = bytes.TrimSpace(value)
	return len(value) > 0 &&
		!bytes.Equal(value, nullLiteral) &&
		!bytes.Equal(value, emptyString)
}
