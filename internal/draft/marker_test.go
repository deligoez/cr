package draft

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aMarker is the grammar's own example line, as a Marker.
func aMarker() Marker {
	return Marker{
		ID: "f1", Kind: "question", Path: "internal/api/handler.go",
		StartLine: 42, Line: 44,
		Severity: "high", Grade: "argued", Disposition: "",
	}
}

// The documented grammar and the renderer are the same grammar.
//
// MarkerGrammar is what a refusal quotes at the reviewer, so a renderer that
// had drifted from it would be teaching them a syntax cr no longer writes.
func TestTheExampleLineIsWhatTheRendererProduces(t *testing.T) {
	assert.Equal(t, MarkerGrammar, aMarker().String())
}

// §7.1.1 names eight fields, and the marker carries all eight in that order —
// `disposition` included, which is empty at draft time and is what §7.2's table
// has the reviewer type `wrong` into.
func TestTheMarkerCarriesEveryFieldSection711NamesInOrder(t *testing.T) {
	require.Equal(t, []string{
		"id", "kind", "path", "start_line", "line", "severity", "grade", "disposition",
	}, MarkerFields(), "§7.1.1's list, in the section's own order")

	rendered := aMarker().String()
	at := -1
	for _, name := range MarkerFields() {
		next := strings.Index(rendered, " "+name+"=\"")
		assert.Greater(t, next, at, "%s is written, and after the field before it", name)
		at = next
	}
	example := aMarker()
	assert.Equal(t, len(MarkerFields()), len(example.values()),
		"the rendered values and the field names are one list read twice")
}

// Every field survives the round trip, values cr does not choose included.
//
// The path is the field that carries whatever the repository under review holds
// — a space, a quote, a newline, or the marker's own terminator — and rule 5
// makes any of them ending the marker early a malformed line rather than a
// wrong one. Round-tripping is how the renderer and the parser are held to one
// grammar instead of two that agree on the easy cases.
func TestEveryFieldSurvivesTheRoundTrip(t *testing.T) {
	for name, path := range map[string]string{
		"a plain path": "internal/api/handler.go",
		"a space":      "internal/api/a handler.go",
		"a quote":      `internal/api/"handler".go`,
		"a newline":    "internal/api/handler.go\nnot a marker",
		"a backslash":  `internal\api\handler.go`,
		"a terminator": `internal/api/x.go --> <!-- cr:record id="f9" -->`,
	} {
		t.Run(name, func(t *testing.T) {
			want := aMarker()
			want.Path = path
			want.Disposition = "wrong"

			rendered := want.String()
			require.Equal(t, 1, len(strings.Split(rendered, "\n")),
				"a marker is one line, whatever the value holds")

			got, err := ParseMarker(3, rendered)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

// A rendered record parses back to the record's own fields, so the marker cr
// writes is a marker cr can read.
func TestARenderedRecordParsesBackToItsFields(t *testing.T) {
	record := aRecord("f7")
	record.Anchor.Path = `internal/api/a "handler".go`

	rendered := renderOf(t, record)
	marker, err := ParseMarker(1, strings.Split(rendered, "\n")[0])
	require.NoError(t, err)

	assert.Equal(t, record.ID, marker.ID)
	assert.Equal(t, string(record.Kind), marker.Kind)
	assert.Equal(t, record.Anchor.Path, marker.Path)
	assert.Equal(t, record.Anchor.StartLine, marker.StartLine)
	assert.Equal(t, record.Anchor.Line, marker.Line)
	assert.Equal(t, string(record.Severity), marker.Severity)
	assert.Equal(t, string(record.Grade), marker.Grade)
	assert.Empty(t, marker.Disposition)
}

// Rule 5: malformed is any deviation from the grammar, and each deviation is
// refused with the line named per §7.2.1.
//
// The cases are one deviation each, against the same example line, so a refusal
// proves the rule it is under rather than some second thing the fixture never
// had. A missing field, an extra field, a reordered pair, a broken delimiter
// and a line number that is not an integer are the five shapes rule 5 lists.
func TestEveryDeviationFromTheGrammarIsMalformed(t *testing.T) {
	sound := MarkerGrammar
	for name, line := range map[string]string{
		"a missing field": strings.Replace(sound, ` grade="argued"`, "", 1),
		"an extra field": strings.Replace(sound,
			` disposition=""`, ` disposition="" note="mine"`, 1),
		"a reordered pair": strings.Replace(sound,
			`id="f1" kind="question"`, `kind="question" id="f1"`, 1),
		"an unquoted value":     strings.Replace(sound, `kind="question"`, `kind=question`, 1),
		"a missing terminator":  strings.TrimSuffix(sound, " -->"),
		"a foreign terminator":  strings.Replace(sound, " -->", " //>", 1),
		"a doubled separator":   strings.Replace(sound, ` line="44"`, `  line="44"`, 1),
		"trailing text":         sound + " and then some",
		"leading indentation":   "  " + sound,
		"a fractional line":     strings.Replace(sound, `line="44"`, `line="44.0"`, 1),
		"a signed line":         strings.Replace(sound, `line="44"`, `line="+44"`, 1),
		"a zero-padded line":    strings.Replace(sound, `line="44"`, `line="044"`, 1),
		"a non-numeric line":    strings.Replace(sound, `line="44"`, `line="forty"`, 1),
		"the wrong open token":  strings.Replace(sound, "<!-- cr:record ", "<!-- cr:block ", 1),
		"no open token at all":  strings.TrimPrefix(sound, "<!-- cr:record "),
		"an empty required key": strings.Replace(sound, ` id="f1"`, ` ="f1"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			require.NotEqual(t, sound, line, "the case has to deviate from the grammar")

			_, err := ParseMarker(9, line)
			require.Error(t, err)

			var malformed *MalformedMarkerError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, 9, malformed.At, "§7.2.1: the refusal names the line")
			assert.Contains(t, err.Error(), MarkerGrammar,
				"and quotes the grammar the reviewer has to match")
		})
	}
}

// A marker-like string takes exactly one of the three documented readings, and
// the three are disjoint.
//
// This is the ambiguity rule 5 exists to close. A line either does not claim to
// be a marker and is body text, or claims to be one and is parsed, or claims to
// be one and is refused — and nothing lands in two of those at once. The
// dangerous reading is the fourth one that does not exist here: a near-marker
// quietly treated as prose, which would drop a record out of the round with
// nobody told.
func TestAMarkerLikeStringTakesExactlyOneReading(t *testing.T) {
	readings := func(line string) []string {
		taken := make([]string, 0, 2)
		if !IsMarkerLine(line) {
			taken = append(taken, "body text")
		}
		if _, err := ParseMarker(1, line); err == nil {
			taken = append(taken, "a marker")
		} else if IsMarkerLine(line) {
			taken = append(taken, "malformed")
		}
		return taken
	}

	for name, want := range map[string]struct {
		line    string
		reading string
	}{
		"the grammar itself":     {MarkerGrammar, "a marker"},
		"ordinary prose":         {"The error Decode returns is dropped.", "body text"},
		"prose naming a marker":  {"The block opens with a <!-- cr:record --> marker.", "body text"},
		"an indented near-miss":  {"    " + MarkerGrammar, "body text"},
		"a truncated marker":     {`<!-- cr:record id="f1" -->`, "malformed"},
		"a marker with a typo":   {strings.Replace(MarkerGrammar, `line=`, `lines=`, 1), "malformed"},
		"an unterminated marker": {strings.TrimSuffix(MarkerGrammar, "-->"), "malformed"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, []string{want.reading}, readings(want.line),
				"exactly one reading, and it is the documented one")
		})
	}
}

// The two line numbers reach the parser as numbers, so §7.2's re-validation of
// `path`, `start_line` and `line` has an anchor to check rather than text to
// re-read.
func TestTheLineNumbersParseAsNumbers(t *testing.T) {
	want := aMarker()
	want.StartLine, want.Line = 1, 1000000

	got, err := ParseMarker(1, want.String())
	require.NoError(t, err)
	assert.Equal(t, 1, got.StartLine)
	assert.Equal(t, 1000000, got.Line)
	assert.Contains(t, want.String(), ` line="`+strconv.Itoa(want.Line)+`"`)
}
