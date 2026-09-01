package draft

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/finding"
)

// The grammar of §7.1.1's record marker, fixed here and nowhere else.
//
// One line, exactly, and MarkerGrammar below is that line:
//
//	<!-- cr:record id="f1" kind="question" path="internal/api/handler.go" start_line="42" line="44" severity="high" grade="argued" disposition="" -->
//
// Rule 1. The line opens with `<!-- cr:record` and closes with `-->`.
//
// Rule 2. Between them come §7.1.1's eight fields — `id`, `kind`, `path`,
// `start_line`, `line`, `severity`, `grade`, `disposition` — all eight, each
// exactly once, in that order.
//
// Rule 3. Each field is one SPACE, the field name, `=`, and a JSON string
// literal. A value may be empty; `disposition` is, until §7.2's table has the
// reviewer type `wrong` into it.
//
// Rule 4. `start_line` and `line` hold a decimal integer as the string's
// content.
//
// Rule 5. Malformed is any deviation from the four rules above: a missing
// field, an extra field, a reordered field, a value that is not a JSON string,
// a non-integer line number, a delimiter that is not exactly these delimiters,
// or anything at all before the opening `<!--` or after the closing `-->`.
//
// The prefix is `<!-- cr:`, the sequence §8.1.3 reserves for cr-owned regions,
// and that is a deliberate choice rather than an accident of spelling. §8.1.3
// rejects with exit code 1 any body carrying a `<!-- cr:` sequence, so no block
// body may hold a string shaped like a marker — which makes "a marker" and
// "text that looks like one" the same set, and leaves §7.2.1, §7.2.3 and
// §8.1.3 one line to branch on rather than two. A prefix of cr's own would be a
// second reserved sequence with no rule keeping it out of a body, and every
// ambiguity that followed would be ours to invent an answer for.
//
// That is why rule 5 makes malformed an error and never prose. A line is judged
// to *claim* to be a marker on its opening token alone, so exactly one of three
// things happens to any line of a draft: it does not open with the token and is
// body text, it opens with the token and matches the grammar and is a marker,
// or it opens with the token and does not match and is refused with the line
// named per §7.2.1. There is no fourth reading in which a near-marker is
// quietly treated as prose — which is the reading that would let a reviewer's
// typo delete a record from the round without anyone being told.
const (
	markerOpen  = "<!-- cr:record"
	markerClose = "-->"
	// markerClaim is what a line must carry to be judged as a marker at
	// all, per rule 5's third reading.
	markerClaim = markerOpen + " "
)

// MarkerGrammar is the one example line the rules above are written against,
// so a caller reporting the grammar quotes it rather than restating it.
const MarkerGrammar = `<!-- cr:record id="f1" kind="question" ` +
	`path="internal/api/handler.go" start_line="42" line="44" ` +
	`severity="high" grade="argued" disposition="" -->`

// markerFieldNames is §7.1.1's field list, in the order the section writes it.
// It is the order the marker is rendered in and the order it is parsed in, so
// the two cannot come to disagree about what "in that order" means.
var markerFieldNames = []string{
	"id", "kind", "path", "start_line", "line", "severity", "grade", "disposition",
}

// MarkerFields returns §7.1.1's field names in section order. The result is a
// copy, so a caller can neither widen the set nor reorder it.
func MarkerFields() []string {
	return append(make([]string, 0, len(markerFieldNames)), markerFieldNames...)
}

// Marker is one record's §7.1.1 marker: the eight fields, as text.
//
// The vocabulary fields — `kind`, `severity`, `grade`, `disposition` — are held
// as strings rather than as their finding types, because this is the grammar
// and not the judgement. §7.2's table decides what a reviewer may edit each of
// them to and what happens when they name something outside §6.1's vocabulary;
// a parser that refused an unknown severity would answer that question here,
// with the wrong error and in the wrong section.
//
// The two line numbers are integers, because that is grammar: rule 4 makes a
// non-integer a deviation, so a Marker that exists has already been past it.
type Marker struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	StartLine   int    `json:"start_line"`
	Line        int    `json:"line"`
	Severity    string `json:"severity"`
	Grade       string `json:"grade"`
	Disposition string `json:"disposition"`
}

// markerOf is the marker of one record, read off the record's own fields.
//
// The three location fields come from the anchor §9.2 recorded rather than
// being restated, so the marker names the location the record carries and there
// is no second copy of it to drift.
func markerOf(record *finding.Finding) Marker {
	return Marker{
		ID:          record.ID,
		Kind:        string(record.Kind),
		Path:        record.Anchor.Path,
		StartLine:   record.Anchor.StartLine,
		Line:        record.Anchor.Line,
		Severity:    string(record.Severity),
		Grade:       string(record.Grade),
		Disposition: string(record.Disposition),
	}
}

// values are the marker's eight fields in markerFieldNames' order, as the
// strings the grammar quotes.
func (m *Marker) values() []string {
	return []string{
		m.ID, m.Kind, m.Path,
		strconv.Itoa(m.StartLine), strconv.Itoa(m.Line),
		m.Severity, m.Grade, m.Disposition,
	}
}

// String renders the marker to the grammar above.
//
// Every field is written even when its value is empty, `disposition` being the
// one that always is at draft time: §7.2's table gives it an edit semantics the
// reviewer reaches by typing `wrong` into it, and a field that appeared only
// once it had a value would leave the verb nowhere to be written.
func (m Marker) String() string {
	var out strings.Builder
	out.WriteString(markerOpen)
	for i, value := range m.values() {
		out.WriteString(" " + markerFieldNames[i] + "=" + quote(value))
	}
	out.WriteString(" " + markerClose)
	return out.String()
}

// IsMarkerLine reports whether a line claims to be a marker.
//
// The claim is the opening token and nothing else, which is what makes the
// three readings above disjoint: a line that carries it is either a marker or
// malformed, and a line that does not is prose whatever else it holds.
func IsMarkerLine(line string) bool {
	return strings.HasPrefix(line, markerClaim)
}

// MalformedMarkerError reports a line deviating from the grammar above.
//
// §7.2.1 has `cr post` refuse to run on one and name the line, so both the
// one-based number and the text are carried: the number is what the reviewer
// opens their editor at, and the text is what they compare against
// MarkerGrammar. The cli layer maps it onto exit code 1, which §7.2 fixes for
// every marker edit it does not admit.
type MalformedMarkerError struct {
	// At is the one-based line the marker sits on in the draft.
	At int
	// Line is the line as it was written.
	Line string
	// Problem completes the sentence naming the deviation.
	Problem string
}

func (e *MalformedMarkerError) Error() string {
	return fmt.Sprintf("draft line %d is a malformed record marker: %s\n  read: %s\n  grammar: %s",
		e.At, e.Problem, e.Line, MarkerGrammar)
}

// ParseMarker reads one line of a draft as a §7.1.1 marker.
//
// It is the whole of the triage input channel's syntax. §7.2 makes editing the
// draft the way a reviewer speaks to cr, §7.2.1 refuses a malformed marker,
// and §7.2.3 aborts on an id the round does not hold — all three read a line
// through this, so there is one answer about what a marker is rather than one
// per caller.
func ParseMarker(at int, line string) (Marker, error) {
	if !IsMarkerLine(line) {
		return Marker{}, &MalformedMarkerError{At: at, Line: line,
			Problem: "it does not open with " + markerClaim}
	}
	rest := strings.TrimPrefix(line, markerOpen)
	values := make([]string, 0, len(markerFieldNames))
	for _, name := range markerFieldNames {
		value, remainder, err := cutMarkerField(at, line, rest, name)
		if err != nil {
			return Marker{}, err
		}
		values = append(values, value)
		rest = remainder
	}
	if rest != " "+markerClose {
		return Marker{}, &MalformedMarkerError{At: at, Line: line, Problem: fmt.Sprintf(
			"§7.1.1's eight fields are followed by %q, and the grammar ends with %q",
			rest, " "+markerClose)}
	}
	return markerFrom(at, line, values)
}

// cutMarkerField takes one `name="value"` field off the front of rest.
//
// The field name is required rather than discovered, which is what rule 2
// means by "in that order": a marker naming the right eight fields in another
// order fails here, at the first one that is not where §7.1.1 puts it, and so
// does one carrying a ninth.
func cutMarkerField(at int, line, rest, name string) (value, remainder string, err error) {
	after, named := strings.CutPrefix(rest, " "+name+"=")
	if !named {
		return "", "", &MalformedMarkerError{At: at, Line: line, Problem: fmt.Sprintf(
			"%q does not follow, and §7.1.1 fixes the eight fields and their order", " "+name+"=")}
	}
	value, remainder, quoted := cutQuoted(after)
	if !quoted {
		return "", "", &MalformedMarkerError{At: at, Line: line,
			Problem: name + " carries no JSON string"}
	}
	return value, remainder, nil
}

// cutQuoted takes one JSON string literal off the front of s and returns the
// text it holds together with what follows it.
//
// The closing quote is found by scanning rather than searched for, so an
// escaped quote inside a value does not end it early — which is the whole
// reason values are JSON strings: a path is the one field cr does not choose,
// and it may hold a quote, a space, or a newline.
func cutQuoted(s string) (value, rest string, ok bool) {
	if !strings.HasPrefix(s, `"`) {
		return "", "", false
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			var decoded string
			if err := json.Unmarshal([]byte(s[:i+1]), &decoded); err != nil {
				return "", "", false
			}
			return decoded, s[i+1:], true
		}
	}
	return "", "", false
}

// markerFrom assembles the parsed values into a Marker, applying rule 4 to the
// two line numbers.
func markerFrom(at int, line string, values []string) (Marker, error) {
	start, err := markerNumber(at, line, "start_line", values[3])
	if err != nil {
		return Marker{}, err
	}
	end, err := markerNumber(at, line, "line", values[4])
	if err != nil {
		return Marker{}, err
	}
	return Marker{
		ID: values[0], Kind: values[1], Path: values[2],
		StartLine: start, Line: end,
		Severity: values[5], Grade: values[6], Disposition: values[7],
	}, nil
}

// markerNumber reads one of the two line fields as rule 4 writes it: a decimal
// integer and nothing else.
//
// The round trip is what closes the loop. strconv.Atoi accepts a leading sign
// and leading zeros, and a marker reading `line="+044"` would then parse to a
// number this renderer could never have written — so the value has to be
// exactly what strconv.Itoa gives back.
func markerNumber(at int, line, name, raw string) (int, error) {
	number, err := strconv.Atoi(raw)
	if err != nil || raw != strconv.Itoa(number) {
		return 0, &MalformedMarkerError{At: at, Line: line, Problem: fmt.Sprintf(
			"%s holds %q, and the grammar gives it a decimal integer", name, raw)}
	}
	return number, nil
}

// quote renders one marker value as a JSON string.
//
// JSON rather than a bare word, for the reason cutQuoted gives. Go's encoder
// also escapes `<`, `>` and `&`, which is what keeps `-->` out of every value
// and leaves the marker's own terminator unambiguous.
//
// The error is discarded because there is none to report: encoding/json
// replaces invalid UTF-8 with the replacement rune rather than failing, so a
// string always encodes.
func quote(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
