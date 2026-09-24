package finding

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discountGuard is the anchored code every record below is about: three lines
// of one branch, so §9.2's multi-line pre-image is the one under test rather
// than the single-line special case.
var discountGuard = []string{"if ($discount) {", "    $total -= $discount;", "}"}

// orderBefore and orderAfter are the context window around every copy of the
// guard orderFile holds, three lines on each side as §9.2 records it.
var (
	orderBefore = []string{"public function total(): int", "{", "    $total = $this->subtotal();"}
	orderAfter  = []string{"    return $total;", "}", "public function subtotal(): int"}
)

// The first anchored line of each copy of the guard orderFile holds.
const (
	// guardAt, guardMovedAt and guardFarAt are the same code in the same
	// context at three places in the file.
	guardAt      = 12
	guardMovedAt = 61
	guardFarAt   = 400
	// reindentedAt is the guard re-indented, in the same context.
	reindentedAt = 203
	// editedAt is the guard with its condition edited, in the same context.
	editedAt = 303
	// contextEditedAt is the guard unchanged, below an edit to the line
	// after the lone `{` of its context.
	contextEditedAt = 503
)

// orderFile is the file every fixture tree below holds: filler lines, and the
// guard placed at each of the lines above with its context window around it.
func orderFile() []string {
	lines := make([]string, 0, 600)
	for n := 1; n <= 600; n++ {
		lines = append(lines, fmt.Sprintf("// filler %d", n))
	}
	place := func(at int, before, guard, after []string) {
		copy(lines[at-1-len(before):], slices.Concat(before, guard, after))
	}
	for _, at := range []int{guardAt, guardMovedAt, guardFarAt} {
		place(at, orderBefore, discountGuard, orderAfter)
	}
	place(reindentedAt, orderBefore, []string{"if ($discount) {", "\t$total -= $discount;", "}  "}, orderAfter)
	place(editedAt, orderBefore,
		[]string{"if ($discount && $total > 0) {", "    $total -= $discount;", "}"}, orderAfter)
	place(contextEditedAt,
		[]string{"public function total(): int", "{", "    $total = $this->subtotal() - $this->credit();"},
		discountGuard, orderAfter)
	return lines
}

// everyPathHolding is a Trees whose head and merge base hold every path as
// lines, so a key can be formed for any path and either side.
func everyPathHolding(lines []string) Trees {
	read := func(string) ([]string, bool, error) { return lines, true, nil }
	return Trees{Head: read, MergeBase: read}
}

// orderTrees are the trees every record of this file is keyed against.
func orderTrees() Trees {
	return everyPathHolding(orderFile())
}

// keyOf is §7.4.1's key of record against orderTrees.
func keyOf(t *testing.T, record *Finding) WaiverKey {
	t.Helper()
	key, err := WaiverKeyOf(orderTrees(), record)
	require.NoError(t, err)
	return key
}

// §9.2.4: the key hash StampAnchor stores is the one WaiverKeyOf reads the tree
// for, byte for byte, so a waiver keyed from either matches a waiver keyed from
// the other — every waiver and posted-index entry written before the hash was
// stored still matches the records written after it. And a stored hash is
// used as it stands: a record keyed without a tree must not need one.
func TestTheStampedKeyHashIsTheKeyTheTreeGives(t *testing.T) {
	for _, at := range []int{1, 7, 12} {
		record := Finding{Class: "unguarded-discount", Anchor: Anchor{Path: "app/Order.php", Side: "RIGHT"}}
		anchoredAt(t, &record, at)
		require.NotEmpty(t, record.Anchor.ContextHash, "StampAnchor stores the key hash")

		stamped := keyOf(t, &record)
		unstamped := record
		unstamped.Anchor.ContextHash = ""

		assert.Equal(t, keyOf(t, &unstamped), stamped, "line %d: a key from the tree is the stamped key", at)
	}

	record := Finding{Class: "unguarded-discount", Anchor: Anchor{Path: "app/Order.php", Side: "RIGHT"}}
	anchoredAt(t, &record, 7)
	noTree := Trees{}
	key, err := WaiverKeyOf(noTree, &record)
	require.NoError(t, err, "a stamped record is keyed without reading a tree")
	assert.Equal(t, record.Anchor.ContextHash, key.ContentHash)
}

// anchoredAt moves record's anchor to the three lines from at and stamps it
// against orderTrees, as `cr record` stamps an anchor against the round's head.
func anchoredAt(t *testing.T, record *Finding, at int) {
	t.Helper()
	record.Anchor.StartLine, record.Anchor.Line = at, at+len(discountGuard)-1
	require.NoError(t, StampAnchor(orderTrees(), "fixture.ndjson", 1, &record.Anchor))
}

// keyFields are the fields of §6.1's table §7.4.1's key is made of, in the
// dotted form fieldsInCommon reports: §7.4.1's path and class, the anchored code
// with the context window around it, plus the `anchor.side` round 8's finding
// side-omitted-from-identity-keys adds. `anchor.context_hash` is the same
// code's key hash stamped in advance (§9.2.4), so the same code agrees on it.
var keyFields = []string{
	"Class", "Anchor.Path", "Anchor.Side", "Anchor.ContentHash", "Anchor.ContextBefore", "Anchor.ContextAfter",
	"Anchor.ContextHash",
}

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

	assert.Equal(t, keyOf(t, &waived), keyOf(t, &reworded),
		"§7.4.1: a waived finding must not return under a reworded summary, a different producer, or a later round")
}

// theSameDefectAtTheSameCode is a record a reviewer waived and the record a
// later round produces about the same defect: one class over one range of one
// file, written by another role, in another register, at another severity, with
// another summary, against another head.
//
// Every field of §6.1's table outside the key differs, including the two line
// numbers — the code and its context have moved down the file without changing,
// which §7.4.2 says is still the same waived code.
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
			StartLine:     guardAt,
			Line:          guardAt + 2,
			ContentHash:   content,
			ContextBefore: slices.Clone(orderBefore),
			ContextAfter:  slices.Clone(orderAfter),
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
			StartLine:     guardMovedAt,
			Line:          guardMovedAt + 2,
			ContentHash:   content,
			ContextBefore: slices.Clone(orderBefore),
			ContextAfter:  slices.Clone(orderAfter),
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
	key := keyOf(t, &waived)

	for name, elsewhere := range map[string]func(*Finding){
		"another file":   func(record *Finding) { record.Anchor.Path = "app/Models/Invoice.php" },
		"another class":  func(record *Finding) { record.Class = "unhandled-error" },
		"the other tree": func(record *Finding) { record.Anchor.Side = git.Left },
	} {
		t.Run(name, func(t *testing.T) {
			other, _ := theSameDefectAtTheSameCode(t)
			elsewhere(&other)
			assert.NotEqual(t, key, keyOf(t, &other),
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
	key := keyOf(t, &waived)

	for name, at := range map[string]int{
		"the same code further down the file": guardFarAt,
		"the same code re-indented":           reindentedAt,
	} {
		t.Run(name, func(t *testing.T) {
			later, _ := theSameDefectAtTheSameCode(t)
			anchoredAt(t, &later, at)
			assert.Equal(t, key, keyOf(t, &later),
				"§7.4.2: the waiver holds over the same unchanged code, or the author re-dismisses it every round")
		})
	}

	edited, _ := theSameDefectAtTheSameCode(t)
	anchoredAt(t, &edited, editedAt)
	assert.NotEqual(t, key, keyOf(t, &edited),
		"§7.4.2: the waiver stops once that code changes, which is when the judgement behind it is worth revisiting")
}

// §7.4.1 hashes the context window with the anchored lines, and §7.4.2 stops a
// waiver once "that code or its context changes". The anchored lines below are
// the waived guard to the byte, so §9.2's anchor hash is the waived record's;
// only the line after the lone `{` above them was edited. A key over the
// anchored lines alone would keep silencing a finding whose surroundings the
// reviewer never read.
func TestAWaiverStopsMatchingOnceTheContextAroundTheAnchoredLinesChanges(t *testing.T) {
	waived, _ := theSameDefectAtTheSameCode(t)
	key := keyOf(t, &waived)

	moved, _ := theSameDefectAtTheSameCode(t)
	anchoredAt(t, &moved, contextEditedAt)

	require.Equal(t, waived.Anchor.ContentHash, moved.Anchor.ContentHash,
		"§9.2's content hash stays over the anchored lines alone, and those did not change")
	require.NotEqual(t, waived.Anchor.ContextBefore, moved.Anchor.ContextBefore)
	assert.NotEqual(t, key, keyOf(t, &moved),
		"§7.4.2: the waiver stops once the code's context changes")
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

			waiver, err := WaiverFor(orderTrees(), &discarded)
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

	waiver, err := WaiverFor(orderTrees(), &discarded)
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

			waiver, err := WaiverFor(orderTrees(), &discarded)
			require.NoError(t, err)
			assert.Equal(t, disposition, waiver.Disposition,
				"§7.4.5: the waiver records the disposition §7.2 set on the record it was written for")
			assert.Equal(t, keyOf(t, &discarded), waiver.WaiverKey,
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

			waiver, err := WaiverFor(orderTrees(), &discarded)
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
	key := keyOf(t, &waived)
	require.Equal(t, "app/Models/Order.php", waived.Anchor.Path)

	unchanged, _ := theSameDefectAtTheSameCode(t)
	require.Equal(t, key, keyOf(t, &unchanged),
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
			assert.NotEqual(t, key, keyOf(t, &nearby),
				"§7.4.6's path-prefix widening is dropped from v0.1: a waiver reaches one path, exactly")
		})
	}
}

// §7.4.1's pre-image, spelled out: the context before, the anchored lines and
// the context after, joined by LF and normalised per §1.4 as one text. A key
// formed any other way — the three hashed apart, or the window left out —
// would agree with the waivers cr has written on nothing.
func TestTheKeyHashIsTheContextAndTheAnchoredLinesJoinedByLF(t *testing.T) {
	waived, _ := theSameDefectAtTheSameCode(t)
	want, err := text.NormalisedHash(strings.Join(slices.Concat(orderBefore, discountGuard, orderAfter), "\n"))
	require.NoError(t, err)
	assert.Equal(t, want, keyOf(t, &waived).ContentHash)

	alone, err := ContextKeyHash(nil, discountGuard, nil)
	require.NoError(t, err)
	assert.Equal(t, hashOf(t, discountGuard), alone,
		"with no context on either side the key hashes the text §9.2's anchor hash does")
}

// The anchored lines are read again from the tree the anchor's side names, and
// an anchor that tree cannot give its lines for is cr's own state gone wrong:
// the stamp resolved it there. It is refused as the file failure §11.2 codes 3,
// never keyed on fewer lines, because a key over other lines waives code nobody
// read.
func TestAKeyIsRefusedForAnAnchorItsTreeCannotGiveLinesFor(t *testing.T) {
	read := func(path string) ([]string, bool, error) {
		if path == "app/Models/Gone.php" {
			return nil, false, nil
		}
		return orderFile(), true, nil
	}
	trees := Trees{Head: read, MergeBase: read}
	for name, broken := range map[string]func(*Anchor){
		"a side naming no tree":         func(a *Anchor) { a.Side = "MIDDLE" },
		"a path the tree does not hold": func(a *Anchor) { a.Path = "app/Models/Gone.php" },
		"a range past the end":          func(a *Anchor) { a.StartLine, a.Line = 600, 601 },
		"a range starting at line 0":    func(a *Anchor) { a.StartLine = 0 },
		"a range running backwards":     func(a *Anchor) { a.StartLine, a.Line = guardAt+2, guardAt },
	} {
		t.Run(name, func(t *testing.T) {
			record, _ := theSameDefectAtTheSameCode(t)
			broken(&record.Anchor)

			key, err := WaiverKeyOf(trees, &record)

			var unusable *state.FileError
			require.ErrorAs(t, err, &unusable)
			assert.Equal(t, state.UnusableHint, unusable.Hint())
			assert.Equal(t, WaiverKey{}, key)
		})
	}
}

// The refusal above is for a range the tree cannot give, and only for that: a
// single-line anchor and one ending on the file's last line are ranges the tree
// holds, and each is keyed over exactly its own lines. Refusing either would
// leave the most common anchor, and every one at the end of a file, unwaivable.
func TestAKeyIsFormedForAnAnchorOnOneLineOrEndingOnTheLastLine(t *testing.T) {
	lines := orderFile()
	for name, span := range map[string][2]int{
		"one line":                {guardAt, guardAt},
		"ending on the last line": {len(lines) - 1, len(lines)},
	} {
		t.Run(name, func(t *testing.T) {
			record, _ := theSameDefectAtTheSameCode(t)
			record.Anchor.StartLine, record.Anchor.Line = span[0], span[1]

			key, err := WaiverKeyOf(orderTrees(), &record)

			require.NoError(t, err)
			want, err := ContextKeyHash(record.Anchor.ContextBefore, lines[span[0]-1:span[1]], record.Anchor.ContextAfter)
			require.NoError(t, err)
			assert.Equal(t, want, key.ContentHash)
		})
	}
}
