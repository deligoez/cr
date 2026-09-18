package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// §6.2.2's binding over the edges of an anchor range: a target holds only on
// the same path, inside `start_line`..`line` inclusive, and only for a RIGHT
// anchor. One line past either end is outside, which is the case the binding
// exists for — one experiment cannot grade a second finding next to the first.
func TestASpanHoldsOnlyATargetInsideItsRightRangeOnTheSamePath(t *testing.T) {
	anchored := Span{Path: "internal/api/handler.go", Side: git.Right, StartLine: 42, Line: 44}
	for name, tc := range map[string]struct {
		span   Span
		target string
		holds  bool
	}{
		"the first line of the range":   {anchored, "internal/api/handler.go:42", true},
		"the last line of the range":    {anchored, "internal/api/handler.go:44", true},
		"one line above the range":      {anchored, "internal/api/handler.go:41", false},
		"one line below the range":      {anchored, "internal/api/handler.go:45", false},
		"the same line of another file": {anchored, "internal/api/store.go:43", false},
		"a LEFT anchor on the same line": {
			Span{Path: "internal/api/handler.go", Side: git.Left, StartLine: 42, Line: 44},
			"internal/api/handler.go:43", false,
		},
		"a target that is not path:line": {anchored, "internal/api/handler.go", false},
		"no anchor at all":               {Span{}, "internal/api/handler.go:43", false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.holds, tc.span.Holds(tc.target))
		})
	}
}

// A probe that would establish everything else establishes nothing for a
// record whose anchor does not hold its target.
func TestEstablishesRefusesAProbeWhoseTargetTheRecordsAnchorDoesNotHold(t *testing.T) {
	// A mutation probe, whose ladder reads no mapping, so the mapping is
	// left empty and the span is the only thing varied.
	proving := Record{
		ID: "p1", Kind: Mutation, Result: resultNoTestFailed, Baseline: "r1", Target: "app.go:3",
		Stamp: state.Stamp{Head: gradingHead, Round: 2},
	}
	passing := passingBaseline(t, &proving, true)

	assert.True(t, Establishes(&proving, gradingHead, passing, ClaimMapping{}, onTarget))
	assert.False(t, Establishes(&proving, gradingHead, passing, ClaimMapping{},
		Span{Path: "app.go", Side: git.Right, StartLine: 4, Line: 6}),
		"§6.2.2: app.go:3 is one line above the anchor range")
	assert.False(t, Establishes(&proving, gradingHead, passing, ClaimMapping{},
		Span{Path: "app.go", Side: git.Left, StartLine: 3, Line: 3}),
		"§6.2.2: a LEFT anchor never reaches probed")
}
