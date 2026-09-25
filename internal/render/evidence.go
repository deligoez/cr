package render

import (
	"fmt"
	"regexp"
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

// evidenceFields are the evidence region's field names in one language.
//
// Each is §5.5's or §6.1's field, and a Turkish name is that field's rather
// than a phrase cr composed, so the author still checks the region against the
// state cr wrote. The values beneath them — a probe's kind, its `result`, a
// path — are cr's vocabulary and are written as the record holds them.
type evidenceFields struct {
	// lang is the language the row is written in.
	lang Lang
	// kind, target, filter, paths and result name §5.5's fields.
	kind, target, filter, paths, result string
	// input names §5.5's `input`, and truncated is the row standing in its
	// place when the input was cut: a format over the bytes shown, the
	// bytes held, and the setting that cut it, in that order.
	input, truncated string
	// outputTail names §5.5's `output_tail`, and counted the row standing in
	// its place when §8.1.7 shows the lines tests.count_pattern matches.
	outputTail, counted string
	// citation names one stored citation of §6.1.
	citation string
	// rerun names a probe whose `rerun_of` names the record's probe.
	rerun string
	// limit is what §5.4.4 concedes about a gap probe, carried into the
	// region for one as a whole row: cr cannot tell a wrong behaviour from a
	// wrong test, and the author is the only one left to make that
	// distinction. The English row is §5.4.4's own sentence rather than one
	// cr composed, and the Turkish row says the same, so a comment in
	// render.lang carries no English line an author would have to edit out.
	// Each row is one literal, not a concatenation, so no mutant can land
	// in an expression no coverage block reaches.
	limit string
}

// evidenceFieldNames is §8.1.7's field names for every language §8.1.1
// admits, built in and not configurable, as §8.1.4's labels are.
//
// Measured with cr 0.15.0 on a real pull request on a private Laravel
// repository: under render.lang `tr` the region still read `kind:`,
// `target:`, `paths:`, `result:` and `input:` in an otherwise Turkish comment.
var evidenceFieldNames = []evidenceFields{
	{
		lang: LangEN,
		kind: "kind", target: "target", filter: "filter", paths: "paths", result: "result",
		input:      "input",
		truncated:  "input (truncated to %[1]d of %[2]d bytes by %[3]s)",
		outputTail: "output_tail",
		counted:    "output_tail, the lines tests.count_pattern matches",
		citation:   "citation",
		rerun:      "rerun",
		limit:      "limit: a failed gap probe means either the behaviour is wrong or the supplied test is wrong, and cr cannot distinguish the two",
	},
	{
		lang: LangTR,
		kind: "tür", target: "hedef", filter: "filtre", paths: "yollar", result: "sonuç",
		input:      "girdi",
		truncated:  "girdi (%[3]s gereği %[2]d bayttan %[1]d bayta kısaltıldı)",
		outputTail: "test çıktısı",
		counted:    "test çıktısı (özet)",
		citation:   "kaynak",
		rerun:      "yeniden koşu",
		limit:      "sınır: başarısız bir boşluk deneyi, ya davranışın ya da denenen testin hatalı olduğunu gösterir; cr hangisi olduğunu ayırt edemez",
	},
}

// fieldsIn is the evidence region's field names in lang, and English's for a
// language the table holds no row for or when BodyComment, the body the region
// is part of, is not one Setting's language governs: English is the language
// §8.1.2 stores a record in, so a region in it still names the fields the
// state holds.
func fieldsIn(lang Lang) *evidenceFields {
	if !BodyComment.AuthorFacing() {
		return &evidenceFieldNames[0]
	}
	for i := range evidenceFieldNames {
		if evidenceFieldNames[i].lang == lang {
			return &evidenceFieldNames[i]
		}
	}
	return &evidenceFieldNames[0]
}

// CitedEvidence renders §8.1.7's evidence region for a record graded `cited`:
// each stored citation as `path:line`, in the order the record holds them,
// then the re-runs of the probe the record names, and nothing else. Its field
// names are lang's, per evidenceFieldNames.
//
// It is template substitution, as ProbeEvidence is. The author opens each
// location and judges whether it supports the body, which is all §6.2.4 lets a
// citation establish; a word of cr's about what the location shows would be
// the judgement §6.2.4 withholds.
//
// A record carrying no citation and no re-run has no region, since the
// region's whole content is those rows. §6.2 grades no record `cited` without
// a citation, so this is a record cr did not grade rather than an assertion
// stripped of its support.
func CitedEvidence(lang Lang, record string, citations []finding.Citation, reruns []*probe.Record) (string, error) {
	if len(citations) == 0 {
		return RerunEvidence(lang, record, reruns)
	}
	fields := fieldsIn(lang)
	var out strings.Builder
	for _, cited := range citations {
		fmt.Fprintf(&out, "%s: %s:%d\n", fields.citation, cited.Path, cited.Line)
	}
	rerunRows(&out, fields, reruns)
	return evidenceRegion.checked(record, strings.TrimSuffix(out.String(), "\n"))
}

// RerunEvidence renders §8.1.7's evidence region for a record that asserts
// nothing, `argued` among them, and is empty unless the probe the record names
// has re-runs at the round's head.
//
// §8.1.7 carries the re-runs whatever the grade. Measured on
// tarfin-labs/backend#6328 with cr 0.13.0: a question whose probe a `--rerun`
// had since refuted by experiment reached the draft with nothing saying so.
func RerunEvidence(lang Lang, record string, reruns []*probe.Record) (string, error) {
	if len(reruns) == 0 {
		return "", nil
	}
	var out strings.Builder
	rerunRows(&out, fieldsIn(lang), reruns)
	return evidenceRegion.checked(record, strings.TrimSuffix(out.String(), "\n"))
}

// rerunRows writes one row per re-run: its id and its `result`, under §5.5's
// field names in fields' language, in the order given.
func rerunRows(out *strings.Builder, fields *evidenceFields, reruns []*probe.Record) {
	for _, rerun := range reruns {
		fmt.Fprintf(out, "%s: %s, %s: %s\n", fields.rerun, rerun.ID, fields.result, rerun.Result)
	}
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
// A run in which nothing failed shows, in place of its output tail, the lines
// of it countPattern — the profile's tests.count_pattern — matches. Measured
// on tarfin-labs/backend#6328 with cr 0.13.0: a probed comment carried sixty
// lines of passing tests. With no pattern to match by, the tail is shown
// whole. The probe's re-runs at the round's head follow, one row each.
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
func ProbeEvidence(
	lang Lang, record string, p *probe.Record, maxInput int, countPattern string, reruns []*probe.Record,
) (string, error) {
	fields := fieldsIn(lang)
	var out strings.Builder
	fmt.Fprintf(&out, "%s: %s\n", fields.kind, p.Kind)
	fmt.Fprintf(&out, "%s: %s\n", fields.target, p.Target)
	if p.Filter != "" {
		fmt.Fprintf(&out, "%s: %s\n", fields.filter, p.Filter)
	}
	for _, path := range p.Paths {
		fmt.Fprintf(&out, "%s: %s\n", fields.paths, path)
	}
	fmt.Fprintf(&out, "%s: %s\n", fields.result, p.Result)
	if p.Kind == probe.Gap {
		out.WriteString(fields.limit + "\n")
	}
	input, truncated := capped(p.Input, maxInput)
	if truncated {
		fmt.Fprintf(&out, fields.truncated+":\n", len(input), len(p.Input), MaxProbeInputSetting)
	} else {
		out.WriteString(fields.input + ":\n")
	}
	out.WriteString(fenced(input))
	if counted, ok := countedLines(p, countPattern); ok {
		out.WriteString(fields.counted + ":\n")
		out.WriteString(fenced(counted))
	} else {
		out.WriteString(fields.outputTail + ":\n")
		out.WriteString(fenced(p.OutputTail))
	}
	rerunRows(&out, fields, reruns)
	return evidenceRegion.checked(record, out.String())
}

// countedLines is the lines of the probe's output tail countPattern matches,
// and whether §8.1.7 shows them in place of the tail: only for a run in which
// nothing failed, and only with a pattern that compiles.
func countedLines(p *probe.Record, countPattern string) (string, bool) {
	if !p.Result.NothingFailed() || countPattern == "" {
		return "", false
	}
	pattern, err := regexp.Compile(countPattern)
	if err != nil {
		return "", false
	}
	var kept strings.Builder
	for line := range strings.Lines(p.OutputTail) {
		if pattern.MatchString(line) {
			kept.WriteString(line)
		}
	}
	return kept.String(), true
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
