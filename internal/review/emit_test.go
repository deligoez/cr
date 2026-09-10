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
		Units:      units,
		Pairs:      nil,
		Mapped:     true,
		Candidates: reinvention.Attachments{Attached: []reinvention.Attachment{}, Unavailable: []reinvention.Unavailable{}},
		Hits:       []rule.Attachment{{Unit: "u1", Hits: []rule.Hit{}}, {Unit: "u2", Hits: []rule.Hit{}}},
		Tests:      []testadequacy.Attachment{empty, empty},
		Unmapped:   []UnmappedUnit{{Unit: "u2", Kind: finding.KindQuestion}},
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

// A hunk quoting a fence of its own is fenced one backtick longer, so the
// author's code cannot close the block and have the rest read as prompt.
func TestAHunkIsFencedLongerThanAnyFenceItQuotes(t *testing.T) {
	r := handRound()
	r.Units[0].Texts = []string{"@@ -1 +1 @@\n+s := \"````\""}

	text := Emit(r)[0].Text
	assert.Contains(t, text, "`````diff\n@@ -1 +1 @@\n+s := \"````\"\n`````")
}

// Every prompt names the NDJSON path its role writes to, and states §6.1's
// record schema together with the fields §6.1.4 forbids the agent to write
// (§4.6.2); an intent prompt also states §3.3's claim record.
//
// The path is asserted against cr merge's own reading of it. finding.RoleForFile
// is what binds a merged file's records to a role (§6.1.3), so a prompt naming a
// path whose base name bound some other role — or none — would have the role's
// records refused or misattributed on the way in.
func TestEveryPromptNamesItsOutputAndStatesTheRecordContract(t *testing.T) {
	for _, prompt := range Emit(handRound()) {
		assert.Equal(t, "/state/pr-7/fanout/1/"+prompt.Unit+"/review-"+prompt.Role+".ndjson", prompt.Output)
		bound, ok := finding.RoleForFile(prompt.Output)
		assert.True(t, ok)
		assert.Equal(t, prompt.Role, bound, "cr merge attributes the file to the role that wrote it")

		assert.Contains(t, prompt.Text, "\n    "+prompt.Output+"\n")
		for _, row := range []string{
			"- id: required", "- kind: required", "- axis: computed by cr", "- claim: optional",
			"- grade: computed by cr\n", "- round: stamped by cr", "- origin: computed by cr",
			"- disposition: optional, and §6.1.4 reserves it to cr", "- suppressed_by: optional\n",
		} {
			assert.Contains(t, prompt.Text, row)
		}
		assert.Contains(t, prompt.Text, "You may not write axis, grade, state, disposition, duplicate_of, "+
			"thread_id, span_hash, issue_hash, a citation's content_hash, a citation's origin, round, head.")

		claims := strings.Contains(prompt.Text, "## Claim record (§3.3)")
		assert.Equal(t, prompt.Axis == axis.Intent, claims, "%s on %s", prompt.Role, prompt.Unit)
		if claims {
			assert.Contains(t, prompt.Text, "- span_hash: computed by cr")
			assert.Contains(t, prompt.Text, "source is one of description, acceptance, comment, note")
		}
	}
}
