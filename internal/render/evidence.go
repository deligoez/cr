package render

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
)

// The named pair §8.1.3 gives the evidence region.
//
// A pair rather than a count: §8.1.3 has recovery "never depend on counting",
// so the region is found by its own opening and closing marker wherever it
// sits. Where it sits and which regions surround it are Comment's; the refusal
// of an agent body carrying a `<!-- cr:` sequence is ValidateBody's.
const (
	evidenceOpen  = "<!-- cr:evidence -->"
	evidenceClose = "<!-- cr:/evidence -->"
)

// MaxProbeInputSetting is the §2.7 key capping the probe input the evidence
// region carries. Its default lives in internal/config's table beside every
// other key's, and the key lives here, beside the one region that reads it, so
// the two cannot drift apart.
const MaxProbeInputSetting = "post.max_probe_input_bytes"

// gapLimit is what §5.4.4 concedes about a gap probe, carried into the region
// for one: cr cannot tell a wrong behaviour from a wrong test, and the author
// is the only one left to make that distinction. It is §5.4.4's own sentence
// rather than one cr composed, so the region still says nothing the spec does
// not already say cr knows — and it is English for the reason every field name
// beside it is: it is the record's vocabulary, not reader-facing prose.
//
// It is one literal, not a concatenation, so no mutant can land in a constant
// expression that no coverage block reaches.
const gapLimit = "limit: a failed gap probe means either the behaviour is wrong or the supplied test is wrong, and cr cannot distinguish the two"

// CitedEvidence renders §8.1.7's evidence region for a record graded `cited`:
// each stored citation as `path:line`, in the order the record holds them, and
// nothing else.
//
// It is template substitution, as ProbeEvidence is. The author opens each
// location and judges whether it supports the body, which is all §6.2.4 lets a
// citation establish; a word of cr's about what the location shows would be
// the judgement §6.2.4 withholds.
//
// A record carrying no citation has no region, since the region's whole
// content is its citations. §6.2 grades no record `cited` without one, so this
// is a record cr did not grade rather than an assertion stripped of its
// support.
func CitedEvidence(record string, citations []finding.Citation) (string, error) {
	if len(citations) == 0 {
		return "", nil
	}
	lines := make([]string, 0, len(citations))
	for _, cited := range citations {
		lines = append(lines, fmt.Sprintf("citation: %s:%d", cited.Path, cited.Line))
	}
	return evidenceRegion.checked(record, strings.Join(lines, "\n"))
}

// ProbeEvidence renders §8.1.7's evidence region for a record whose `probed`
// grade rests on this probe.
//
// It is template substitution and nothing else. §8.1.7 names what the region
// carries for a probe — `kind`, `target`, `filter`, `paths`, `result` and
// `output_tail` — and each is written out as the record holds it, under §5.5's
// own field name, so the author checks the comment against the state cr wrote
// rather than against a phrasing cr chose. `paths` is a list and takes a row
// each, rather than one row holding a separator cr invented: a path may carry
// any character a file name may, and a reader of a joined row could not tell
// which of them belonged to cr.
//
// Round 9's unverifiable-evidence-region adds §5.5's `input`, for both kinds:
// the patch or the test file cr actually executed. Without it an inert mutation
// — a comment, a whitespace edit, dead code — produces a genuine
// `no-test-failed` and posts as "this line is untested" with nothing the author
// can inspect, and a failed gap probe shows its output without the test that
// produced it. The input is capped at maxInput bytes, the resolved
// post.max_probe_input_bytes, and a truncated input says so on its own row, so
// a cut never reads as the whole experiment.
//
// A gap probe carries §5.4.4's limit too, beneath its result: cr cannot
// distinguish a wrong behaviour from a wrong test, and round 11's
// undisclosed-evidence-limit has the reader told so rather than left to assume
// cr checked.
//
// Nothing is composed, concluded or summarised here, and that is §5.3.6's
// requirement as much as §8.1.7's. A filtered or path-narrowed run proves the
// gap only for the tests it selected, and cr cannot establish that a filter or
// its paths select the tests which would have caught the mutation — so the
// region says which filter and which paths ran and stops.
//
// The filter row is absent when the run carried no filter, and there are no
// path rows when it carried no path, as §5.5 makes both columns optional and
// the record omits them. A row carrying an invented value standing for "no
// filter" would put a phrase cr composed into the one region that exists to be
// checked.
//
// The input and the output tail are text cr did not write, and AgentRegion
// ends the region at the first closing marker it meets. So a region that would
// carry Reserved is refused naming the record, under §8.1.3's rejection of a
// body holding it, rather than written into a draft that reads back wrong.
func ProbeEvidence(record string, p *probe.Record, maxInput int) (string, error) {
	var out strings.Builder
	fmt.Fprintf(&out, "kind: %s\n", p.Kind)
	fmt.Fprintf(&out, "target: %s\n", p.Target)
	if p.Filter != "" {
		fmt.Fprintf(&out, "filter: %s\n", p.Filter)
	}
	for _, path := range p.Paths {
		fmt.Fprintf(&out, "paths: %s\n", path)
	}
	fmt.Fprintf(&out, "result: %s\n", p.Result)
	if p.Kind == probe.Gap {
		out.WriteString(gapLimit + "\n")
	}
	input, truncated := capped(p.Input, maxInput)
	if truncated {
		fmt.Fprintf(&out, "input (truncated to %d of %d bytes by %s):\n",
			len(input), len(p.Input), MaxProbeInputSetting)
	} else {
		out.WriteString("input:\n")
	}
	out.WriteString(fenced(input))
	out.WriteString("output_tail:\n")
	out.WriteString(fenced(p.OutputTail))
	return evidenceRegion.checked(record, out.String())
}

// capped is input cut to at most limit bytes, and whether anything was cut.
//
// The cut falls back to the start of the rune it lands inside, so a multi-byte
// character is dropped whole rather than split into bytes that no longer
// decode — the region would otherwise carry invalid UTF-8 into a comment. A
// limit below zero shows nothing, as a limit of zero does.
func capped(input string, limit int) (string, bool) {
	if len(input) <= limit {
		return input, false
	}
	cut := max(limit, 0)
	for cut > 0 && !utf8.RuneStart(input[cut]) {
		cut--
	}
	return input[:cut], true
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
