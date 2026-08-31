package finding

import (
	"reflect"
	"testing"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dedupKeyFields are the four fields §6.4.1's key is made of, in the dotted form
// fieldsInCommon reports: the section's own three, plus the `anchor.side` round
// 8's dedup-key-omits-side and round 12's dedup-key-drops-side add.
var dedupKeyFields = []string{"Class", "Anchor.Path", "Anchor.Side", "Anchor.Line"}

// §6.4.1 keys a duplicate group on the anchored line and the class, and on
// nothing that names the producer: two roles reporting one defect at one place
// is the case the section exists for, and a key reading the role, the severity,
// the grade, the summary or the id would put them in two groups and post the
// comment twice.
//
// A comment saying so proves nothing, so what is asserted is the collision: two
// records agreeing on nothing but the key's four fields key identically and fall
// into one group. The pair is held honest from the other side by reading §6.1's
// table off the record type — every field outside the key must differ between
// them, so the equality cannot have come from a field that happens to match, and
// a field added to the table later without being varied here fails this test
// rather than quietly joining the key's blind spot.
func TestADedupKeyIsTheAnchoredLineAndTheClassAndNothingElse(t *testing.T) {
	byCorrectness, byConvention := theSameDefectAtTheSameLine()

	require.ElementsMatch(t, dedupKeyFields,
		fieldsInCommon(reflect.ValueOf(byCorrectness), reflect.ValueOf(byConvention), ""),
		"the two records must agree on the key's fields and differ in every other field of §6.1's table")

	assert.Equal(t, DedupKeyOf(&byCorrectness), DedupKeyOf(&byConvention),
		"§6.4.1: one defect at one place is one group whichever role reported it")

	groups := Groups([]*Finding{&byCorrectness, &byConvention})
	require.Len(t, groups, 1, "§6.4.1 groups them together")
	assert.Len(t, groups[0].Records, 2)
}

// theSameDefectAtTheSameLine is one defect at one anchored line seen by two
// roles: the same class over the same last line of the same file on the same
// side, and every other field of §6.1's table different, down to the head and
// the round.
//
// The two anchors deliberately start at different lines. §6.4.1 keys on
// `anchor.line`, so a role that anchored the whole added block and a role that
// anchored its last line are reporting one defect and must group.
func theSameDefectAtTheSameLine() (byCorrectness, byConvention Finding) {
	byCorrectness = Finding{
		ID:       "f1",
		Kind:     KindFinding,
		Axis:     "correctness",
		Role:     "correctness",
		Class:    "unchecked-error",
		Rule:     "handle-every-error",
		Severity: SeverityHigh,
		Grade:    GradeCited,
		Unit:     "u1",
		Claim:    "CR-1#c1",
		Anchor: Anchor{
			Path:          "internal/store/write.go",
			Side:          git.Right,
			StartLine:     40,
			Line:          44,
			ContentHash:   "1f0a2b3c4d5e6f70",
			ContextBefore: []string{"func (s *Store) Save() error {"},
			ContextAfter:  []string{"	return nil"},
		},
		Summary:          "The returned error is discarded.",
		Evidence:         "The call's error result is assigned to the blank identifier.",
		Citations:        []Citation{{Path: "internal/store/write.go", Line: 44}},
		Probe:            "p1",
		Suggestion:       "	if err != nil {",
		SuggestionOrigin: OriginRule,
		State:            StateDraft,
		Disposition:      DispositionWrong,
		DuplicateOf:      "f8",
		SuppressedBy:     "f9",
		ThreadID:         "t1",
		Stamp:            state.Stamp{Head: "0a1b2c3", Round: 1},
	}

	byConvention = Finding{
		ID:       "f2",
		Kind:     KindQuestion,
		Axis:     "convention",
		Role:     "convention",
		Class:    "unchecked-error",
		Rule:     "no-blank-error",
		Severity: SeverityLow,
		Grade:    GradeArgued,
		Unit:     "u7",
		Claim:    "CR-1#c2",
		Anchor: Anchor{
			Path:          "internal/store/write.go",
			Side:          git.Right,
			StartLine:     44,
			Line:          44,
			ContentHash:   "9e8d7c6b5a493827",
			ContextBefore: []string{"	_ = s.flush()", "	// nothing above this line"},
			ContextAfter:  []string{"}", ""},
		},
		Summary:          "An error result is assigned to the blank identifier here.",
		Evidence:         "The repository rule forbids discarding an error result.",
		Citations:        []Citation{{Path: "internal/store/read.go", Line: 12}},
		Probe:            "p4",
		Suggestion:       "	if err := s.flush(); err != nil {",
		SuggestionOrigin: OriginAgent,
		State:            StateQueued,
		Disposition:      DispositionNotHere,
		DuplicateOf:      "f5",
		SuppressedBy:     "f6",
		ThreadID:         "t2",
		Stamp:            state.Stamp{Head: "4d5e6f7", Round: 4},
	}

	return byCorrectness, byConvention
}
