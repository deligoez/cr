package render

import (
	"fmt"
	"strings"

	"github.com/deligoez/cr/internal/probe"
)

// The named pair §8.1.3 gives the evidence region.
//
// A pair rather than a count: §8.1.3 has recovery "never depend on counting",
// so the region is found by its own opening and closing marker wherever it
// sits. Where it sits, which regions surround it, and the refusal of an agent
// body carrying a `<!-- cr:` sequence are §8.1.3's own obligations and belong
// to the task that assembles the comment.
const (
	evidenceOpen  = "<!-- cr:evidence -->"
	evidenceClose = "<!-- cr:/evidence -->"
)

// ProbeEvidence renders §8.1.7's evidence region for a record whose `probed`
// grade rests on this probe.
//
// It is template substitution and nothing else. §8.1.7 names what the region
// carries for a probe — `kind`, `target`, `filter`, `result` and `output_tail`
// — and each is written out as the record holds it, under §5.5's own field
// name, so the author checks the comment against the state cr wrote rather than
// against a phrasing cr chose.
//
// Nothing is composed, concluded or summarised here, and that is §5.3.6's
// requirement as much as §8.1.7's. A filtered run proves the gap only for the
// tests it selected, and cr cannot establish that a filter selects the tests
// which would have caught the mutation — so the region says which filter ran
// and stops. A sentence about what the filtered run showed of the suite would
// be exactly the claim the section refuses, and §5.3.5's guarantee stated
// beside it would be stated more widely than the empty selection it catches.
//
// The filter row is absent when the run carried no filter, as §5.5 makes the
// column optional and the record omits it. A row carrying an invented value
// standing for "no filter" would put a phrase cr composed into the one region
// that exists to be checked.
//
// What is not here belongs to two other tasks, and is left whole to them rather
// than half-stated: the probe's `input` with its `post.max_probe_input_bytes`
// cap and truncation notice, the `cited` half of §8.1.7, and the limit §5.4.4
// concedes for a gap probe are the evidence region's own; the placement of this
// region beneath the agent body, the rest of §8.1.3's sequence, and the
// regeneration at render and post time are the comment sequence's.
func ProbeEvidence(record *probe.Record) string {
	var out strings.Builder
	out.WriteString(evidenceOpen + "\n")
	fmt.Fprintf(&out, "kind: %s\n", record.Kind)
	fmt.Fprintf(&out, "target: %s\n", record.Target)
	if record.Filter != "" {
		fmt.Fprintf(&out, "filter: %s\n", record.Filter)
	}
	fmt.Fprintf(&out, "result: %s\n", record.Result)
	out.WriteString("output_tail:\n")
	out.WriteString(fenced(record.OutputTail))
	out.WriteString(evidenceClose)
	return out.String()
}

// fenced wraps the runner's output in a code fence long enough to hold it.
//
// The tail is whatever the runner printed, and a runner that printed a fence of
// its own would otherwise close the block early and leave the rest of its
// output to be read as the comment's own prose — which in this region means
// read as something cr asserted. The fence is one backtick longer than the
// longest run the tail holds, and never shorter than three, which is the rule
// CommonMark already gives for nesting one fence inside another.
func fenced(tail string) string {
	fence := strings.Repeat("`", max(3, longestBacktickRun(tail)+1))
	if tail != "" && !strings.HasSuffix(tail, "\n") {
		tail += "\n"
	}
	return fence + "\n" + tail + fence + "\n"
}

// longestBacktickRun is the length of the longest unbroken run of backticks in
// s, and zero when it holds none.
func longestBacktickRun(s string) int {
	longest, current := 0, 0
	for _, char := range s {
		if char != '`' {
			current = 0
			continue
		}
		current++
		longest = max(longest, current)
	}
	return longest
}
