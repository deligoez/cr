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

// §9.2 makes `side` part of an anchor and §6.1.2 resolves a RIGHT anchor against
// the head and a LEFT one against the merge base, so one line number names a
// line in each of two file versions. Round 8's dedup-key-omits-side and
// side-omitted-from-identity-keys, re-reported as round 12's
// dedup-key-drops-side, are the same defect stated twice: keyed on the three
// fields §6.4.1 lists in as many words, a record about a removed line at
// merge-base line 40 and a record about an added line at head line 40 of the
// same file, in the same class, collapse into one group — and §6.4.3 then
// retires one of them in state `duplicate`, naming a representative that is
// about different code, with no reason a reader could observe.
//
// This is the colliding pair, built to differ in nothing else the key reads.
func TestTwoSidesOfOneLineNumberAreTwoDedupGroups(t *testing.T) {
	removed, added := theSameClassAtOneLineNumberOnBothSides()

	require.Equal(t, removed.Anchor.Path, added.Anchor.Path)
	require.Equal(t, removed.Anchor.Line, added.Anchor.Line)
	require.Equal(t, removed.Class, added.Class)
	require.NotEqual(t, removed.Anchor.Side, added.Anchor.Side,
		"the pair must differ in the side and in nothing else the key reads")

	assert.NotEqual(t, DedupKeyOf(&removed), DedupKeyOf(&added),
		"§9.2: LEFT and RIGHT are counted in different trees, so one number is not one line of code")

	groups := Groups([]*Finding{&removed, &added})
	assert.Len(t, groups, 2,
		"§6.4.3 would otherwise retire a record about a deleted line as a duplicate of one about an added line")
}

// theSameClassAtOneLineNumberOnBothSides is the pair §6.4.1's key must keep
// apart: one record about a line the pull request removed, counted in the merge
// base, and one about a line it added, counted in the head — one file, one
// class, one line number, two trees.
func theSameClassAtOneLineNumberOnBothSides() (removed, added Finding) {
	removed = Finding{
		ID:       "f1",
		Kind:     KindFinding,
		Axis:     "correctness",
		Role:     "correctness",
		Class:    "dropped-validation",
		Severity: SeverityHigh,
		Unit:     "u1",
		Anchor: Anchor{
			Path:        "internal/store/write.go",
			Side:        git.Left,
			StartLine:   40,
			Line:        40,
			ContentHash: "1f0a2b3c4d5e6f70",
		},
		Summary:  "The removed line was the only bounds check.",
		Evidence: "The merge base checks the index before the write.",
	}

	added = Finding{
		ID:       "f2",
		Kind:     KindFinding,
		Axis:     "correctness",
		Role:     "correctness",
		Class:    "dropped-validation",
		Severity: SeverityHigh,
		Unit:     "u2",
		Anchor: Anchor{
			Path:        "internal/store/write.go",
			Side:        git.Right,
			StartLine:   40,
			Line:        40,
			ContentHash: "9e8d7c6b5a493827",
		},
		Summary:  "The added line writes without a bounds check.",
		Evidence: "The head writes at the index with nothing above it.",
	}

	return removed, added
}
