package finding

import (
	"encoding/json"
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
	for field := range typ.Fields() {
		if !field.IsExported() {
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

// §7.4.1 does not let a caller choose where a waiver reaches: the scope follows
// the disposition, `wrong` to the repository and `not-here` to the pull request.
// Both directions are asserted here, over one record that differs between the
// two runs in the disposition alone, so what moves the scope is the only thing
// that changed.
//
// The names are asserted too, because §7.4.7 prints the scope beside the
// disposition and §7.4.4 selects a file by it — a scope that could not say which
// of the two it is would leave both of those to guess.
func TestAWaiverScopeFollowsItsDisposition(t *testing.T) {
	for _, expected := range []struct {
		disposition Disposition
		scope       WaiverScope
		name        string
	}{
		{DispositionWrong, ScopeRepository, "repository"},
		{DispositionNotHere, ScopePullRequest, "pull-request"},
	} {
		t.Run(string(expected.disposition), func(t *testing.T) {
			discarded, _ := theSameDefectAtTheSameCode(t)
			discarded.Disposition = expected.disposition

			waiver, err := WaiverFor(&discarded)
			require.NoError(t, err)

			scope, err := waiver.Scope()
			require.NoError(t, err)
			assert.Equal(t, expected.scope, scope,
				"§7.4.1: a waiver written for %q is scoped there and nowhere else", expected.disposition)
			assert.Equal(t, expected.name, scope.String(),
				"§7.4.7 prints the scope, and §7.4.4 selects one of two files by it")
		})
	}

	assert.NotEqual(t, ScopeRepository, ScopePullRequest,
		"§7.2: the two dispositions are deliberately distinct, and collapsing their scopes collapses them")
}

// §7.4.3 is the asymmetric half of the rule, and the expensive one to get
// wrong. "This is wrong" generalises across the repository; "not worth saying
// here" is a fact about one pull request and MUST NOT silence the finding
// anywhere else. A `not-here` that reached the repository file would suppress a
// true finding in every later pull request, and the silence would be invisible:
// nothing is raised, so nothing is there to look at.
//
// Asserting that WaiverFor returns the narrow scope proves only that today's
// mapping is right. What keeps it right is that a scope cannot come from
// anywhere else, so this asserts the shape that makes that true: a waiver holds
// no scope of its own that could contradict its disposition, and WaiverScope
// keeps its only field to itself, so no composite literal outside the package
// can name a scope into existence the way `var s WaiverScope = "repository"`
// would if it were a defined string type.
func TestNotWorthSayingHereSilencesNothingBeyondItsPullRequest(t *testing.T) {
	discarded, _ := theSameDefectAtTheSameCode(t)
	discarded.Disposition = DispositionNotHere

	waiver, err := WaiverFor(&discarded)
	require.NoError(t, err)
	scope, err := waiver.Scope()
	require.NoError(t, err)
	assert.NotEqual(t, ScopeRepository, scope,
		"§7.4.3: not worth saying here must not silence the finding anywhere else")

	scopeType := reflect.TypeFor[WaiverScope]()
	waiverType := reflect.TypeFor[Waiver]()
	for field := range waiverType.Fields() {
		assert.NotEqual(t, scopeType, field.Type,
			"a waiver carrying a scope of its own could hold one that disagrees with its disposition")
	}

	require.NotZero(t, scopeType.NumField())
	assert.False(t, everyFieldExported(scopeType),
		"§7.4.3 is a fence only while Scope is the one thing that can produce a WaiverScope")
}

// §7.4.5 makes the disposition part of the waiver rather than a detail of the
// moment it was written, and §7.3.4 is why: `discarded-wrong` goes into the
// demotion numerator and `not-here` is excluded from it in as many words. A
// waiver that recorded only that something had been silenced would leave §7.3
// unable to tell a false positive from a deliberate silence, and a class that is
// always right and merely never worth saying would be demoted as imprecise.
//
// The round trip is the part that matters. §7.4.4 puts waivers in a file and
// §7.4.7 reads them back, so a disposition held only in memory would satisfy the
// letter of §7.4.5 and none of its purpose.
func TestAWaiverRecordsTheDispositionItWasWrittenFor(t *testing.T) {
	for _, disposition := range []Disposition{DispositionWrong, DispositionNotHere} {
		t.Run(string(disposition), func(t *testing.T) {
			discarded, _ := theSameDefectAtTheSameCode(t)
			discarded.Disposition = disposition

			waiver, err := WaiverFor(&discarded)
			require.NoError(t, err)
			assert.Equal(t, disposition, waiver.Disposition,
				"§7.4.5: the waiver records the disposition §7.2 set on the record it was written for")
			assert.Equal(t, WaiverKeyOf(&discarded), waiver.WaiverKey,
				"§7.4.1: and it covers that record, so the reason and the key describe one discard")

			written, err := json.Marshal(waiver)
			require.NoError(t, err)
			assert.Contains(t, string(written), `"disposition":"`+string(disposition)+`"`,
				"§7.3 reads the disposition off the stored waiver, so it has to be on the wire under that name")

			var read Waiver
			require.NoError(t, json.Unmarshal(written, &read))
			assert.Equal(t, waiver, read,
				"§7.4.4 stores the waiver in a file and §7.4.7 lists it back; nothing may be lost on the way")
		})
	}
}

// §7.2's table has exactly two dispositions, so the mapping of §7.4.1 is total
// over what a discard can carry — and nothing else may be waived. A record with
// no disposition is a discard §9.1 never recorded, and a value outside the two
// was written by something that is not cr, since §6.1.4 has cr write the field.
//
// The fail-closed direction is the whole point. A scope guessed for an unknown
// disposition would have to guess one of the two, and the tempting guess is
// repository — the wider file, on the record cr understands least, which is
// precisely the silence §7.4.3 forbids. Refusing costs a discard that has to be
// re-expressed; guessing costs a finding that stops being raised and never says
// so.
func TestARecordIsNotWaivedWithoutOneOfTheTwoDispositions(t *testing.T) {
	for name, disposition := range map[string]Disposition{
		"never dispositioned":     "",
		"a verb cr did not write": "accepted",
	} {
		t.Run(name, func(t *testing.T) {
			discarded, _ := theSameDefectAtTheSameCode(t)
			discarded.Disposition = disposition

			waiver, err := WaiverFor(&discarded)
			require.Error(t, err, "§7.2 has two dispositions, and a waiver may be written for neither more nor fewer")
			assert.Equal(t, Waiver{}, waiver,
				"a refused waiver must carry no key either, or a caller could store what it was refused")

			var unknown *UnknownDispositionError
			require.ErrorAs(t, err, &unknown)
			assert.Equal(t, string(disposition), unknown.Value,
				"the error names what was rejected, exactly as it was written")

			scope, err := waiver.Scope()
			require.Error(t, err)
			assert.Equal(t, WaiverScope{}, scope,
				"§7.4.3: an unknown disposition must not fall back to the repository, the wider of the two")
		})
	}
}

// Round 8's finding capability-without-mechanism, resolved by an approved
// decision: §7.4.6's path-prefix widening is dropped from v0.1. No command ever
// wrote a widened waiver, §7.4.1 fixed the key shape without one, and the
// legacy-area case the widening existed for is covered by a rule's `exempt`
// field per §2.6 — which keeps a rule off those paths before it produces a
// record, rather than silencing records after the fact.
//
// A decision not to build something leaves no code behind to read, so this is
// where it is written down. Matching is `==` over the four fields of the key,
// and a path that merely contains the waived one is a different path. The cases
// below hold the class, the side and the content hash fixed, so each is one
// waiver meeting code that reads identically in a file whose name extends the
// waived path or whose directory contains it — exactly the pairs a widening
// rule would join. Equality has no direction, so each case covers a waiver
// written above the file and a waiver written below it alike.
func TestAWaiverNeverWidensAlongThePath(t *testing.T) {
	waived, _ := theSameDefectAtTheSameCode(t)
	key := WaiverKeyOf(&waived)
	require.Equal(t, "app/Models/Order.php", waived.Anchor.Path)

	unchanged, _ := theSameDefectAtTheSameCode(t)
	require.Equal(t, key, WaiverKeyOf(&unchanged),
		"the waived path still matches itself, so the cases below fail on the path and not on the fixture")

	for _, path := range []string{
		"app",
		"app/Models",
		"app/Models/",
		"app/Models/Order",
		"app/Models/Order.php.orig",
		"app/Models/Order/Line.php",
	} {
		t.Run(path, func(t *testing.T) {
			nearby, _ := theSameDefectAtTheSameCode(t)
			nearby.Anchor.Path = path
			assert.NotEqual(t, key, WaiverKeyOf(&nearby),
				"§7.4.6's path-prefix widening is dropped from v0.1: a waiver reaches one path, exactly")
		})
	}
}
