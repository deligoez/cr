// Package run holds §5.2.4's test run record: what one execution of the
// profile's suite inside the sandbox produced, stored as a line of
// runs.ndjson.
//
// It is the only record type in cr that no agent ever writes. §6.1's findings,
// §3.3's claims, §4.1.6's mapping and §4.5's cells all arrive through a `cr …
// record` command and are fenced field by field, because the agent supplies
// most of a record and cr computes the rest. A run record has no such split:
// every value in it is something cr measured, allocated, or was handed by the
// runner, so there is no command that reads one in and nothing for a
// Required or Optional row of the §6.1 kind to mean here. checkRecordFields
// keeps that true — see the fields table below.
package run

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/deligoez/cr/internal/state"
)

// Record is one line of runs.ndjson: §5.2.4's run record, with the head and
// round §2.3.3 stamps onto every record of that file.
//
// The two counts are pointers because §5.2.4 asks for them "when derivable"
// and §5.2.5 reads the difference: a run whose counts are underivable never
// passes and cannot serve as a baseline, which is not the same as a run that
// executed zero tests. A pointer distinguishes them and `omitempty` keeps an
// underivable count off the wire entirely, so a reader cannot mistake an
// absent count for a zero one. Deriving the values is §5.2.1's extraction,
// which is a separate task; what is settled here is how the two states are
// stored.
type Record struct {
	// ID is §5.2.4's `r<n>`. It is the id space a probe's `baseline`
	// field references (§5.5), so it is allocated against every run the
	// pull request has recorded and never reused.
	ID string `json:"id"`
	// Stamp carries §5.2.4's `head` and §2.3.3's `round`. They are
	// embedded rather than declared here because state.WriteStamped is
	// their one author: a record cannot arrive carrying either, and a
	// local field would be a second place they could come from.
	state.Stamp
	// Filter is the expression the run was narrowed to, absent when the
	// whole suite ran. §5.2.2 makes the unfiltered run the baseline and
	// §5.5 has a mutation probe reference the run carrying its own
	// filter, so the two are told apart by this field.
	Filter string `json:"filter,omitempty"`
	// ExitCode is the runner's own exit status.
	ExitCode int `json:"exit_code"`
	// TimedOut says the run was killed for exceeding
	// `tests.timeout_seconds`, which is what §5.2.3 requires be
	// "recorded as timeout".
	//
	// Round 12's unstorable-value finding is why the field exists:
	// §5.2.4's list has no slot for the outcome §5.2.3 mandates, and
	// `exit_code` cannot carry it. A process cr killed reports whatever
	// the platform reports for a killed process — the same shape of
	// number a runner that decided to fail produces — so a reader of the
	// exit code alone cannot tell a suite that finished and failed from
	// one that never finished at all. §5.3.4's ladder puts the two on
	// different rungs, `timeout` before `error` and both before any
	// reading of the counts, so the distinction has to survive into
	// storage rather than being inferred from it.
	TimedOut bool `json:"timed_out"`
	// Unstarted says the runner could not be started at all, so there is
	// no exit status of its own: ExitCode then holds -1 rather than a 0
	// that would read as a run that succeeded. It is absent for a run
	// that started, and §5.2.4's list has no slot for it, for the reason
	// it has none for TimedOut.
	Unstarted bool `json:"unstarted,omitempty"`
	// Contaminated says §5.1.6's cleanliness check failed after the run,
	// so the sandbox the suite executed in was not the pull request head.
	//
	// Round 12's baseline-contamination finding is why the field exists.
	// §5.1.7 voids a probe whose post-run check fails, overriding the
	// ladder outcome with `error`, but nothing voided a plain `cr test`
	// run — so a suite that dirtied a tracked file under itself could be
	// stored `passed: true` and later resolved as the baseline a probe is
	// graded against. That is the most expensive shape of wrong cr has:
	// a pre-existing failure attributed to the probe, asserted to a
	// colleague on an experiment that measured something else.
	//
	// §5.2.4's list has no slot for it, for the reason it has none for
	// TimedOut: the outcome another section mandates has to survive into
	// storage rather than be inferred from a number that cannot carry it.
	Contaminated bool `json:"contaminated"`
	// DurationMS is the wall-clock duration of the run in milliseconds.
	DurationMS int64 `json:"duration_ms"`
	// TestsRun is §5.2.1's executed test count, absent when undetermined.
	TestsRun *int `json:"tests_run,omitempty"`
	// TestsFailed is §5.2.1's failed test count, absent when
	// undetermined.
	//
	// Undetermined is not the same as zero here, and §5.2.1 spells the
	// difference out: a `tests.failed_pattern` that never matches yields
	// zero, because a runner that prints a status only when its count is
	// non-zero prints nothing at all for no failures, while a
	// `tests.count_pattern` that never matches leaves both counts
	// undetermined. So a stored `"tests_failed": 0` beside a present
	// `tests_run` is a measurement, and an absent one is the absence of a
	// measurement.
	TestsFailed *int `json:"tests_failed,omitempty"`
	// OutputTail is the last `tests.output_tail_bytes` of the runner's
	// output, per §5.2.1. §12.5 has `--compact` omit it.
	OutputTail string `json:"output_tail"`
	// Passed is §5.2.5's verdict, and the field a baseline is read
	// through: §5.2.6 admits only a run record carrying no `probe` as a
	// baseline, and §5.3.5 and §5.4.4 require that baseline to have
	// passed before a probe may support a `probed` grade.
	//
	// The field is not set by whoever builds the record. Verdict computes
	// it from the record's own fields at the moment of the write, beside
	// the id allocation, so no path into runs.ndjson can store a `true`
	// the counts do not support.
	Passed bool `json:"passed"`
	// Probe is the id of the probe record whose mutated or
	// probe-injected code this run measured, absent for a run on
	// untouched code. §5.2.6 reads it as the fence around a baseline:
	// only a run carrying no `probe` may serve as one.
	Probe string `json:"probe,omitempty"`
}

// author says who writes one field of the run record. There are two, and the
// absence of a third is the point: §6.1's Required and Optional rows mark the
// fields an agent supplies, and a run record has none of those.
type author int

const (
	// measured marks a field cr allocates, measures, or takes from the
	// runner it started.
	measured author = iota
	// stamped marks a field state.WriteStamped writes on the way out
	// (§2.3.3), which is why it reaches the record through state.Stamp
	// rather than through a field of its own.
	stamped
)

// field is one row of §5.2.4's record, by the name it goes by on the wire.
type field struct {
	// Name is the JSON key.
	Name string
	// Author is who writes it.
	Author author
}

// fields is §5.2.4's record in wire order, with §2.3.3's `round` beside the
// `head` it travels with.
//
// It is held as data for the reason internal/finding holds §6.1's table as
// data: so the shape the section fixes has one home, and a field added to the
// Go struct has to be added to the section's list too rather than reaching
// runs.ndjson unannounced.
var fields = []field{
	{Name: "id", Author: measured},
	{Name: "head", Author: stamped},
	{Name: "round", Author: stamped},
	{Name: "filter", Author: measured},
	{Name: "exit_code", Author: measured},
	{Name: "timed_out", Author: measured},
	{Name: "unstarted", Author: measured},
	{Name: "contaminated", Author: measured},
	{Name: "duration_ms", Author: measured},
	{Name: "tests_run", Author: measured},
	{Name: "tests_failed", Author: measured},
	{Name: "output_tail", Author: measured},
	{Name: "passed", Author: measured},
	{Name: "probe", Author: measured},
}

// The check runs at package initialisation, so a Record that has drifted from
// §5.2.4 cannot reach a run: the binary refuses to start rather than writing
// one line of a shape the section does not describe.
var _ = checkRecordFields()

// checkRecordFields holds Record to the fields table, and the table to §5.2.4.
//
// It is the device internal/cli/output.go's omittedFields uses, aimed at the
// other half of the same problem. There, a rule written only in a comment is
// one an edit reads past; here, the rule is that runs.ndjson carries §5.2.4's
// fields and nothing else, and the way past it is an ordinary-looking field
// added to a struct. Reflection over the JSON tags is what makes the two
// statements the same statement — the wire names are what §5.2.4 fixes, and
// the wire names are what this reads.
//
// It also proves where `head` and `round` come from. A run record that
// declared them itself would encode identically and stamp nothing, so the
// check requires that they arrive promoted from the embedded state.Stamp,
// which is the only type state.WriteStamped can write through.
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
			panic("§5.2.4: Record." + visible.Name + " reaches runs.ndjson under no name")
		}
		// The field is stamped when the struct it came out of is
		// state.Stamp. Index[0] is the top-level field it arrived
		// through, which for a promoted field is the embedded struct
		// and for a direct one is the field itself — and no direct
		// field of Record is a state.Stamp, so the one test answers.
		by := measured
		if record.Field(visible.Index[0]).Type == stamp {
			by = stamped
		}
		declared = append(declared, field{Name: name, Author: by})
	}
	if !slices.Equal(declared, fields) {
		panic(fmt.Sprintf(
			"§5.2.4: a run record would be written as %v, and the section's rows are %v",
			declared, fields))
	}
	return true
}

// Verdict is §5.2.5's predicate over a run: `passed` is true when the exit
// code is 0, the executed count is derivable and greater than zero, and the
// failed count is 0; otherwise false.
//
// All four clauses are conjoined and none of them is redundant. An exit code
// of 0 is not enough on its own — a runner that selected nothing and reported
// success is the shape §5.3.4 rung 4 and §5.4.3 rung 3 exist to catch — and
// neither is a failed count of zero, which an unmatched `tests.failed_pattern`
// produces for a run that never got as far as executing a test.
//
// An undetermined count is not a passing one, in either position. §5.2.5 says
// so directly — "A run whose counts are underivable never passes, so it cannot
// serve as a baseline" — and §5.3.5 and §5.4.4 are what make it matter: a
// probe may support a `probed` grade only when its baseline passed, and a
// baseline that passed on counts nobody could read would attribute the
// repository's pre-existing failures to the probe.
//
// Contamination is the fourth clause, and it is not one of §5.2.5's three.
// §5.2.5 describes a run of the pull request head's code, and round 12's
// baseline-contamination finding is about a run that was not one: §5.1.6's
// check failed after it, so the suite measured a sandbox that had drifted from
// the head under it. §5.1.7 already gives that situation its answer for a probe
// — the cleanliness failure overrides the ladder outcome, which §5.3.4 declares
// total over every run — and the same failure is given the same standing here,
// so a contaminated run cannot be stored `passed: true` and later resolved as
// somebody's baseline.
//
// This is a method on the record rather than a computation at the call site
// because there is one truthful answer per record and the caller must not be
// able to supply a different one.
func (r *Record) Verdict() bool {
	return !r.Contaminated &&
		r.ExitCode == 0 &&
		r.TestsRun != nil && *r.TestsRun > 0 &&
		r.TestsFailed != nil && *r.TestsFailed == 0
}

// idPrefix is the letter §5.2.4 gives a run record id.
const idPrefix = "r"

// NextID allocates the id for a new run record.
//
// existing MUST be every record runs.ndjson holds rather than the current
// round's, and for the reason finding.NextID reads the whole file: §5.5 has a
// probe reference its baseline by this id, and §5.5.3 keeps probe records from
// earlier heads on disk, so an id an earlier round spent still names the run a
// stored probe points at. Reusing it would silently repoint that probe at a
// different measurement.
func NextID(existing []Record) string {
	highest := 0
	// Indexed rather than ranged by value: a record carries an output
	// tail, and only its id is read here.
	for i := range existing {
		// `>=` here would behave identically — it would assign the
		// value already held — so no test can tell the two apart, and
		// gremlins reports the boundary as a surviving mutant for the
		// same reason it does in finding.NextID.
		if n, ok := parseID(existing[i].ID); ok && n > highest {
			highest = n
		}
	}
	return idPrefix + strconv.Itoa(highest+1)
}

// parseID reads the n of an r<n> id, accepting only the canonical spelling, as
// finding.parseID accepts only f<n>. An id cr did not write is no evidence
// about what is taken, so it contributes nothing to the next allocation.
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

// Tail keeps the last limit bytes written through it, which is §5.2.4's
// `output_tail`.
//
// It is an io.Writer rather than a function over a captured buffer because the
// alternative is to hold the whole of a suite's output in memory to throw away
// all but four kilobytes of it, and a suite that prints a line per assertion
// prints a great many of them. The runner's output also has to reach the
// person watching the command as it is produced, so it is already passing
// through a writer.
//
// Writes are not synchronised, and do not need to be: os/exec calls Write from
// at most one goroutine at a time when a command's Stdout and Stderr are the
// same comparable value, which is how sandbox.Run merges the two streams.
type Tail struct {
	limit int
	held  []byte
}

// NewTail returns a tail keeping the last limit bytes. A limit of zero or less
// keeps nothing; §2.4 gives `tests.output_tail_bytes` a default of 4096 and
// refuses a non-positive value, so a caller reaching this with one has already
// declined to retain output.
func NewTail(limit int) *Tail {
	return &Tail{limit: max(limit, 0)}
}

// Write keeps the last limit bytes of everything written so far, and reports
// the whole of p as written: what a caller hands over is accepted, and the
// truncation is this type's business rather than a short write.
// Both boundaries here survive mutation and are equivalent: at exactly the
// limit the first slices from index zero and the second copies the whole of
// what is held onto itself, so `>=` produces the same bytes as `>` does.
func (t *Tail) Write(p []byte) (int, error) {
	written := len(p)
	if len(p) > t.limit {
		p = p[len(p)-t.limit:]
	}
	t.held = append(t.held, p...)
	if len(t.held) > t.limit {
		// Copied to the front rather than resliced, so the bytes
		// already dropped stop being retained by the backing array.
		t.held = append(t.held[:0], t.held[len(t.held)-t.limit:]...)
	}
	return written, nil
}

// String is the retained tail, with a leading partial rune removed.
//
// §5.2.4 measures the tail in bytes, and a cut at a byte boundary can land
// inside a multi-byte rune. The fragment left at the front is not a character,
// and carrying it into JSON would have the encoder substitute U+FFFD for it —
// a replacement the reader cannot tell from output that really held one. So
// the fragment is dropped, which keeps the result inside the byte bound §5.2.4
// sets and leaves it valid UTF-8. Nothing else is trimmed: the tail is the
// runner's output as the runner wrote it.
//
// utf8.UTFMax bounds the search because a rune is at most that many bytes, so a
// cut through one leaves at most UTFMax-1 continuation bytes ahead of the next
// start. The bound is what keeps the sentence above true: output that is not
// UTF-8 at all can carry more continuation bytes than any cut explains, and a
// search past the bound would trim them off the front as though they were half
// a character.
func (t *Tail) String() string {
	held := t.held
	for i := 0; i < len(held) && i < utf8.UTFMax; i++ {
		if utf8.RuneStart(held[i]) {
			return string(held[i:])
		}
	}
	return string(held)
}
