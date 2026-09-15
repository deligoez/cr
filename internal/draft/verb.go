package draft

import (
	"fmt"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// Verb is one of §7.2.4's four triage verbs, as `cr triage` takes it.
type Verb string

// §7.2.4's verbs, each the name of the hand edit it makes.
const (
	// VerbNotHere deletes the block, §7.2's discard as `not-here`.
	VerbNotHere Verb = "not-here"
	// VerbWrong sets `disposition=wrong` in the block's marker.
	VerbWrong Verb = "wrong"
	// VerbSoften changes the marker's `kind=finding` to `kind=question`.
	VerbSoften Verb = "soften"
	// VerbKeep changes no marker field.
	VerbKeep Verb = "keep"
)

// verbs is §7.2.4's list, in the order the section writes it.
var verbs = []Verb{VerbNotHere, VerbWrong, VerbSoften, VerbKeep}

// ParseVerb reads a verb as it was typed on the command line.
//
// A word outside §7.2.4's four is a malformed invocation rather than bad
// input data, so the error is a plain one: §11.2 codes every error no row
// claims 2.
func ParseVerb(word string) (Verb, error) {
	if slices.Contains(verbs, Verb(word)) {
		return Verb(word), nil
	}
	names := make([]string, 0, len(verbs))
	for _, verb := range verbs {
		names = append(names, string(verb))
	}
	return "", fmt.Errorf("%q is not a triage verb: §7.2.4 names %s", word, strings.Join(names, ", "))
}

// KeepsBlock reports whether the verb leaves a block whose agent region
// `--body-file` could replace. §7.2.4 refuses the flag beside the two that
// discard, since a discarded record's body reaches nobody.
func (v Verb) KeepsBlock() bool {
	return v == VerbSoften || v == VerbKeep
}

// NoBlockError is §7.2.4's refusal of a record id the draft holds no block
// for: one never rendered, one already deleted, or one mistyped. The cli layer
// maps it onto exit code 1.
type NoBlockError struct {
	// ID is the record id `cr triage` was given.
	ID string
}

func (e *NoBlockError) Error() string {
	return fmt.Sprintf("the draft holds no block for record %s, so there is nothing to triage", e.ID)
}

// SoftenQuestionError is §7.2.4's refusal of `soften` on a block whose marker
// already reads `kind=question`. The cli layer maps it onto exit code 1.
type SoftenQuestionError struct {
	// ID is the record whose block was named.
	ID string
	// At is the one-based line its marker sits on.
	At int
}

func (e *SoftenQuestionError) Error() string {
	return fmt.Sprintf("record %s's marker at draft line %d already reads kind=%q, and §7.2.4 softens only kind=%q",
		e.ID, e.At, finding.KindQuestion, finding.KindFinding)
}

// Triaged is a draft with one §7.2.4 verb applied to one record's block, as
// the matching hand edit would leave it.
//
// The edit is made on the file's bytes and nowhere else, so everything the
// verb does not name stays exactly as the reviewer left it: `not-here` removes
// the block from its marker line up to the next marker line or the end of the
// file, `wrong` and `soften` replace one value of the marker line, and a body
// replaces the text between the block's cr-owned regions. Reading the draft
// back is then §7.2's ordinary reading, which is what makes the verb and the
// hand edit one effect.
//
// body is `--body-file`'s content, and nil when none was given. Its leading and
// trailing line feeds are dropped, because render.AgentRegion drops them from
// every block it reads and a region therefore never carries them. Nothing else
// about it is judged here: §8.1.3 is `cr draft`'s and `cr post`'s to apply to
// the region they read.
//
// A draft this cannot split into blocks by record — a malformed marker, or two
// blocks for one record — is refused as `cr draft` refuses it, since there is
// then no one block the verb names.
func Triaged(file, id string, verb Verb, body *string) (string, error) {
	read, err := readBlocks(file)
	if err != nil {
		return "", err
	}
	if _, err := indexBlocks(nil, read); err != nil {
		return "", err
	}
	i := slices.IndexFunc(read, func(block readBlock) bool { return block.marker.ID == id })
	if i < 0 {
		return "", &NoBlockError{ID: id}
	}
	block := &read[i]
	start := lineOffset(file, block.at)
	lineEnd := start + len(block.line)
	end := len(file)
	if i+1 < len(read) {
		end = lineOffset(file, read[i+1].at)
	}
	line := block.line
	switch verb {
	case VerbNotHere:
		return file[:start] + file[end:], nil
	case VerbWrong:
		line, err = withMarkerValue(line, "disposition", string(finding.DispositionWrong))
	case VerbSoften:
		if block.marker.Kind == string(finding.KindQuestion) {
			return "", &SoftenQuestionError{ID: id, At: block.at}
		}
		line, err = withMarkerValue(line, "kind", string(finding.KindQuestion))
	}
	if err != nil {
		return "", err
	}
	text := file[lineEnd:end]
	if body != nil {
		text = withAgentRegion(text, strings.Trim(*body, "\n"))
	}
	return file[:start] + line + text + file[end:], nil
}

// lineOffset is the byte offset at which the one-based line at begins.
func lineOffset(file string, at int) int {
	offset := 0
	for range at - 1 {
		offset += strings.IndexByte(file[offset:], '\n') + 1
	}
	return offset
}

// withMarkerValue replaces the value of one field of a marker line that
// ParseMarker accepted, leaving every other byte of the line as it was.
func withMarkerValue(line, name, value string) (string, error) {
	rest := strings.TrimPrefix(line, markerOpen)
	for _, field := range markerFieldNames {
		_, remainder, err := cutMarkerField(0, line, rest, field)
		if err != nil {
			return "", err
		}
		if field == name {
			valueAt := len(line) - len(rest) + len(" "+field+"=")
			return line[:valueAt] + quote(value) + line[len(line)-len(remainder):], nil
		}
		rest = remainder
	}
	return "", fmt.Errorf("%s is not one of §7.1.1's marker fields", name)
}

// withAgentRegion replaces the agent region of one block's text — everything
// beneath its marker line — with body, leaving its cr-owned regions and the
// blank lines around them where they stand.
//
// The agent region is the text between the owned regions, which is where a
// block cr rendered holds it. Its first stretch holding more than line feeds
// takes body, and any later such stretch is emptied, so render.AgentRegion
// reads body back whichever way the reviewer rearranged the block. A block
// whose region was emptied takes body where §8.1.3's sequence puts it: above
// the evidence region, beneath the last region above the body, or after the
// blank line beneath the marker.
func withAgentRegion(text, body string) string {
	type stretch struct{ start, end int }
	cores := make([]stretch, 0)
	belowAt, lastEnd, cursor := -1, -1, 0
	gap := func(from, to int) {
		core := strings.Trim(text[from:to], "\n")
		if core == "" {
			return
		}
		at := from + strings.Index(text[from:to], core)
		cores = append(cores, stretch{at, at + len(core)})
	}
	for _, span := range render.OwnedSpans(text) {
		if span.Start > cursor {
			gap(cursor, span.Start)
		}
		if span.BeneathBody && belowAt < 0 {
			belowAt = span.Start
		}
		cursor = max(cursor, span.End)
		lastEnd = cursor
	}
	gap(cursor, len(text))
	switch {
	case len(cores) > 0:
		for i := len(cores) - 1; i > 0; i-- {
			text = text[:cores[i].start] + text[cores[i].end:]
		}
		return text[:cores[0].start] + body + text[cores[0].end:]
	case belowAt >= 0:
		return text[:belowAt] + body + "\n\n" + text[belowAt:]
	case lastEnd >= 0:
		return text[:lastEnd] + "\n\n" + body + text[lastEnd:]
	}
	lead := len(text) - len(strings.TrimLeft(text, "\n"))
	lead = min(lead, len("\n\n"))
	return "\n\n" + body + text[lead:]
}
