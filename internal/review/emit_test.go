package review

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/reinvention"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/symbol"
	"github.com/deligoez/cr/internal/testadequacy"
	"github.com/deligoez/cr/internal/unit"
)

// handRound is a round of two units and two active roles, one of them on the
// intent axis, whose mapping maps u1 and leaves u2 unmapped.
func handRound() *Round {
	units := []Unit{
		{Unit: unit.Unit{ID: "u1", Path: "a.go", Side: git.Right, HunkRanges: []unit.Range{{Start: 1, End: 1}}},
			Texts: []string{"@@ -1 +1 @@\n-old\n+new"}, FanOut: "/state/pr-7/fanout/1/u1"},
		{Unit: unit.Unit{ID: "u2", Path: "b.go", Side: git.Right, HunkRanges: []unit.Range{{Start: 4, End: 4}}},
			Texts: []string{"@@ -4 +4 @@\n-before\n+after"}, FanOut: "/state/pr-7/fanout/1/u2"},
	}
	empty := testadequacy.Attachment{Paths: []string{}, Symbols: []string{}, Unavailable: []testadequacy.Unavailable{}}
	return &Round{
		Round: 1, Head: "abc123",
		Roles: []role.Role{
			{ID: "intent-coverage", Title: "Intent coverage", Axis: axis.Intent, Instructions: "Map it."},
			{ID: "correctness", Title: "Correctness", Axis: axis.Correctness, Instructions: "Check it."},
		},
		Active:     []string{"intent-coverage", "correctness"},
		Places:     []string{"intent-coverage", "correctness"},
		Units:      units,
		Pairs:      nil,
		Mapped:     true,
		Candidates: reinvention.Attachments{Attached: []reinvention.Attachment{}, Unavailable: []reinvention.Unavailable{}},
		Hits:       []rule.Attachment{{Unit: "u1", Hits: []rule.Hit{}}, {Unit: "u2", Hits: []rule.Hit{}}},
		Tests:      []testadequacy.Attachment{empty, empty},
		Unmapped:   []UnmappedUnit{{Unit: "u2", Kind: finding.KindQuestion}},
		Contract:   "/state/pr-7/rounds/1/contract.md",
	}
}

// A round holding no unit emits no prompt, as an empty list rather than a nil
// one. Nothing between the round's unit records and Emit refuses an empty set —
// a diff that clusters into no unit leaves exactly that — and every other
// fixture here has units, so none asks what the fan-out is when one factor of
// roles times units is zero.
func TestARoundWithNoUnitEmitsAnEmptyListOfPrompts(t *testing.T) {
	r := handRound()
	r.Units, r.Hits, r.Tests = []Unit{}, []rule.Attachment{}, []testadequacy.Attachment{}

	prompts := Emit(r)

	assert.Empty(t, prompts)
	assert.NotNil(t, prompts, "§12: an empty list serialises as [], never null")
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

// §4.3.1's candidates, with the line standing in for an empty list only where
// the list is empty. An added symbol with a qualifying candidate lists it and
// says nothing about having none; one without says so. A prompt carrying both
// for one symbol hands the role a contradiction, and the absence is the half a
// role would act on.
func TestAnAddedSymbolSaysNoCandidateQualifiedOnlyWhenNoneDid(t *testing.T) {
	r := handRound()
	r.Units[0].HunkRanges = []unit.Range{{Start: 1, End: 2}}
	r.Candidates.Attached = []reinvention.Attachment{
		{
			Added: symbol.Decl{Path: "a.go", Line: 1, Name: "formatMoneys", Kind: symbol.Function, Params: 2},
			Candidates: []symbol.Decl{
				{Path: "money.go", Line: 3, Name: "FormatMoney", Kind: symbol.Function, Params: 2},
			},
		},
		{
			Added:      symbol.Decl{Path: "a.go", Line: 2, Name: "roundCents", Kind: symbol.Function, Params: 1},
			Candidates: []symbol.Decl{},
		},
	}

	text := Emit(r)[0].Text

	assert.Contains(t, text, "- added function formatMoneys (2 params) at a.go:1\n"+
		"  - candidate function FormatMoney (2 params) at money.go:3\n")
	assert.Contains(t, text, "- added function roundCents (1 params) at a.go:2\n"+
		"  - no candidate qualified under §4.3.2\n")
	assert.Equal(t, 1, strings.Count(text, "no candidate qualified"))
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

// §4.3.6's hits, and the instruction §2.6.1.5 attaches to them, reach only a
// unit that has one. A unit with a hit is told to confirm or drop it and never
// that no detector matched; a unit with none is told that, and not handed an
// instruction about hits it does not have.
func TestAUnitIsToldNoDetectorMatchedOnlyWhenNoneDid(t *testing.T) {
	r := handRound()
	r.Hits[0].Hits = []rule.Hit{{RuleID: "no-panic", Path: "a.go", Line: 1, Text: "new"}}

	prompts := Emit(r)
	require.Len(t, prompts, 4)
	for _, prompt := range prompts {
		hit := prompt.Unit == "u1"
		assert.Equalf(t, hit, strings.Contains(prompt.Text, "- rule no-panic at a.go:1: new"),
			"%s on %s", prompt.Role, prompt.Unit)
		assert.Equalf(t, hit, strings.Contains(prompt.Text, "confirm a hit to raise it, or drop it"),
			"%s on %s", prompt.Role, prompt.Unit)
		assert.Equalf(t, !hit, strings.Contains(prompt.Text, "No rule's detector matched a changed line of this unit."),
			"%s on %s", prompt.Role, prompt.Unit)
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

// Once a mapping is recorded, a unit mapped to a claim lists it and a unit
// mapped to none says so, and neither prompt carries the other's line. The
// case above covers only the first pass, where neither line may appear; an
// intent role told "no claim" above a claim it is then shown would raise
// §4.1.2's question about a unit the mapping covers.
func TestAMappedUnitListsItsClaimAndOnlyAnUnmappedOneSaysItHasNone(t *testing.T) {
	r := handRound()
	r.Claims = []intent.Claim{{ID: "CR-7#c1", Text: "The total sums the subtotal and the shipping."}}
	pair := mapping.Pair{Claim: "CR-7#c1", Unit: "u1"}
	pair.Head, pair.Round = r.Head, r.Round
	r.Pairs = []mapping.Pair{pair}

	prompts := Emit(r)
	require.Len(t, prompts, 4)
	for _, prompt := range prompts {
		mapped := prompt.Unit == "u1"
		assert.Equalf(t, mapped,
			strings.Contains(prompt.Text, "- CR-7#c1: The total sums the subtotal and the shipping."),
			"%s on %s", prompt.Role, prompt.Unit)
		assert.Equalf(t, !mapped, strings.Contains(prompt.Text, "The mapping maps no claim to this unit."),
			"%s on %s", prompt.Role, prompt.Unit)
	}
}

// A hunk quoting a fence of its own is fenced one backtick longer, so the
// author's code cannot close the block and have the rest read as prompt.
func TestAHunkIsFencedLongerThanAnyFenceItQuotes(t *testing.T) {
	r := handRound()
	r.Units[0].Texts = []string{"@@ -1 +1 @@\n+s := \"````\""}

	text := Emit(r)[0].Text
	assert.Contains(t, text, "`````diff\n@@ -1 +1 @@\n+s := \"````\"\n`````")
}

// The fence is exactly the length block promises: one backtick longer than the
// longest run the hunk holds, and three when it holds none. The case above
// finds its fence inside any longer one just as well, so it cannot tell a fence
// of five from a fence of twenty; the fence line asserted newline to newline
// can.
func TestAHunkFenceIsExactlyTheLengthBlockPromises(t *testing.T) {
	for name, c := range map[string]struct{ hunk, fence string }{
		"a hunk holding no backtick":      {"@@ -1 +1 @@\n-old\n+new", "```"},
		"a hunk quoting a fence of four":  {"@@ -1 +1 @@\n+s := \"````\"", "`````"},
		"a hunk holding a lone backtick":  {"@@ -1 +1 @@\n+s := `x`", "```"},
		"a hunk quoting a fence of three": {"@@ -1 +1 @@\n+```go", "````"},
	} {
		t.Run(name, func(t *testing.T) {
			r := handRound()
			r.Units[0].Texts = []string{c.hunk}

			assert.Contains(t, Emit(r)[0].Text, "\n"+c.fence+"diff\n"+c.hunk+"\n"+c.fence+"\n")
		})
	}
}

// forbiddenSentence is §6.1.4's fence as the prompt and the contract file state it.
const forbiddenSentence = "You may not write axis, grade, state, disposition, duplicate_of, thread_id, span_hash, " +
	"issue_hash, a citation's content_hash, a citation's origin, round, head. cr computes or stamps them, " +
	"and a record arriving with one is rejected with exit code 1 (§6.1.4, §2.3.3).\n"

// englishSentence is §6.1.1's rule as a whole line.
const englishSentence = "Write summary and evidence in English (§6.1.1), whatever language the issue, the " +
	"threads or the code comments are in; reader-facing prose is produced from them at draft time (§8.1).\n"

// schemaRows are rows of §6.1's schema as the contract file lists them.
var schemaRows = []string{
	"\n- id: required\n", "\n- kind: required\n", "\n- axis: computed by cr\n", "\n- claim: optional\n",
	"\n- grade: computed by cr\n", "\n- round: stamped by cr\n", "\n- origin: computed by cr\n",
	"\n- disposition: optional, and §6.1.4 reserves it to cr\n", "\n- suppressed_by: optional\n",
}

// §4.6.2 in 0.3.0: every prompt ends on its output section, which names the
// NDJSON path the role writes to with the record ids it may write, names the
// round's contract file, states §6.1.1's language rule and §6.1.4's fence, and
// no longer carries §6.1's schema, which is the contract file's; an intent
// prompt also states §3.3's claim record.
//
// The section is compared whole, and the path is asserted against cr merge's
// own reading of it. finding.RoleForFile is what binds a merged file's records
// to a role (§6.1.3), so a prompt naming a path whose base name bound some
// other role — or none — would have the role's records refused or misattributed.
func TestEveryPromptNamesItsOutputAndTheContractFile(t *testing.T) {
	r := handRound()
	for _, prompt := range Emit(r) {
		assert.Equal(t, "/state/pr-7/fanout/1/"+prompt.Unit+"/review-"+prompt.Role+".ndjson", prompt.Output)
		bound, ok := finding.RoleForFile(prompt.Output)
		assert.True(t, ok)
		assert.Equal(t, prompt.Role, bound, "cr merge attributes the file to the role that wrote it")

		_, section, found := strings.Cut(prompt.Text, "\n## Output (§4.6.2)\n\n")
		require.True(t, found, "%s on %s", prompt.Role, prompt.Unit)
		assert.Equal(t, "Write this role's records for this unit, one JSON object per line, to:\n\n"+
			"    "+prompt.Output+"\n\n"+
			"The file's name binds every record in it to role "+prompt.Role+": cr merge attributes a record to "+
			"the role whose file it arrived in and rejects one naming another (§6.1.3). With nothing to "+
			"raise, write nothing.\n\n"+
			"Give the records you write here the ids "+prompt.FirstID+" through "+prompt.LastID+", in order from "+
			prompt.FirstID+". No other prompt of round 1 is given any of them, and none is held by a stored "+
			"record. An id outside them may be another prompt's, and cr merge refuses an id two records carry, "+
			"with exit code 1 (§6.1).\n\n"+
			"A record's fields, the values each takes, and a citation's fields are §6.1's record schema, in the "+
			"round's contract file; read it before writing a record:\n\n"+
			"    /state/pr-7/rounds/1/contract.md\n\n"+
			englishSentence+"\n"+forbiddenSentence, section, "%s on %s", prompt.Role, prompt.Unit)
		for _, row := range schemaRows {
			assert.NotContains(t, section, row, "§6.1's schema is the contract file's, not the prompt's")
		}

		claims := strings.Contains(prompt.Text, "## Claim record (§3.3)")
		assert.Equal(t, prompt.Axis == axis.Intent, claims, "%s on %s", prompt.Role, prompt.Unit)
		if claims {
			assert.Contains(t, prompt.Text, "- span_hash: computed by cr")
			assert.Contains(t, prompt.Text, "source is one of description, acceptance, comment, note")
		}
	}
}

// The contract file carries §6.1's schema: every row, §6.1.1's language rule and
// §6.1.4's fence, each as a whole line, under a heading naming its round.
func TestTheContractFileCarriesTheRecordSchema(t *testing.T) {
	text := Contract(3)
	assert.True(t, strings.HasPrefix(text, "# Record contract for round 3 (§4.6.2)\n\n"+
		"Every prompt of round 3 names this file. A role writes its records to the path its prompt names, "+
		"one JSON object per line, with the ids its prompt names.\n\n## Record schema (§6.1)\n\n"+
		"A record carries §6.1's fields:\n- id: required\n"), text)
	for _, row := range schemaRows {
		assert.Contains(t, text, row)
	}
	assert.Contains(t, text, "\n"+englishSentence)
	assert.True(t, strings.HasSuffix(text, "\n\n"+forbiddenSentence), text)
}

// §2.6.1.4's standards without a detector reach the prompt of their axis's
// role, and the sentence saying no rule is injected appears only where none
// is. A correctness role told no correctness rule is injected, beneath the rule
// that is, would read that rule as withdrawn.
func TestARoleIsToldNoStandardIsInjectedOnlyWhereNoneIs(t *testing.T) {
	r := handRound()
	r.Rules = []rule.Resolved{{
		Rule: rule.Rule{
			ID: "handle-every-error", Title: "Every error is handled where it is returned.",
			Rationale: "A dropped error turns a failure into a wrong answer nobody sees.",
			Class:     "unchecked-error", Axis: axis.Correctness,
		},
		Path: "/home/.cr/rules/handle-every-error.json",
	}}

	prompts := Emit(r)
	require.Len(t, prompts, 4)
	for _, prompt := range prompts {
		injected := prompt.Axis == axis.Correctness
		assert.Equalf(t, injected, strings.Contains(prompt.Text, "rule handle-every-error (axis correctness"),
			"%s on %s", prompt.Role, prompt.Unit)
		assert.Equalf(t, !injected,
			strings.Contains(prompt.Text, "No rule of the "+prompt.Axis+" axis is injected as text."),
			"%s on %s", prompt.Role, prompt.Unit)
	}
}
