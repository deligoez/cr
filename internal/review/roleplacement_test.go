package review

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAdequacyPlacement is the test lens's instruction on where a record about
// an untested line goes (field-feedback 2.3), one line of the prompt per
// paragraph. It is written out rather than read from the role file, so a
// prompt that dropped or reworded it cannot pass by reading the same change.
var testAdequacyPlacement = []string{
	"Adequacy is a property of the code under test, so judge it there. When a changed production line has no " +
		"test that would notice it breaking, raise that from the cell of the unit holding the line: anchor the " +
		"record on the production line, which must lie in the diff and inside that unit, and cite the test file " +
		"you looked in. cr keeps such a record a question until a probe supports it, and a probe supports a " +
		"record only when its target lies inside the record's anchor, so a record anchored on a test can never " +
		"be supported by breaking the code.",
	"The cell of a unit holding a test file judges the test itself: whether it asserts anything, whether it " +
		"exercises a mock instead of the code, whether it repeats another test. That cell records pass, a " +
		"question, or a finding about the test, never a finding about the production code the test calls.",
	"A production line outside the diff belongs to no unit, so there is no cell to raise it from and no " +
		"record to write.",
}

// correctnessOccurrences is the correctness lens's instruction to look for a
// predicate's other occurrences and to keep them out of the citations
// (field-feedback 3.1).
const correctnessOccurrences = "When a finding rests on a predicate, a condition or a rule that appears in more " +
	"than one place, search the head for its other occurrences, because a fix made where the record points and " +
	"missed where it does not leaves the same defect standing. Name the other occurrences in the evidence and " +
	"say which you checked; they are not citations, because they show where the rule appears, not that it is " +
	"wrong. A citation is a location a reader can open to see the premise of the defect — the definition the " +
	"code violates, the caller that passes the value it mishandles, the constant or enum it disagrees with — " +
	"never another copy of the same code."

// The emitted prompt carries each instruction as a whole line on the cells of
// the role that owns it — the production unit u1 and the test file's unit u2
// alike, since a cell cannot know in advance which one it will be — and on no
// cell of the other shipped roles.
func TestAPromptCarriesItsOwnRolesPlacementInstructions(t *testing.T) {
	fan, err := Run(briefed(t))
	require.NoError(t, err)
	require.Len(t, fan.Prompts, 8, "four shipped roles over the round's two units")

	owned := map[string][]string{
		"test-adequacy": testAdequacyPlacement,
		"correctness":   {correctnessOccurrences},
	}
	for _, prompt := range fan.Prompts {
		lines := strings.Split(prompt.Text, "\n")
		for owner, paragraphs := range owned {
			for _, paragraph := range paragraphs {
				if owner == prompt.Role {
					assert.Contains(t, lines, paragraph, "%s on %s lost its instruction", prompt.Role, prompt.Unit)
				} else {
					assert.NotContains(t, lines, paragraph, "%s on %s carries %s's instruction",
						prompt.Role, prompt.Unit, owner)
				}
			}
		}
	}
}
