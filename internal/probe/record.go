package probe

import (
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/state"
)

// Record is one line of probes.ndjson: §5.5's probe record, with the head and
// round §2.3.3 stamps onto every record of that file.
//
// The two counts are pointers for the reason run.Record's are: §5.5 asks for
// them "when derivable", and §5.3.4's fifth rung turns on the difference
// between a count of zero and no count at all. A pointer distinguishes them and
// `omitempty` keeps an underivable count off the wire, so a reader cannot
// mistake an absent count for a zero one.
//
// What is not here is §5.5's per-kind result vocabulary and the field-table
// check that holds this struct to the section. Both belong to one place each —
// §5.5 says the vocabulary "is per kind and MUST NOT be shared", and the gap
// probe's half of the table does not exist yet — so they are left to the tasks
// that own them rather than half-stated here.
type Record struct {
	// ID is §5.5's `p<n>`. It is the id space §5.5.2 has a finding
	// reference, so it is allocated against every probe the pull request
	// has recorded and never reused.
	ID string `json:"id"`
	// Stamp carries §5.5's `head` and §2.3.3's `round`. They are embedded
	// rather than declared here because state.WriteStamped is their one
	// author: a record cannot arrive carrying either.
	state.Stamp
	// Kind is §5.5's `kind`: the sort of probe §5.3 or §5.4 ran.
	Kind Kind `json:"kind"`
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
	// DurationMS is the wall-clock duration of the probe's own run.
	DurationMS int64 `json:"duration_ms"`
	// OutputTail is the runner's output truncated to
	// `tests.output_tail_bytes`, per §5.5. §12.5 has `--compact` omit it.
	OutputTail string `json:"output_tail"`
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
