package probe

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/state"
)

// IDTakenError reports a probe id reserved before a run and allocated by
// another run before this one's record could be written.
//
// §5.4.2 has a gap probe know its id before it runs, because §2.4's
// `tests.probe_path_template` puts the id in the path the test file is placed
// at. The reservation is read outside §2.3.1's lock and the record is written
// under it, so the two can be separated by another `cr probe run` — and §5.5.2
// has a finding reference a probe by this id, which two records sharing one
// would make ambiguous. Nothing is written when this refuses, and the probe
// file has already been removed, so the experiment can simply be repeated.
type IDTakenError struct {
	// Reserved is the id the placement was named for.
	Reserved string
	// Next is the id the allocation now hands out.
	Next string
}

func (e *IDTakenError) Error() string {
	return fmt.Sprintf(
		"probe id %s was taken while this probe ran, and the next free id is %s: "+
			"§5.4.2 fixes a gap probe's id before the run because §2.4's template puts it "+
			"in the path; nothing was recorded, so run the probe again",
		e.Reserved, e.Next)
}

// Record is one line of probes.ndjson: §5.5's probe record, with the head and
// round §2.3.3 stamps onto every record of that file.
//
// The declaration is in §5.5's own table order, with §2.3.3's `round` beside
// the `head` it travels with, and checkRecordFields below holds it there: the
// rows the section fixes have one home, and a field added to the struct has to
// be added to the section's list too rather than reaching probes.ndjson
// unannounced.
//
// The two counts are pointers for the reason run.Record's are: §5.5 asks for
// them "when derivable", and §5.3.4's fifth rung turns on the difference
// between a count of zero and no count at all. A pointer distinguishes them and
// `omitempty` keeps an underivable count off the wire, so a reader cannot
// mistake an absent count for a zero one.
//
// What is not here is §5.5's per-kind result vocabulary. The section says it
// "is per kind and MUST NOT be shared", so which values a `mutation` may carry
// and which a `gap` may carry is one statement, and it belongs in the one place
// that closes the set rather than half-stated on the field.
type Record struct {
	// ID is §5.5's `p<n>`. It is the id space §5.5.2 has a finding
	// reference, so it is allocated against every probe the pull request
	// has recorded and never reused.
	ID string `json:"id"`
	// Kind is §5.5's `kind`: the sort of probe §5.3 or §5.4 ran.
	Kind Kind `json:"kind"`
	// Stamp carries §5.5's `head` and §2.3.3's `round`. They are embedded
	// rather than declared here because state.WriteStamped is their one
	// author: a record cannot arrive carrying either.
	state.Stamp
	// Input is the patch or test file content, kept whole. §12.5 has
	// `--compact` omit it from a payload; the record keeps it, because it
	// is what says which experiment was performed.
	Input string `json:"input"`
	// Filter is the test filter the run was narrowed to, absent when the
	// whole suite ran. §5.3.6 carries it into the evidence region, so a
	// filtered run never claims more than the tests it selected.
	Filter string `json:"filter,omitempty"`
	// Result is §5.5's `result`, and is always the value Decide produced:
	// §5.1.7's check has had its say before the record is written.
	Result Result `json:"result"`
	// TestsRun is the executed test count, absent when undetermined.
	TestsRun *int `json:"tests_run,omitempty"`
	// TestsFailed is the failed test count, absent when undetermined.
	TestsFailed *int `json:"tests_failed,omitempty"`
	// Baseline is the id of the run record §5.2.6 admits as this probe's
	// baseline. §5.3.5 and §5.4.4 read that record's `passed` before
	// letting the probe support a `probed` grade, so a probe without one
	// is a probe that can establish nothing.
	Baseline string `json:"baseline"`
	// Target is §5.5's `path:line`, always `side: RIGHT` because a probe
	// runs against the sandbox at head. For a mutation probe it is
	// derived from the patch per §5.3.2 and never supplied, which is what
	// keeps the evidence chain running on the experiment cr performed
	// rather than on a flag an agent typed; for a gap probe it is the
	// `--target` the agent supplied, validated by CheckTarget as §6.2.3
	// validates a citation.
	Target string `json:"target"`
	// DurationMS is the wall-clock duration of the probe's own run.
	DurationMS int64 `json:"duration_ms"`
	// OutputTail is the runner's output truncated to
	// `tests.output_tail_bytes`, per §5.5. §12.5 has `--compact` omit it.
	OutputTail string `json:"output_tail"`
}

// field is one row of §5.5's table, by the name it goes by on the wire.
type field struct {
	// Name is the JSON key.
	Name string
	// Stamped marks the two rows §2.3.3 takes out of every writer's
	// hands. They reach the record through the embedded state.Stamp,
	// which is the only type state.WriteStamped writes through, so a
	// record cannot arrive carrying either.
	Stamped bool
}

// fields is §5.5's table in its own order, with §2.3.3's `round` beside the
// `head` it travels with.
//
// It is held as data for the reason internal/run and internal/finding hold
// their sections' tables as data: so the shape the section fixes has one home,
// and a row is added to the Go struct only by being added here too.
var fields = []field{
	{Name: "id"},
	{Name: "kind"},
	{Name: "head", Stamped: true},
	{Name: "round", Stamped: true},
	{Name: "input"},
	{Name: "filter"},
	{Name: "result"},
	{Name: "tests_run"},
	{Name: "tests_failed"},
	{Name: "baseline"},
	{Name: "target"},
	{Name: "duration_ms"},
	{Name: "output_tail"},
}

// The check runs at package initialisation, so a Record that has drifted from
// §5.5 cannot reach a probe run: the binary refuses to start rather than
// writing one line of a shape the section does not describe.
var _ = checkRecordFields()

// checkRecordFields holds Record to the fields table, and the table to §5.5.
//
// It is run.checkRecordFields aimed at §5.5's table, and it reads the JSON tags
// for the same reason: the wire names are what the section fixes, and the wire
// names are what a reader of probes.ndjson sees. A field added to the struct
// without a row here is what it catches, and that is the way §5.5's table is
// quietly outgrown — an ordinary-looking field, added beside the others,
// carrying something the section never described.
//
// It also proves where `head` and `round` come from. A probe record that
// declared them itself would encode identically and stamp nothing, so the check
// requires that they arrive promoted from the embedded state.Stamp.
func checkRecordFields() bool {
	record := reflect.TypeFor[Record]()
	stamp := reflect.TypeFor[state.Stamp]()
	declared := make([]field, 0, len(fields))
	for _, visible := range reflect.VisibleFields(record) {
		// The embedded struct itself carries no JSON name; the fields
		// it contributes follow it and are what the wire sees.
		if visible.Anonymous {
			continue
		}
		name, _, _ := strings.Cut(visible.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			panic("§5.5: Record." + visible.Name + " reaches probes.ndjson under no name")
		}
		// Index[0] is the top-level field the value arrived through,
		// which for a promoted field is the embedded struct and for a
		// direct one is the field itself — and no direct field of
		// Record is a state.Stamp, so the one test answers.
		declared = append(declared, field{
			Name:    name,
			Stamped: record.Field(visible.Index[0]).Type == stamp,
		})
	}
	if !slices.Equal(declared, fields) {
		panic(fmt.Sprintf(
			"§5.5: a probe record would be written as %v, and the section's rows are %v",
			declared, fields))
	}
	return true
}

// idPrefix is the letter §5.5 gives a probe record id.
const idPrefix = "p"

// NextID allocates the id for a new probe record.
//
// existing MUST be every record probes.ndjson holds rather than the current
// round's, for the reason run.NextID reads the whole file: §5.5.2 has a finding
// reference a probe by this id and §5.5.3 keeps records from earlier heads on
// disk, so an id an earlier round spent still names the probe a stored finding
// points at.
func NextID(existing []Record) string {
	highest := 0
	// Indexed rather than ranged by value: a record carries the whole
	// patch and an output tail, and only its id is read here.
	for i := range existing {
		// `>=` here would behave identically — it would assign the
		// value already held — so no test can tell the two apart, and
		// gremlins reports the boundary as a surviving mutant for the
		// same reason it does in run.NextID.
		if n, ok := parseID(existing[i].ID); ok && n > highest {
			highest = n
		}
	}
	return idPrefix + strconv.Itoa(highest+1)
}

// parseID reads the n of a p<n> id, accepting only the canonical spelling, as
// run.parseID accepts only r<n>. An id cr did not write is no evidence about
// what is taken, so it contributes nothing to the next allocation.
func parseID(id string) (int, bool) {
	rest, found := strings.CutPrefix(id, idPrefix)
	if !found {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || rest != strconv.Itoa(n) || n < 1 {
		return 0, false
	}
	return n, true
}

// Grades reports whether this probe record may be used to grade a finding in a
// round whose head is head, per §5.5.3.
//
// One condition, and it is the section's whole sentence: the probe's head is
// the round's. An experiment performed against other code establishes nothing
// about this one, and §5.5.3 does not soften that for any result — a
// `no-test-failed` from the previous head is not weaker evidence, it is
// evidence about a tree that is no longer under review.
//
// Records from earlier heads stay on disk, which is why this exists at all.
// §5.5.1 makes them immutable and NextID allocates above every id the file has
// ever held, so a finding stored in an earlier round still names the probe it
// rested on; what §5.5.3 removes is not the record but its standing in this
// round.
//
// It is one predicate rather than a comparison written at each reader, because
// §6.2's grade computation, §5.4.4's support conditions and §5.4.5's severity
// bounds all ask the same question, and three spellings of it could answer
// three ways.
func (r *Record) Grades(head string) bool { return r.Head == head }
