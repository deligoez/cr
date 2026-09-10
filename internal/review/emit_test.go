package review

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/reinvention"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/testadequacy"
	"github.com/deligoez/cr/internal/unit"
)

// handRound is a round of two units and two active roles, one of them on the
// intent axis, whose mapping maps u1 and leaves u2 unmapped.
func handRound() *Round {
	units := []Unit{
		{Unit: unit.Unit{ID: "u1", Path: "a.go", Side: git.Right, HunkRanges: []unit.Range{{Start: 1, End: 1}}},
			Texts: []string{"@@ -1 +1 @@\n-old\n+new"}},
		{Unit: unit.Unit{ID: "u2", Path: "b.go", Side: git.Right, HunkRanges: []unit.Range{{Start: 4, End: 4}}},
			Texts: []string{"@@ -4 +4 @@\n-before\n+after"}},
	}
	empty := testadequacy.Attachment{Paths: []string{}, Symbols: []string{}, Unavailable: []testadequacy.Unavailable{}}
	return &Round{
		Round: 1, Head: "abc123",
		Roles: []role.Role{
			{ID: "intent-coverage", Title: "Intent coverage", Axis: axis.Intent, Instructions: "Map it."},
			{ID: "correctness", Title: "Correctness", Axis: axis.Correctness, Instructions: "Check it."},
		},
		Units:      units,
		Pairs:      nil,
		Mapped:     true,
		Candidates: reinvention.Attachments{Attached: []reinvention.Attachment{}, Unavailable: []reinvention.Unavailable{}},
		Hits:       []rule.Attachment{{Unit: "u1", Hits: []rule.Hit{}}, {Unit: "u2", Hits: []rule.Hit{}}},
		Tests:      []testadequacy.Attachment{empty, empty},
		Unmapped:   []UnmappedUnit{{Unit: "u2", Kind: finding.KindQuestion}},
	}
}

// For every active role and every unit, one prompt: role by role in corpus
// order, and unit by unit in id order within each (§4.6.1).
func TestOnePromptIsEmittedPerActiveRoleAndUnit(t *testing.T) {
	prompts := Emit(handRound())

	at := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		at = append(at, prompt.Role+"/"+prompt.Unit)
		assert.Contains(t, prompt.Text, "on unit "+prompt.Unit)
	}
	assert.Equal(t, []string{
		"intent-coverage/u1", "intent-coverage/u2", "correctness/u1", "correctness/u2",
	}, at)
}

// The unmapped-unit item reaches the intent role's prompt for the unmapped unit
// and no other prompt, and it says the register §4.1.4 fixes.
//
// The intent role raises the item, so it is the one told to; a correctness
// role handed the same instruction would raise a second question about the
// same unit, and §1.6 prices the second one as much as the first.
func TestTheUnmappedItemReachesOnlyTheIntentPromptOfItsUnit(t *testing.T) {
	prompts := Emit(handRound())
	require.Len(t, prompts, 4)

	for _, prompt := range prompts {
		carries := prompt.Role == "intent-coverage" && prompt.Unit == "u2"
		assert.Equalf(t, carries, strings.Contains(prompt.Text, "Unmapped unit (§4.1.2)"),
			"%s on %s", prompt.Role, prompt.Unit)
		if carries {
			assert.Contains(t, prompt.Text, "raised as kind: question and never as a finding")
		}
	}
}

// Before a mapping is recorded for the round, no prompt says which claims a unit
// is mapped to, and none says a unit is mapped to none: §4.6.5 makes
// unmapped-ness unknowable on the first pass.
func TestBeforeAMappingNoPromptCallsAUnitUnmapped(t *testing.T) {
	r := handRound()
	r.Mapped, r.Unmapped = false, []UnmappedUnit{}

	for _, prompt := range Emit(r) {
		assert.Contains(t, prompt.Text, "No mapping is recorded for round 1 yet")
		assert.NotContains(t, prompt.Text, "The mapping maps no claim to this unit.")
		assert.NotContains(t, prompt.Text, "Unmapped unit (§4.1.2)")
	}
}

