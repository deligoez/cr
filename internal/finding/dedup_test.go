package finding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/role"
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
			ContextHash:   "2a3b4c5d6e7f8091",
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
			ContextHash:   "8f7e6d5c4b3a2910",
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

// The two ladders §6.4.2 ranks a duplicate group by, written out here rather
// than read from the implementation's own gradeStrength and severityStrength: a
// test that ordered by those would agree with whatever order they happened to
// hold, including a reversed one.
var (
	gradesStrongestFirst   = []Grade{GradeProbed, GradeCited, GradeArgued}
	severitiesHighestFirst = []Severity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}
)

// The two role ids the corpus below resolves, chosen so that §2.5.5's order and
// ascending lexicographic order disagree: `zebra-eyes` is resolved from the
// per-repository layer and `alpha-lens` from the global one, so corpus order
// puts `zebra-eyes` first while its id sorts last. Neither is a shipped id, so
// the built-in layer shadows nothing.
const (
	roleAboveInCorpus = "zebra-eyes"
	roleBelowInCorpus = "alpha-lens"
)

// duplicateRecord is one member of a duplicate group: the four fields §6.4.1
// keys on are fixed, and only what §6.4.2 ranks by varies.
func duplicateRecord(id, roleID string, grade Grade, severity Severity) *Finding {
	return &Finding{
		ID:       id,
		Kind:     KindFinding,
		Role:     roleID,
		Class:    "unchecked-error",
		Severity: severity,
		Grade:    grade,
		Unit:     "u1",
		Anchor: Anchor{
			Path:        "internal/store/write.go",
			Side:        git.Right,
			StartLine:   40,
			Line:        44,
			ContentHash: "1f0a2b3c4d5e6f70",
		},
		Summary:  "The returned error is discarded.",
		Evidence: "The call's error result is assigned to the blank identifier.",
	}
}

// writeRoleFile writes a minimal §2.5 role file for id into dir, named <id>.json
// as §2.2 requires.
func writeRoleFile(t *testing.T, dir, id string) {
	t.Helper()
	doc, err := json.Marshal(map[string]any{
		"id":           id,
		"title":        "Title of " + id,
		"axis":         axis.Correctness,
		"instructions": "Framing text for " + id + ".",
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".json"), doc, 0o600))
}

// corpusOrderAcrossTwoLayers resolves a real corpus holding roleAboveInCorpus at
// the per-repository layer and roleBelowInCorpus at the global one, and returns
// §2.5.5's order over it.
//
// It resolves a corpus rather than handing back a comparison written for the
// occasion, because the property under test is that §6.4.2 reads §2.5.5 — and a
// stub comparison would pass whatever order internal/role actually produces.
func corpusOrderAcrossTwoLayers(t *testing.T) func(a, b string) int {
	t.Helper()
	repoRoles, globalRoles := t.TempDir(), t.TempDir()
	writeRoleFile(t, repoRoles, roleAboveInCorpus)
	writeRoleFile(t, globalRoles, roleBelowInCorpus)

	corpus, err := role.Resolve(repoRoles, globalRoles)
	require.NoError(t, err)
	order := role.Order(corpus)

	require.Less(t, roleBelowInCorpus, roleAboveInCorpus,
		"the pair is only worth ranking while ascending id disagrees with §2.5.5")
	require.Negative(t, order(roleAboveInCorpus, roleBelowInCorpus),
		"§2.5.4 puts the per-repository layer first, so %s precedes %s in corpus order",
		roleAboveInCorpus, roleBelowInCorpus)
	return order
}

// representativeOf groups records that share one dedup key and returns the one
// §6.4.2 makes the group's representative.
//
// It goes through Groups rather than building a Group literal, so every case
// below asserts about the partition §6.4.1 actually produces: a pair that
// stopped colliding would arrive here as two groups and fail on the length
// rather than quietly comparing one record against itself.
func representativeOf(order func(a, b string) int, records ...*Finding) *Finding {
	groups := Groups(records)
	if len(groups) != 1 {
		return nil
	}
	return groups[0].Records[groups[0].RepresentativeAt(order)]
}

// §6.4.2's three keys total-order a group only as far as the role, so two
// records from one role at one anchored line in one class, tied on grade and on
// severity, compare equal under all three. Something still has to decide, and
// what decides is arrival: Groups holds a group's records in the order they
// were read, and the first of them represents it.
//
// The alternative is not a different answer but an unstable one. A comparison
// that let an equal record displace the one already held would give the
// representative slot to whichever role file the caller happened to name last,
// and `cr merge` would name a different representative for the same round
// depending on the order a shell expanded its arguments in — while §6.4.3
// retired the other one either way.
//
// Mutation testing is what asked for this case: relaxing the strict comparison
// in outranks left every other assertion in this file green.
func TestArrivalDecidesARepresentativeTheThreeKeysCannotSeparate(t *testing.T) {
	order := corpusOrderAcrossTwoLayers(t)

	first := duplicateRecord("f1", roleBelowInCorpus, GradeCited, SeverityHigh)
	second := duplicateRecord("f2", roleBelowInCorpus, GradeCited, SeverityHigh)
	require.Equal(t, first.Role, second.Role, "the pair has to tie on all three keys")

	assert.Same(t, first, representativeOf(order, first, second),
		"§6.4.2 leaves this tie open, and the record read first is what fills it")
	assert.Same(t, second, representativeOf(order, second, first),
		"and arrival is what decides it, not the record id")
}

// §6.4.2's last key is "the earliest role in corpus order per §2.5.5", and this
// is the group that turns on it: two roles reporting one defect at one anchored
// line, tied on grade and on severity, so nothing above the role can decide it.
//
// The winner is fixed against the two orders that would otherwise look right.
// The loser's id sorts first alphabetically, so a representative picked by role
// id would name it; the loser also arrives first, so one picked by arrival —
// the order the fan-out files happened to be listed in — would name it too.
// Either would hand the posted comment to a role a per-repository corpus had
// deliberately ranked below the other, and §6.4.3 would retire the record the
// project meant to hear from.
func TestTheEarliestRoleInCorpusOrderRepresentsAGroupTiedOnGradeAndSeverity(t *testing.T) {
	order := corpusOrderAcrossTwoLayers(t)

	below := duplicateRecord("f1", roleBelowInCorpus, GradeCited, SeverityHigh)
	above := duplicateRecord("f2", roleAboveInCorpus, GradeCited, SeverityHigh)

	require.Equal(t, below.Grade, above.Grade, "the pair must be tied on grade")
	require.Equal(t, below.Severity, above.Severity, "and on severity")

	groups := Groups([]*Finding{below, above})
	require.Len(t, groups, 1, "§6.4.1: one defect at one place is one group")
	assert.Same(t, above, groups[0].Records[groups[0].RepresentativeAt(order)],
		"§6.4.2: the earliest role in corpus order represents the group")
}

// §6.4.2 orders its three keys, and the order is the whole rule: a group is
// represented by the highest grade, and severity is consulted only among
// records that tied on it.
//
// Each rung is built so that only the key under test can produce the expected
// winner. The stronger-graded record is given the quietest severity and the
// later role, so a comparison that read severity first, or corpus order first,
// picks the other one; the higher-severity record of the second ladder is given
// the later role for the same reason. That is what makes this a test of the
// ordering rather than of three independent comparisons.
func TestARepresentativeIsTheHighestGradeBeforeTheHighestSeverity(t *testing.T) {
	order := corpusOrderAcrossTwoLayers(t)

	for at, stronger := range gradesStrongestFirst {
		for _, weaker := range gradesStrongestFirst[at+1:] {
			t.Run(string(stronger)+" over "+string(weaker), func(t *testing.T) {
				loud := duplicateRecord("f1", roleAboveInCorpus, weaker, SeverityCritical)
				graded := duplicateRecord("f2", roleBelowInCorpus, stronger, SeverityLow)
				assert.Same(t, graded, representativeOf(order, loud, graded),
					"§6.4.2 reads the grade before the severity and before the role")
			})
		}
	}

	for at, higher := range severitiesHighestFirst {
		for _, lower := range severitiesHighestFirst[at+1:] {
			t.Run(string(higher)+" over "+string(lower), func(t *testing.T) {
				quiet := duplicateRecord("f1", roleAboveInCorpus, GradeCited, lower)
				loud := duplicateRecord("f2", roleBelowInCorpus, GradeCited, higher)
				assert.Same(t, loud, representativeOf(order, quiet, loud),
					"§6.4.2 reads the severity before the role")
			})
		}
	}
}

// §6.1.3 checks that `severity` and `grade` are present, not that their values
// are ones cr knows, so an unrecognised value reaches §6.4.2 — and a rank read
// off a bare map would make it zero, the strongest rank there is. A record
// nobody can rank would then represent every group it fell into and speak for
// the records that were graded.
//
// It is not a hypothetical about `grade`: §6.5.1 has `cr merge` compute the
// grade in memory, so a record whose grade cr never computed carries the empty
// string here.
func TestAnUnrankableSeverityOrGradeNeverRepresentsAGroup(t *testing.T) {
	order := corpusOrderAcrossTwoLayers(t)

	unrankable := duplicateRecord("f1", roleAboveInCorpus, "blocker", SeverityCritical)
	ranked := duplicateRecord("f2", roleBelowInCorpus, GradeArgued, SeverityLow)
	assert.Same(t, ranked, representativeOf(order, unrankable, ranked),
		"a grade cr does not recognise ranks behind every grade it does")

	unrankable = duplicateRecord("f1", roleAboveInCorpus, GradeCited, "showstopper")
	ranked = duplicateRecord("f2", roleBelowInCorpus, GradeCited, SeverityLow)
	assert.Same(t, ranked, representativeOf(order, unrankable, ranked),
		"a severity cr does not recognise ranks behind every severity it does")
}
