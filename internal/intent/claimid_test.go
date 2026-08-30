package intent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// §3.3 forms every claim id as `<ISSUE-KEY>#c<n>`, which makes an id two things
// at once: a per-issue counter, and a foreign key into the key §3.2 resolved.
// Reading the second half back out is what lets DecodeClaims hold a claim to
// the issue this run is about, so the spelling has to be exact in both
// directions — an id cr cannot split names no issue at all, and one that split
// too eagerly would name the wrong one.
func TestAClaimIDNamesTheIssueItBelongsTo(t *testing.T) {
	for id, key := range map[string]string{
		"CR-1#c1":     "CR-1",
		"CR-1#c7":     "CR-1",
		"CR-1#c42":    "CR-1",
		"A#c1":        "A",
		"CR-1#c2#c3":  "CR-1#c2",
		"PROJ-99#c10": "PROJ-99",
	} {
		t.Run(id, func(t *testing.T) {
			split, ok := SplitClaimID(id)
			assert.True(t, ok)
			assert.Equal(t, key, split,
				"the split is at the last infix: §2.2 lets an issue key hold #c, "+
					"while the number after the last one is fixed")
		})
	}

	for name, id := range map[string]string{
		"no infix at all":         "CR-1",
		"nothing but an infix":    "#c",
		"an empty issue key":      "#c1",
		"an empty number":         "CR-1#c",
		"a note id":               "CR-1#n1",
		"numbered from zero":      "CR-1#c0",
		"a leading zero":          "CR-1#c07",
		"a signed number":         "CR-1#c+7",
		"a negative number":       "CR-1#c-7",
		"a number with a suffix":  "CR-1#c1x",
		"nothing":                 "",
		"a number that is a word": "CR-1#cone",
	} {
		t.Run(name, func(t *testing.T) {
			split, ok := SplitClaimID(id)
			assert.False(t, ok, "%q is not the id §3.3 forms", id)
			assert.Empty(t, split, "an id cr cannot read names no issue")
		})
	}
}
