package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fiveLines is a tree holding one five-line file on both sides.
func fiveLines() Trees {
	read := func(path string) ([]string, bool, error) {
		if path != "five.go" {
			return nil, false, nil
		}
		return []string{"one", "two", "three", "four", "five"}, true, nil
	}
	return Trees{Head: read, MergeBase: read}
}

// §9.2's window is up to three lines on each side, so it is cut short at either
// end of the file rather than reaching past it, and an agent's own hash and
// window are replaced by the ones the tree gives.
func TestStampAnchorRecordsTheHashAndAWindowCutAtTheFileEnds(t *testing.T) {
	for _, c := range []struct {
		name          string
		start, line   int
		before, after []string
	}{
		{"at the top", 1, 1, []string{}, []string{"two", "three", "four"}},
		{"in the middle", 3, 3, []string{"one", "two"}, []string{"four", "five"}},
		{"at the bottom", 4, 5, []string{"one", "two", "three"}, []string{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			anchor := Anchor{
				Path: "five.go", Side: "RIGHT", StartLine: c.start, Line: c.line,
				ContentHash: "0123456789abcdef", ContextBefore: []string{"typed by the agent"},
			}
			require.NoError(t, StampAnchor(fiveLines(), "merged.ndjson", 1, &anchor))

			want, err := AnchorContentHash([]string{"one", "two", "three", "four", "five"}[c.start-1 : c.line])
			require.NoError(t, err)
			assert.Equal(t, want, anchor.ContentHash)
			assert.Equal(t, c.before, anchor.ContextBefore)
			assert.Equal(t, c.after, anchor.ContextAfter)
		})
	}
}

// An anchor the tree does not hold is refused before anything is stamped.
func TestStampAnchorRefusesAnAnchorThatDoesNotResolve(t *testing.T) {
	anchor := Anchor{Path: "five.go", Side: "RIGHT", StartLine: 5, Line: 6, ContentHash: "kept"}

	err := StampAnchor(fiveLines(), "merged.ndjson", 2, &anchor)

	var rejected *RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, "anchor", rejected.Field)
	assert.Equal(t, "kept", anchor.ContentHash)
}
