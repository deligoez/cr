package finding

import (
	"reflect"
	"testing"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discountGuard is the anchored code every record below is about: three lines
// of one branch, so §9.2's multi-line pre-image is the one under test rather
// than the single-line special case.
var discountGuard = []string{"if ($discount) {", "    $total -= $discount;", "}"}

// keyFields are the four fields §7.4.1's key is made of, in the dotted form
// fieldsInCommon reports: §7.4.1's own three, plus the `anchor.side` round 8's
// finding side-omitted-from-identity-keys adds.
var keyFields = []string{"Class", "Anchor.Path", "Anchor.Side", "Anchor.ContentHash"}

// §7.4.1 excludes the summary from the key in as many words, and gives the
// reason: it is agent-composed prose that differs between rounds, so a key
// carrying it would let a waived finding resurface under a rewording. The same
// holds for the role, the grade, the severity and the id — §7.4.1 keys by class
// "so a waived finding cannot return under a different producer", and those are
// what name the producer.
//
// A comment saying so proves nothing, so what is asserted here is the collision:
// two records that agree on nothing but the anchored code and the class key
// identically. The pair is held honest from the other side by reading §6.1's
// table off the record type — every field outside the key must differ between
// them, or the equality below could have come from a field that happens to
// match, and a field added to the table later without being varied here fails
// this test rather than quietly joining the key's blind spot.
func TestAWaiverKeyIsTheAnchoredCodeAndTheClassAndNothingElse(t *testing.T) {
	waived, reworded := theSameDefectAtTheSameCode(t)

	require.ElementsMatch(t, keyFields,
		fieldsInCommon(reflect.ValueOf(waived), reflect.ValueOf(reworded), ""),
		"the two records must agree on the key's fields and differ in every other field of §6.1's table")

	assert.Equal(t, WaiverKeyOf(&waived), WaiverKeyOf(&reworded),
		"§7.4.1: a waived finding must not return under a reworded summary, a different producer, or a later round")
}

// theSameDefectAtTheSameCode is a record a reviewer waived and the record a
// later round produces about the same defect: one class over one range of one
// file, written by another role, in another register, at another severity, with
// another summary, against another head.
//
// Every field of §6.1's table outside the key differs, including the two line
// numbers — the code has moved down the file without changing, which §7.4.2 says
// is still the same waived code.
func theSameDefectAtTheSameCode(t *testing.T) (waived, reworded Finding) {
	t.Helper()
	content := hashOf(t, discountGuard)

	waived = Finding{
		ID:       "f1",
		Kind:     KindFinding,
		Axis:     "test",
		Role:     "test-adequacy",
		Class:    "missing-test",
		Rule:     "no-untested-branch",
		Severity: SeverityHigh,
		Grade:    GradeCited,
		Unit:     "u1",
		Claim:    "CR-1#c1",
		Anchor: Anchor{
			Path:          "app/Models/Order.php",
			Side:          git.Right,
			StartLine:     12,
			Line:          14,
			ContentHash:   content,
			ContextBefore: []string{"public function total(): int", "{"},
			ContextAfter:  []string{"    return $total;"},
		},
		Summary:          "The added discount branch has no test.",
		Evidence:         "No changed test file references the method.",
		Citations:        []Citation{{Path: "tests/Feature/OrderTest.php", Line: 30}},
		Probe:            "p1",
		Suggestion:       "$this->assertSame(90, $order->total());",
		SuggestionOrigin: OriginRule,
		State:            StateDiscarded,
		Disposition:      DispositionWrong,
		DuplicateOf:      "f2",
		SuppressedBy:     "f3",
		ThreadID:         "t1",
		Stamp:            state.Stamp{Head: "0a1b2c3", Round: 1},
	}

	reworded = Finding{
		ID:       "f9",
		Kind:     KindQuestion,
		Axis:     "correctness",
		Role:     "domain-correctness",
		Class:    "missing-test",
		Rule:     "untested-branch",
		Severity: SeverityLow,
		Grade:    GradeArgued,
		Unit:     "u7",
		Claim:    "CR-1#c2",
		Anchor: Anchor{
			Path:          "app/Models/Order.php",
			Side:          git.Right,
			StartLine:     61,
			Line:          63,
			ContentHash:   content,
			ContextBefore: []string{"private function subtotal(): int"},
			ContextAfter:  []string{"}", ""},
		},
		Summary:          "Nothing exercises the branch that applies the discount.",
		Evidence:         "The suite passes with the branch removed.",
		Citations:        []Citation{{Path: "tests/Unit/TotalTest.php", Line: 8}},
		Probe:            "p4",
		Suggestion:       "$this->assertSame(100, $order->total());",
		SuggestionOrigin: OriginAgent,
		State:            StateDraft,
		Disposition:      DispositionNotHere,
		DuplicateOf:      "f5",
		SuppressedBy:     "f6",
		ThreadID:         "t2",
		Stamp:            state.Stamp{Head: "4d5e6f7", Round: 4},
	}

	return waived, reworded
}

// fieldsInCommon returns the dotted name of every field two records hold equal
// values in, so a claim about which fields a key reads is checked against the
// whole of §6.1's table rather than against the fields the author remembered.
//
// A struct field is walked into when every one of its own fields is exported:
// the anchor, whose three key fields sit beside four that belong to no key, and
// the embedded stamp. A type that keeps a field to itself names one thing rather
// than several, so a record's state is compared whole.
func fieldsInCommon(left, right reflect.Value, prefix string) []string {
	common := make([]string, 0, left.NumField())
	for i := range left.NumField() {
		name := prefix + left.Type().Field(i).Name
		a, b := left.Field(i), right.Field(i)
		if a.Kind() == reflect.Struct && everyFieldExported(a.Type()) {
			common = append(common, fieldsInCommon(a, b, name+".")...)
			continue
		}
		if reflect.DeepEqual(a.Interface(), b.Interface()) {
			common = append(common, name)
		}
	}
	return common
}

// everyFieldExported reports whether a struct type hands all of its fields to a
// reader outside its own package.
func everyFieldExported(typ reflect.Type) bool {
	for i := range typ.NumField() {
		if !typ.Field(i).IsExported() {
			return false
		}
	}
	return true
}

// The three fields of the key that are not the hash each bound what a waiver
// reaches, and each is tested with the other three held fixed — including the
// content hash, so every case below is one waiver meeting code that reads
// identically somewhere else. That is the only interesting shape: two records
// over texts that already differ key differently on the hash alone, whatever
// these three say.
//
// `anchor.side` is round 8's finding side-omitted-from-identity-keys, and the
// case is the sharpest of the three. §6.1.2 resolves the two sides against
// different trees, so a line removed from the merge base and the line that
// replaced it in the head hash the same whenever the edit moved the text rather
// than rewriting it — and a waiver over the deletion would then silence a
// finding about the addition, in a file the reviewer never waived anything in.
func TestAWaiverReachesOneClassInOneFileInOneTree(t *testing.T) {
	waived, _ := theSameDefectAtTheSameCode(t)
	key := WaiverKeyOf(&waived)

	for name, elsewhere := range map[string]func(*Finding){
		"another file":   func(record *Finding) { record.Anchor.Path = "app/Models/Invoice.php" },
		"another class":  func(record *Finding) { record.Class = "unhandled-error" },
		"the other tree": func(record *Finding) { record.Anchor.Side = git.Left },
	} {
		t.Run(name, func(t *testing.T) {
			other, _ := theSameDefectAtTheSameCode(t)
			elsewhere(&other)
			assert.NotEqual(t, key, WaiverKeyOf(&other),
				"§7.4.2: a waiver suppresses the same class at the same code in the same file, and nothing further")
		})
	}
}

// §7.4.2 is the trust-economy argument for a narrow key, and it is a claim in
// two directions. A key too wide silences a finding the author never waived; a
// key too narrow makes the author dismiss the same thing every round, which
// costs the reviewer's standing just as surely.
//
// The key answers both by carrying the anchored code itself where §6.4.1's
// dedup key carries the line number. Code that moved down the file is the same
// code, so the waiver still holds — the two line numbers are deliberately not in
// the key, and this is what that buys — and §1.4 normalises before the digest,
// so re-indenting the body of the branch is not a change to it either. Code that
// was edited is not the same code, so the waiver stops: §7.4.2 says that is
// exactly when the judgement behind it should be revisited, and a waiver
// outliving the code it was written over is a silence nobody chose.
func TestAWaiverStopsMatchingOnceTheAnchoredLinesChange(t *testing.T) {
	waived, _ := theSameDefectAtTheSameCode(t)
	key := WaiverKeyOf(&waived)

	for name, unchanged := range map[string]func(*Finding){
		"the same code further down the file": func(record *Finding) {
			record.Anchor.StartLine, record.Anchor.Line = 61, 63
		},
		"the same code re-indented": func(record *Finding) {
			record.Anchor.ContentHash = hashOf(t, []string{"if ($discount) {", "\t$total -= $discount;", "}  "})
		},
	} {
		t.Run(name, func(t *testing.T) {
			later, _ := theSameDefectAtTheSameCode(t)
			unchanged(&later)
			assert.Equal(t, key, WaiverKeyOf(&later),
				"§7.4.2: the waiver holds over the same unchanged code, or the author re-dismisses it every round")
		})
	}

	edited, _ := theSameDefectAtTheSameCode(t)
	edited.Anchor.ContentHash = hashOf(t, []string{"if ($discount && $total > 0) {", "    $total -= $discount;", "}"})
	assert.NotEqual(t, key, WaiverKeyOf(&edited),
		"§7.4.2: the waiver stops once that code changes, which is when the judgement behind it is worth revisiting")
}
