package unit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
)

// twinPatch changes three files: a.go and c.go receive the same edit in the same
// context, at different lines and under different section headings, and b.go
// receives the same added line in a different context.
const twinPatch = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -3,2 +3,2 @@ func A() {
 	fire("placed")
-	send()
+	send(locale())
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -3,2 +3,2 @@ func B() {
 	fire("refunded")
-	send()
+	send(locale())
diff --git a/c.go b/c.go
--- a/c.go
+++ b/c.go
@@ -10,2 +10,2 @@ func C() {
 	fire("placed")
-	send()
+	send(locale())
`

// §4.6.8: a unit whose hunks equal an earlier unit's line for line, context
// included and headers aside, is that unit's twin; a unit whose context differs
// is not, however alike the changed lines are.
func TestMarkTwinsPairsUnitsWhoseHunksReadTheSame(t *testing.T) {
	hunks, err := git.ParseHunks(twinPatch)
	require.NoError(t, err)
	texts, err := git.HunkTexts(twinPatch)
	require.NoError(t, err)
	units := make([]Unit, 0, len(hunks))
	for i := range hunks {
		start, end := hunks[i].SideRange()
		units = append(units, Unit{
			ID: "u" + string(rune('1'+i)), Path: hunks[i].Path, Side: hunks[i].Side,
			HunkRanges: []Range{{Start: start, End: end}},
		})
	}

	require.NoError(t, MarkTwins(units, hunks, texts))

	assert.Empty(t, units[0].TwinOf)
	assert.Empty(t, units[1].TwinOf, "a different context line is a different change")
	assert.Equal(t, "u1", units[2].TwinOf, "the same body at another line of another file is a twin")
}
