// Package proposal models the proposed experiment of spec/0.4.0.md §5.7.
//
// A role holds the suspicion and cr holds the runner. §6.1 gives a record only
// `probe`, the id of an experiment that already ran, so before §5.7 a role that
// had found something it could not establish had nowhere to put the experiment
// that would settle it. Measurement 3 is what that cost: 51 records graded 32
// `cited`, 19 `argued` and 0 `probed`.
//
// A proposal is the ask, and it is data of the same kind as a claim. The agent
// writes it, this package validates it, and `cr probe run --proposal` executes
// it. **It is never evidence.** §5.7.2 keeps cr from reading a proposal when it
// grades, and this package accordingly exposes nothing a grader could ask: a
// proposal becomes evidence only by being run, and then it is the probe record
// that grades.
//
// Like internal/coverage, the type has no constructor. state.DecodeStamped
// allocates every Proposal cr holds, out of a line an agent wrote, so cr cannot
// invent an experiment nobody asked for.
package proposal

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/state"
)

// The two kinds §5.7 admits, which are §5.3's and §5.4's and no others.
//
// They are probe.Kind's values rather than strings of this package's, so a
// proposal cannot name a kind `cr probe run` has never heard of.
const (
	KindMutation = string(probe.Mutation)
	KindGap      = string(probe.Gap)
)

// kinds is the closed set in the order §5.7 names them, so a rejection always
// lists them the same way.
var kinds = []string{KindMutation, KindGap}

// The three states §5.7's table gives a proposal. cr writes all of them; an
// agent that supplies one is refused per §5.7.1.
const (
	// StateOpen is a proposal cr has stored and not yet run.
	StateOpen = "open"
	// StateRun is a proposal `cr probe run --proposal` executed, which
	// carries the probe id it produced.
	StateRun = "run"
	// StateUnrunnable is a proposal cr cannot execute, which carries the
	// reason. §5.7.5 stores it rather than dropping it: a proposal cr
	// silently discarded would read to the operator as a role that asked
	// for nothing.
	StateUnrunnable = "unrunnable"
)

// idPrefix is the letter §5.7 gives a proposal id. It is not `f`, so a proposal
// id and a record id can never be confused for one another — which is what lets
// §5.7.2's refusal of a proposal in `probe` be decided by spelling.
const idPrefix = "x"

// Proposal is one line of proposals.ndjson: §5.7's fields, and the head and
// round §2.3.3 stamps onto every record of that file.
type Proposal struct {
	state.Stamp
	// ID is `x<n>`, from the block §4.6.2 gave the prompt.
	ID string `json:"id"`
	// Kind is KindMutation or KindGap.
	Kind string `json:"kind"`
	// Role is the role that proposed the experiment.
	Role string `json:"role"`
	// Unit is the unit whose code the experiment addresses.
	Unit string `json:"unit"`
	// Finding is the record of the current round the experiment would
	// settle, and is empty for a proposal that settles none.
	Finding string `json:"finding,omitempty"`
	// Target is the `path:line` the experiment addresses, always head-side.
	Target string `json:"target"`
	// Hypothesis is what the role believes and cannot establish.
	Hypothesis string `json:"hypothesis"`
	// Settles is the result that would settle it.
	Settles string `json:"settles"`
	// Input is the unified diff for a mutation, the test file's content for
	// a gap.
	Input string `json:"input"`
	// Filter is the test filter the run is to use.
	Filter string `json:"filter,omitempty"`
	// Paths are the `--path` values the run is to use.
	Paths []string `json:"paths"`
	// State is one of the three above; cr writes it.
	State string `json:"state"`
	// Probe is the probe that executed it; cr writes it.
	Probe string `json:"probe,omitempty"`
	// Reason is why the proposal is unrunnable; cr writes it.
	Reason string `json:"reason,omitempty"`
}

// ValidID reports whether id is spelled the way §5.7 spells a proposal id, for
// a command that takes one from a person rather than from the file cr wrote.
func ValidID(id string) bool {
	_, ok := parseID(id)
	return ok
}

// IDSuffix reads the n of an `x<n>` proposal id.
func IDSuffix(id string) (int, bool) {
	return parseID(id)
}

// IDOf spells n as §5.7's `x<n>` proposal id, the reading IDSuffix takes back.
func IDOf(n int) string {
	return idPrefix + strconv.Itoa(n)
}

// parseID is the one reading of a proposal id, so ValidID, IDSuffix and IDOf
// cannot disagree about what one is.
func parseID(id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, idPrefix)
	if !ok || rest == "" || strings.HasPrefix(rest, "0") {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// RejectedError reports a proposal §5.7.1 refuses. The cli layer maps it onto
// exit code 1.
type RejectedError struct {
	// File is the NDJSON file the agent handed the command.
	File string
	// Line is the one-based line the proposal sits on.
	Line int
	// Field is the field the refusal blames.
	Field string
	// Problem completes the sentence naming that field.
	Problem string
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("%s line %d: %s %s", e.File, e.Line, e.Field, e.Problem)
}

// Unit is one unit of the round a proposal may sit on: its id, and the
// containment question §6.2.1 answers about it.
//
// The containment predicate is unit.Unit's, named through internal/finding's
// interface rather than declared again here, so §6.2.1's rule has one home.
type Unit struct {
	// ID is the unit id of §3.4.6.
	ID string
	// In answers whether a `path:line` lies inside it, in head coordinates.
	In finding.Containment
}

// Round is what §5.7.1 holds a proposal to: the round's units, its active
// roles, the record ids it holds, and the reader that resolves a target against
// the head.
type Round struct {
	// Units are the round's units in id order.
	Units []Unit
	// Active are the active role ids of §4.5.1.
	Active []string
	// Records are the ids of the records the current round holds, which a
	// proposal's `finding` must name one of.
	Records []string
	// Head reads a file at the round's head, for §6.2.3's resolution of the
	// target.
	Head probe.HeadFile
}

// DecodeInRound reads the proposals an agent hands `cr proposals record`,
// holding every line to §5.7's fields, to §5.7.1's refusals, and to the
// authorship of the fields cr computes.
//
// The checks run inside the decode rather than after it because
// state.DecodeStamped is the one place that counts lines, blank ones included,
// and every refusal here has to name the line the user must open.
func DecodeInRound(file string, body []byte, r *Round) ([]*Proposal, error) {
	against := checker{file: file, round: r}
	seen := make(map[string]int)
	return state.DecodeStamped[Proposal](file, body,
		func(line int, supplied map[string]json.RawMessage, p *Proposal) error {
			if err := against.check(line, supplied, p); err != nil {
				return err
			}
			return refuseRepeatedID(file, seen, line, p)
		})
}

// refuseRepeatedID refuses a proposal whose id an earlier line of the same file
// already used, naming both lines, as `cr claims record` refuses a repeated
// claim id.
func refuseRepeatedID(file string, seen map[string]int, line int, p *Proposal) error {
	if first, repeated := seen[p.ID]; repeated {
		return &RejectedError{
			File: file, Line: line, Field: "id",
			Problem: fmt.Sprintf("%q is the id of line %d; §5.7.1 refuses a repeated id, "+
				"so give each proposal in the file an id of its own", p.ID, first),
		}
	}
	seen[p.ID] = line
	return nil
}

// checker holds one file's lines to §5.7.1.
type checker struct {
	file  string
	round *Round
}

// proposalFields are §5.7's rows an agent may write, in the table's order.
var proposalFields = []string{
	"id", "kind", "role", "unit", "finding", "target",
	"hypothesis", "settles", "input", "filter", "paths",
}

// proposalComputed are §5.7's rows cr writes. `head` and `round` are not here:
// state.DecodeStamped refuses those before this checker sees the line, and
// naming them twice would give the agent two different sentences for one rule.
var proposalComputed = []string{"state", "probe", "reason"}

// check is §5.7.1, in the order the sentence names its refusals.
func (c *checker) check(line int, supplied map[string]json.RawMessage, p *Proposal) error {
	for _, step := range []func(int, map[string]json.RawMessage, *Proposal) error{
		c.fields, c.computed, c.id, c.kind, c.unit, c.role, c.target, c.finding, c.prose,
	} {
		if err := step(line, supplied, p); err != nil {
			return err
		}
	}
	if p.Paths == nil {
		p.Paths = make([]string, 0)
	}
	p.State = StateOpen
	return nil
}

// fields refuses a key §5.7's table does not give a proposal, so a role that
// misspells one is told rather than having the value silently dropped.
func (c *checker) fields(line int, supplied map[string]json.RawMessage, _ *Proposal) error {
	for name := range supplied {
		if slices.Contains(proposalFields, name) || slices.Contains(proposalComputed, name) {
			continue
		}
		return &RejectedError{
			File: c.file, Line: line, Field: name,
			Problem: "is not a field of §5.7's table; the fields a proposal carries are " +
				strings.Join(proposalFields, ", "),
		}
	}
	return nil
}

// computed refuses a field cr writes, as §6.1.4 refuses one on a record.
func (c *checker) computed(line int, supplied map[string]json.RawMessage, _ *Proposal) error {
	for _, name := range proposalComputed {
		if _, written := supplied[name]; written {
			return &RejectedError{
				File: c.file, Line: line, Field: name,
				Problem: "is computed by cr per §5.7's table; remove it from the line",
			}
		}
	}
	return nil
}

// id refuses a spelling §5.7 does not give a proposal id. The block a prompt
// gave the role is checked by the command, which is where the round's emissions
// are read.
func (c *checker) id(line int, _ map[string]json.RawMessage, p *Proposal) error {
	if !ValidID(p.ID) {
		return &RejectedError{
			File: c.file, Line: line, Field: "id",
			Problem: fmt.Sprintf("reads %q, and §5.7 spells a proposal id x<n>", p.ID),
		}
	}
	return nil
}

// kind refuses a kind outside §5.3's and §5.4's two.
func (c *checker) kind(line int, _ map[string]json.RawMessage, p *Proposal) error {
	if !slices.Contains(kinds, p.Kind) {
		return &RejectedError{
			File: c.file, Line: line, Field: "kind",
			Problem: fmt.Sprintf("reads %q, and §5.7's two kinds are %s",
				p.Kind, strings.Join(kinds, " and ")),
		}
	}
	return nil
}

// unit refuses a unit that is not one of the round's.
func (c *checker) unit(line int, _ map[string]json.RawMessage, p *Proposal) error {
	if c.at(p.Unit) == nil {
		return &RejectedError{
			File: c.file, Line: line, Field: "unit",
			Problem: fmt.Sprintf("reads %q, which is no unit of the current round", p.Unit),
		}
	}
	return nil
}

// role refuses a role that is not active this round, as §4.5.6 refuses a cell
// naming one.
func (c *checker) role(line int, _ map[string]json.RawMessage, p *Proposal) error {
	if !slices.Contains(c.round.Active, p.Role) {
		return &RejectedError{
			File: c.file, Line: line, Field: "role",
			Problem: fmt.Sprintf("reads %q, which is no active role of the current round", p.Role),
		}
	}
	return nil
}

// target resolves the `path:line` against the head and holds it inside the
// proposal's own unit, which is §5.7's table quoting §6.2.3 and §6.2.1.
//
// The containment half is what keeps a proposal from reaching past the code its
// role was given: a role that could target any line of the head could ask for
// an experiment on a unit another role is reviewing.
func (c *checker) target(line int, _ map[string]json.RawMessage, p *Proposal) error {
	if err := probe.CheckTarget(c.round.Head, p.Target); err != nil {
		// Only the target's own fault is reported as the target's. A
		// read that failed because the clone lacks the head is the
		// clone's, and folding it in here would answer a missing commit
		// with "correct this line" — a step that cannot work, over data
		// nothing is wrong with.
		invalid := &probe.InvalidTargetError{}
		if !errors.As(err, &invalid) {
			return err
		}
		return &RejectedError{
			File: c.file, Line: line, Field: "target",
			Problem: fmt.Sprintf("reads %q, which does not resolve at the round's head: %v", p.Target, err),
		}
	}
	path, at, err := probe.ParseTarget(p.Target)
	if err != nil {
		return &RejectedError{
			File: c.file, Line: line, Field: "target",
			Problem: fmt.Sprintf("reads %q, and §5.7 spells a target path:line", p.Target),
		}
	}
	if in := c.at(p.Unit); in != nil && !in.In.Contains(path, at) {
		return &RejectedError{
			File: c.file, Line: line, Field: "target",
			Problem: fmt.Sprintf("reads %q, which lies outside unit %s; §5.7's table holds a "+
				"proposal's target inside its own unit under §6.2.1's containment", p.Target, p.Unit),
		}
	}
	return nil
}

// finding refuses a `finding` naming no record of the current round. It is
// optional, so an empty one is a proposal that settles no record rather than a
// refusal.
func (c *checker) finding(line int, _ map[string]json.RawMessage, p *Proposal) error {
	if p.Finding == "" || slices.Contains(c.round.Records, p.Finding) {
		return nil
	}
	return &RejectedError{
		File: c.file, Line: line, Field: "finding",
		Problem: fmt.Sprintf("names %q, which is no record of the current round", p.Finding),
	}
}

// prose refuses an empty required string. `hypothesis` and `settles` are what
// make a proposal readable by the person who decides whether to spend a probe
// on it, and `input` is what there is to run.
func (c *checker) prose(line int, _ map[string]json.RawMessage, p *Proposal) error {
	for _, required := range []struct{ field, value string }{
		{"hypothesis", p.Hypothesis}, {"settles", p.Settles}, {"input", p.Input},
	} {
		if strings.TrimSpace(required.value) == "" {
			return &RejectedError{
				File: c.file, Line: line, Field: required.field,
				Problem: "is required by §5.7's table and this line leaves it empty",
			}
		}
	}
	return nil
}

// at is the round's unit under id, and nil when the round holds none.
func (c *checker) at(id string) *Unit {
	for i := range c.round.Units {
		if c.round.Units[i].ID == id {
			return &c.round.Units[i]
		}
	}
	return nil
}
