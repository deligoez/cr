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

// §1.4 step 6 spells out the one-line case, because it is where a rejoin that
// appends a terminator per line rather than between lines looks right on every
// multi-line document and is wrong on every single-line one. The hashes of
// §3.3, §7.4 and §9.2 are taken over short texts — a claim span, an anchored
// line — so a stray terminator would be the common case and not the corner.
//
// A trailing LF on the input is asserted alongside the bare line, since it is
// how a file arrives from disk and must reach the same value: the whole point
// of the transform is that two spellings of one text hash alike.
func TestAOneLineInputNormalisesWithNoTerminator(t *testing.T) {
	for name, in := range map[string]string{
		"as typed, with no terminator": "the guard has no test",
		"as a file leaves it, with LF": "the guard has no test\n",
		"as Windows leaves it":         "the guard has no test\r\n",
		"with the blank lines a paste adds": "\n\n" +
			"the guard has no test" + "\n\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := Normalise(in)
			require.NoError(t, err)
			assert.Equal(t, "the guard has no test", got)
			assert.NotContains(t, got, "\n", "a one-line result carries no line separator at all")
		})
	}
}

// §1.4 step 1 decodes as UTF-8 and makes invalid input fail with exit code 1,
// so the refusal has to happen before anything else in the transform touches
// the bytes. Every case below is built with string([]byte{…}) rather than
// written into a literal, because that conversion copies bytes and replaces
// nothing — it is the one way to hand Normalise a string that is genuinely not
// UTF-8, and it is also the reason the check can exist at all.
//
// The offsets are asserted, not just the failure. A refusal that could not say
// where the text went wrong would leave the user with an issue body or a diff
// and nowhere to look, and the offset is also what separates a real check from
// one that fires on the first byte whatever the input.
func TestInvalidUTF8IsRefusedBeforeAnyStepRuns(t *testing.T) {
	for name, tc := range map[string]struct {
		in     string
		offset int
		bad    byte
	}{
		"a byte that begins no sequence, at the very start": {
			string([]byte{0xFF}) + "abc", 0, 0xFF,
		},
		"the same byte behind a multi-byte rune": {
			"aé" + string([]byte{0xFF}), 3, 0xFF,
		},
		"a continuation byte with nothing in front of it": {
			string([]byte{0x80}) + "abc", 0, 0x80,
		},
		"a two-byte sequence cut short": {
			"a" + string([]byte{0xC3}), 1, 0xC3,
		},
		"an overlong encoding of SPACE": {
			string([]byte{0xC0, 0xA0}), 0, 0xC0,
		},
		"a surrogate half, which UTF-8 does not encode": {
			string([]byte{0xED, 0xA0, 0x80}), 0, 0xED,
		},
		"a bad byte behind text that would have normalised": {
			"  a  \n\n" + string([]byte{0xFE}), 7, 0xFE,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := Normalise(tc.in)
			require.Error(t, err)
			assert.Empty(t, got, "a refused text normalises to nothing at all")

			var invalid *InvalidUTF8Error
			require.ErrorAs(t, err, &invalid)
			assert.Equal(t, tc.offset, invalid.Offset, "the refusal names the byte to look at")
			assert.Equal(t, tc.bad, invalid.Byte, "and what that byte is")
			assert.Contains(t, err.Error(), "UTF-8", "§12.4: the error names the next step")
		})
	}

	valid, err := Normalise("aé" + replacementChar + "b")
	require.NoError(t, err, "a text that decodes is not refused for looking unusual")
	assert.Equal(t, "aé"+replacementChar+"b", valid)
}

// §1.4's closing sentence forbids case-folding, and the prohibition is not
// stylistic. Everything downstream of the transform is a comparison: §6.4
// suppresses a duplicate finding, §7.4 matches a waiver, §9.2 identifies an
// anchor. Fold case and two texts that differ collapse onto one value, so the
// duplicate suppression drops a finding nobody raised twice and the waiver
// silences a line nobody waived.
//
// The assertion is therefore a difference rather than a shape: two texts that
// differ only in case must still differ afterwards, which no amount of
// incidental spacing can hide.
func TestNormalisationDoesNotCaseFold(t *testing.T) {
	upper, err := Normalise("The Guard Has No Test")
	require.NoError(t, err)
	lower, err := Normalise("the guard has no test")
	require.NoError(t, err)

	assert.Equal(t, "The Guard Has No Test", upper, "every letter survives as it was written")
	assert.NotEqual(t, upper, lower, "two texts differing only in case still differ")

	turkish, err := Normalise("İSTANBUL istanbul")
	require.NoError(t, err)
	assert.Equal(t, "İSTANBUL istanbul", turkish,
		"including where a locale-aware fold would not even round-trip")
}

// §1.4 forbids stripping punctuation, and in a code review punctuation is very
// often the whole difference. `x != nil` and `x == nil` are one substitution
// apart; so are a call and a call whose result is discarded. A transform that
// dropped punctuation would let §6.4 suppress a finding about the second on the
// strength of a finding about the first.
//
// The comparison texts are chosen so that only punctuation separates them: the
// letters, the words and the order are identical, and if the two normalise
// alike then punctuation was thrown away.
func TestNormalisationDoesNotStripPunctuation(t *testing.T) {
	kept, err := Normalise("if (x != nil) { return -1; } // guard, always.")
	require.NoError(t, err)
	assert.Equal(t, "if (x != nil) { return -1; } // guard, always.", kept,
		"every punctuation mark survives exactly where it was written")

	negated, err := Normalise("if x != nil")
	require.NoError(t, err)
	affirmed, err := Normalise("if x == nil")
	require.NoError(t, err)
	assert.NotEqual(t, negated, affirmed,
		"two conditions one operator apart do not collapse onto one value")
}

// §1.4 forbids reordering lines, which rules out the obvious way to make two
// texts compare equal regardless of layout — sort them. §3.4.6 hashes a unit's
// changed lines and §9.2 hashes an anchor's, and in both the order is the
// meaning: two statements swapped are a different program, and a review that
// could not tell them apart would carry a waiver from one head onto a line that
// no longer says what it said.
//
// The permutation is the assertion. A transform that sorted, or that grouped
// blank lines by moving rather than dropping them, would make these two equal.
func TestNormalisationDoesNotReorderLines(t *testing.T) {
	asWritten, err := Normalise("lock()\nread()\nunlock()")
	require.NoError(t, err)
	assert.Equal(t, "lock()\nread()\nunlock()", asWritten, "the lines come back in the order they went in")

	swapped, err := Normalise("read()\nlock()\nunlock()")
	require.NoError(t, err)
	assert.NotEqual(t, asWritten, swapped, "a permutation of the same lines is a different text")

	sortedAlready, err := Normalise("a\nb\nc")
	require.NoError(t, err)
	reversed, err := Normalise("c\nb\na")
	require.NoError(t, err)
	assert.Equal(t, "c\nb\na", reversed, "descending order is left descending")
	assert.NotEqual(t, sortedAlready, reversed, "and never sorted into agreement with its reverse")
}
