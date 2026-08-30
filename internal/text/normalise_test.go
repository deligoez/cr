package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nbsp is U+00A0 NO-BREAK SPACE: whitespace to Unicode, content to §1.4, whose
// steps 3 and 4 name U+0020 and U+0009 "and nothing else". It is built from its
// code point rather than typed into a literal, so a case that turns on it says
// which character it means instead of hiding an invisible byte in a string.
const nbsp = string(rune(0x00A0))

// replacementChar is U+FFFD, which a decoder also reports for a byte that
// decodes to nothing. Spelling it here keeps the two apart: this one is three
// bytes an author wrote on purpose, and §1.4 step 1 must let it through.
const replacementChar = string(rune(0xFFFD))

// §1.4 numbers six steps and calls the transformation "exactly this and no
// other", so the table below is written step by step rather than over whole
// realistic documents: a case exercising three steps at once cannot say which
// of them is wrong when it fails.
//
// Two of the cases are the ones a naive implementation gets wrong. Step 4 says
// "including leading indentation", so indentation collapses to one SPACE
// rather than surviving or disappearing; and steps 3 and 4 name U+0020 and
// U+0009 "and nothing else", so every other character Unicode calls whitespace
// is content here. A transform that also trimmed a non-breaking space would
// make two texts that differ hash alike, which is the one thing §1.4 exists to
// prevent.
func TestNormaliseAppliesTheSixStepsOfSection14(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"step 2: CRLF becomes a single LF":               {"a\r\nb", "a\nb"},
		"step 2: a lone CR becomes LF":                   {"a\rb", "a\nb"},
		"step 2: CR CR is two line endings, not one":     {"a\r\rb", "a\n\nb"},
		"step 3: trailing spaces and tabs go":            {"a \t \nb\t", "a\nb"},
		"step 3: no other trailing whitespace goes":      {"a" + nbsp, "a" + nbsp},
		"step 4: an interior run becomes one SPACE":      {"a \t  b", "a b"},
		"step 4: leading indentation becomes one SPACE":  {"\t\tif x {", " if x {"},
		"step 4: a single SPACE is already collapsed":    {"a b", "a b"},
		"step 4: no other whitespace collapses":          {"a" + nbsp + nbsp + "b", "a" + nbsp + nbsp + "b"},
		"step 5: leading blank lines are dropped":        {"\n\n\na", "a"},
		"step 5: trailing blank lines are dropped":       {"a\n\n\n", "a"},
		"step 5: an interior run collapses to one":       {"a\n\n\n\nb", "a\n\nb"},
		"step 5: a whitespace-only line is blank":        {"a\n \t \nb", "a\n\nb"},
		"step 5: a document of blank lines empties":      {"\n \n\t\n", ""},
		"step 6: no trailing LF survives":                {"a\n", "a"},
		"step 6: the empty document stays empty":         {"", ""},
		"a deliberate U+FFFD is content, not a fault":    {"a" + replacementChar + "b", "a" + replacementChar + "b"},
		"non-ASCII is not disturbed by the byte scan":    {"  şey\t\tböyle  \r\n", " şey böyle"},
		"indentation on a line that is otherwise empty":  {"a\n    \nb", "a\n\nb"},
		"a blank run split by a whitespace-only line":    {"a\n\n \n\nb", "a\n\nb"},
		"a line whose whole content is a run of tabs":    {"\t\t\t", ""},
		"a one-line document keeps its interior spacing": {"a  b  c", "a b c"},
		"all six together over one realistic fragment": {
			"\r\n\tfunc  f() {\t\r\n\r\n\r\n\t\treturn  1\t\r\n}\r\n\r\n",
			" func f() {\n\n return 1\n}",
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := Normalise(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
