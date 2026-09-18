package draft

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A marker whose value never closes its quote is malformed, and saying so is
// all that happens: the scan runs off the end of the line and stops there.
//
// This is the boundary the rest of the grammar's cases never reach. Every other
// deviation is caught before the scan begins or while it is still inside the
// line — an unquoted value fails at the opening quote, a missing field fails at
// the field name — so the one path that walks to the last byte without finding
// a closing quote is only taken by a value that has none. Mutation testing
// found it: widening the scan's bound by one survived every other case in the
// suite, and the widened bound reads one byte past the line.
//
// The trailing backslash is the sharper half. An escape consumes the byte after
// it, so a line ending in `\` moves the scan past the end in one step rather
// than by reaching it — a bound that admits the extra byte is not merely wrong
// about where the value ends, it reads memory the line does not have.
func TestAValueThatNeverClosesItsQuoteIsMalformed(t *testing.T) {
	for name, line := range map[string]string{
		"an unterminated value":      `<!-- cr:record id="f1 -->`,
		"a value ending in escape":   `<!-- cr:record id="f1\`,
		"nothing after the quote":    `<!-- cr:record id="`,
		"an escape and nothing more": `<!-- cr:record id="\`,
	} {
		t.Run(name, func(t *testing.T) {
			require.True(t, IsMarkerLine(line), "the line claims to be a marker")

			marker, err := ParseMarker(11, line)
			require.Error(t, err, "rule 5: a value that never closes is a deviation")
			assert.Equal(t, Marker{}, marker)

			var malformed *MalformedMarkerError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, 11, malformed.At, "§7.2.1: the refusal names the line")
			assert.Contains(t, malformed.Problem, "id",
				"and the field the scan gave up in")
		})
	}
}

// cutQuoted stops at the end of what it was given and never reads past it.
//
// It is exercised directly as well as through ParseMarker, because the bound is
// a property of the scan rather than of the grammar around it: every prefix of
// a well-formed value is a string with no closing quote, and walking one of
// them must end in a report rather than in an index nothing holds.
func TestTheScanStopsAtTheEndOfWhatItWasGiven(t *testing.T) {
	// Every inner quote is escaped, so no proper prefix of this literal
	// closes it: each one is a value that never ends.
	quoted := `"internal/api/a \"handler\".go"`
	value, rest, ok := cutQuoted(quoted + " and the rest")
	require.True(t, ok)
	assert.Equal(t, `internal/api/a "handler".go`, value)
	assert.Equal(t, " and the rest", rest)

	for cut := 1; cut < len(quoted); cut++ {
		_, _, ok := cutQuoted(quoted[:cut])
		assert.False(t, ok, "the scan reports rather than reads past %q", quoted[:cut])
	}
}
