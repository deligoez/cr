package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// aRecord is one queued record, complete enough for §7.1.1's eight marker
// fields to have something to say.
func aRecord(id string) *finding.Finding {
	return &finding.Finding{
		ID:       id,
		Kind:     finding.KindFinding,
		Role:     "correctness",
		Class:    "unchecked-error",
		Severity: finding.SeverityHigh,
		Grade:    finding.GradeCited,
		Unit:     "u1",
		Anchor: finding.Anchor{
			Path: "internal/api/handler.go", Side: "RIGHT",
			StartLine: 42, Line: 44, ContentHash: "0123456789abcdef",
		},
		Summary:  "The error Decode returns is dropped.",
		Evidence: "The call's second result is assigned to the blank identifier.",
		State:    finding.StateQueued,
	}
}

// renderOf is Render over records, for a test whose records §8.1.3 accepts.
func renderOf(t *testing.T, records ...*finding.Finding) string {
	t.Helper()
	rendered, err := Render(records)
	require.NoError(t, err)
	return rendered
}

// §7.1.1: one record is one block, introduced by a marker carrying the eight
// fields the section names.
func TestABlockIsAMarkerAndABody(t *testing.T) {
	rendered := renderOf(t, aRecord("f1"))

	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")
	require.Len(t, lines, 5, "a marker, a blank line, and the body's two paragraphs")
	assert.Equal(t,
		`<!-- cr:record id="f1" kind="finding" path="internal/api/handler.go" `+
			`start_line="42" line="44" severity="high" grade="cited" disposition="" -->`,
		lines[0])
	assert.Empty(t, lines[1])
	assert.Equal(t, "The error Decode returns is dropped.", lines[2],
		"§7.1.2: the body is the region the user may rewrite entirely")
	assert.Empty(t, lines[3])
	assert.Equal(t, "The call's second result is assigned to the blank identifier.", lines[4],
		"§8.1.2: the initial body is drawn from the summary and the evidence")
}

// §7.1.1 names eight fields, and the marker carries all eight — `disposition`
// included, which is empty at draft time and is what §7.2's table has the
// reviewer type `wrong` into. A field that appeared only once it had a value
// would leave the verb no place to be written.
func TestTheMarkerCarriesEveryFieldSection711Names(t *testing.T) {
	rendered := renderOf(t, aRecord("f1"))

	for _, field := range []string{
		"id", "kind", "path", "start_line", "line", "severity", "grade", "disposition",
	} {
		assert.Contains(t, rendered, " "+field+"=\"", "§7.1.1 names %s", field)
	}
}

// §7.1 renders every queued record and keeps them apart. Two records are two
// blocks, in the order they arrive, with a blank line between them so a
// reviewer editing one body cannot run it into the next block's marker.
func TestEveryRecordIsItsOwnBlockInTheOrderItArrived(t *testing.T) {
	first, second := aRecord("f1"), aRecord("f2")
	second.Summary = "The retry never backs off."

	rendered := renderOf(t, first, second)

	assert.Equal(t, 2, strings.Count(rendered, "<!-- cr:record "))
	assert.Less(t, strings.Index(rendered, `id="f1"`), strings.Index(rendered, `id="f2"`),
		"the caller's order is the draft's order")
	assert.Contains(t, rendered, first.Evidence+"\n\n<!-- cr:record "+`id="f2"`,
		"a blank line separates one block's body from the next block's marker")
}

// A round with nothing queued renders nothing, rather than a document
// describing an empty round. The header §7.1.4 asks for is its own obligation;
// what this says is that the block renderer invents no block.
func TestNothingQueuedRendersNoBlock(t *testing.T) {
	assert.Empty(t, renderOf(t))
	assert.Empty(t, renderOf(t, []*finding.Finding{}...))
}

// A marker value cr does not choose cannot break the marker.
//
// The path is the one field that carries whatever the repository under review
// happens to hold, and §7.2.1 has `cr post` refuse a malformed marker — so a
// path with a quote, a space, or a newline in it must not be able to produce
// one. The `-->` case is the sharp one: escaped, it cannot close the comment
// early and leave the rest of the path to be read as prose.
func TestAPathCannotBreakOutOfTheMarker(t *testing.T) {
	for name, path := range map[string]string{
		"a space":     "internal/api/a handler.go",
		"a quote":     `internal/api/"handler".go`,
		"a newline":   "internal/api/handler.go\nnot a marker",
		"a backslash": `internal\api\handler.go`,
		"a terminator": "internal/api/handler.go --> " +
			`<!-- cr:record id="f9" -->`,
	} {
		t.Run(name, func(t *testing.T) {
			record := aRecord("f1")
			record.Anchor.Path = path
			rendered := renderOf(t, record)

			markers := 0
			for _, line := range strings.Split(rendered, "\n") {
				if strings.HasPrefix(line, "<!-- cr:record ") {
					markers++
					assert.True(t, strings.HasSuffix(line, " -->"),
						"the marker ends where it says it ends")
				}
			}
			assert.Equal(t, 1, markers, "one record is one marker, whatever the path holds")
		})
	}
}
