package role

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The paragraphs below are pinned whole, one per line of a role's instructions,
// because each one tells a reviewing agent where a record goes, and a reworded
// paragraph is a changed instruction to every repository cr reviews. Changing
// one here is the deliberate half of changing it in builtin.
//
// testAdequacyPlacement is field-feedback 2.3: seven of nineteen probes in a
// field trial supported nothing, because the test lens anchored on the test
// while the mutation broke the production line, and a probe supports only a
// record whose anchor holds its target.
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

// correctnessOccurrences is field-feedback 3.1: a fix needed the same status
// predicate changed in a third place that no record named.
var correctnessOccurrences = []string{
	"When a finding rests on a predicate, a condition or a rule that appears in more than one place, search " +
		"the head for its other occurrences, cite each one you find, and say in the evidence which occurrences " +
		"you checked. A fix made where the record points and missed where it does not leaves the same defect " +
		"standing.",
}

// Each paragraph is a whole line of its own role's instructions and of no
// other shipped role's, so a paragraph that moved to the wrong lens fails as
// surely as one that was reworded or dropped.
func TestTheShippedRolesSayWhereARecordGoes(t *testing.T) {
	pinned := map[string][]string{
		testAdequacyID: testAdequacyPlacement,
		correctnessID:  correctnessOccurrences,
	}
	for id, content := range Builtins() {
		t.Run(id, func(t *testing.T) {
			r, err := Parse(id+fileExt, []byte(content))
			require.NoError(t, err)
			lines := strings.Split(r.Instructions, "\n")

			for owner, paragraphs := range pinned {
				for _, paragraph := range paragraphs {
					if owner == id {
						assert.Contains(t, lines, paragraph, "%s lost or reworded a pinned paragraph", id)
					} else {
						assert.NotContains(t, lines, paragraph, "%s carries a paragraph of %s", id, owner)
					}
				}
			}
		})
	}
}
