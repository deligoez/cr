package finding

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waiverOver is a stored waiver covering one record's §7.4.1 key, under the id
// and disposition given.
//
// The key is read off the record through WaiverKeyOf rather than spelled out,
// because that is how the waiver §7.2 writes at triage is built: a fixture that
// spelled the four fields here would keep matching after a fifth joined the key
// and would stop measuring anything.
func waiverOver(id string, record *Finding, disposition Disposition) WaiverRecord {
	return WaiverRecord{
		ID:     id,
		Waiver: Waiver{WaiverKey: WaiverKeyOf(record), Disposition: disposition},
		WaiverProvenance: WaiverProvenance{
			Round: 1, PR: 7, Head: "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c",
			Reason: "the store's caller already handles this",
		},
	}
}

// ndjson is the records as the bytes a round's file is written from, so "no
// trace" can be asserted against text rather than against a slice length.
func ndjson(t *testing.T, records []*Finding) string {
	t.Helper()
	out := ""
	for _, record := range records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		out += string(line) + "\n"
	}
	return out
}

// §6.4.4: a finding an active waiver covers is dropped, counted, and leaves no
// record behind.
//
// The three assertions are the sentence taken apart. It is gone from what the
// merge hands on, so nothing downstream can write it to findings.ndjson; it was
// not marked instead of removed, which is what §9.1's exhaustive table makes
// impossible to do safely — round 3's `waived-record-no-state` is the finding,
// and a record dropped after it was written would sit in `draft` with no legal
// edge out while §10.2.4 blocked the round forever. And the count moved, which
// is the one thing §10.3's summary receives.
//
// The survivors are asserted in order as well, because §6.5.1 has the merged
// file read in the order the role files were read and a filter is the easiest
// place to lose that.
func TestAWaivedFindingIsDroppedCountedAndLeavesNoRecord(t *testing.T) {
	first := duplicateRecord("f1", "correctness", GradeCited, SeverityHigh)
	silenced := duplicateRecord("f2", "convention", GradeArgued, SeverityLow)
	silenced.Anchor.Line = 90
	silenced.Class = "reinvented-helper"
	silenced.Summary = "MoneyFormat repeats FormatMoney."
	third := duplicateRecord("f3", "correctness", GradeCited, SeverityMedium)
	third.Anchor.Line = 120

	kept, dropped := DropWaived(
		[]*Finding{first, silenced, third},
		[]WaiverRecord{waiverOver("wr1", silenced, DispositionWrong)},
	)

	require.Equal(t, []*Finding{first, third}, kept,
		"§6.4.4 drops the waived record and leaves the order of the rest")
	written := ndjson(t, kept)
	assert.NotContains(t, written, `"f2"`,
		"§6.4.4: a dropped finding is never written to findings.ndjson at all")
	assert.NotContains(t, written, "reinvented-helper")
	assert.NotContains(t, written, silenced.Summary)

	assert.Empty(t, silenced.State.String(),
		"§9.1 defines no state for a waived record, which is why §6.4.4 removes rather than marks")
	assert.Empty(t, silenced.DuplicateOf, "and nothing marked it as something else either")

	assert.Equal(t, Drops{Dropped: 1, Waivers: []string{"wr1"}}, dropped,
		"§6.4.4 counts the drop, and §7.4.7 is where the silence itself is read")
	assert.Equal(t, "1 finding(s) dropped by 1 active waiver(s), per §6.4.4", dropped.Disclosure())
}

// §7.4.2's narrowness, from both sides: the waiver matches the same class at
// the same unchanged code, and stops matching once that code changes.
//
// The second half is the half worth fixing in a test. A waiver that kept
// matching after the anchored lines changed would silence a finding about code
// the reviewer never judged, and the silence is the one failure nobody observes
// — the finding simply stops being raised.
func TestAWaiverStopsSilencingOnceTheAnchoredCodeChanges(t *testing.T) {
	unchanged := duplicateRecord("f1", "correctness", GradeCited, SeverityHigh)
	waiver := waiverOver("wp1", unchanged, DispositionNotHere)

	kept, dropped := DropWaived([]*Finding{unchanged}, []WaiverRecord{waiver})
	require.Empty(t, kept, "the same class at the same unchanged code is what the waiver covers")
	require.Equal(t, 1, dropped.Dropped)

	rewritten := duplicateRecord("f1", "correctness", GradeCited, SeverityHigh)
	rewritten.Anchor.ContentHash = "9e8d7c6b5a493827"

	kept, dropped = DropWaived([]*Finding{rewritten}, []WaiverRecord{waiver})

	assert.Equal(t, []*Finding{rewritten}, kept,
		"§7.4.2: the waiver stops suppressing once the code changes, which is when the judgement should be revisited")
	assert.Equal(t, Drops{Dropped: 0, Waivers: []string{}}, dropped)
	assert.Equal(t, "0 finding(s) dropped by 0 active waiver(s), per §6.4.4", dropped.Disclosure(),
		"the count is printed at zero, so a round where no waiver matched reads differently from one where the drop never ran")
}

// One waiver silencing two findings is one waiver, and two waivers reach the
// records whichever of §7.4.4's two files they came from.
//
// The first half is why Waivers is a set rather than a tally: §10.1.6 reports
// the waivers applied beside the findings dropped, and a list repeating an id
// once per finding would make one decision read as several.
func TestTheDropNamesEachApplyingWaiverOnceAcrossBothScopes(t *testing.T) {
	wide := duplicateRecord("f1", "correctness", GradeCited, SeverityHigh)
	again := duplicateRecord("f2", "convention", GradeArgued, SeverityLow)
	here := duplicateRecord("f3", "correctness", GradeCited, SeverityMedium)
	here.Anchor.Path = "internal/store/read.go"
	survivor := duplicateRecord("f4", "correctness", GradeCited, SeverityLow)
	survivor.Anchor.Path = "internal/store/open.go"

	// `again` carries `wide`'s whole §7.4.1 key: a second role's record of
	// the same class over the same anchored lines, which §6.4.1 would have
	// grouped and one waiver covers. §7.4.1's key carries no line, so this
	// holds however far apart the two anchors sit.
	require.Equal(t, WaiverKeyOf(wide), WaiverKeyOf(again))
	require.NotEqual(t, WaiverKeyOf(wide), WaiverKeyOf(survivor))
	require.NotEqual(t, WaiverKeyOf(here), WaiverKeyOf(survivor))

	kept, dropped := DropWaived(
		[]*Finding{wide, again, here, survivor},
		[]WaiverRecord{
			waiverOver("wr1", wide, DispositionWrong),
			waiverOver("wp1", here, DispositionNotHere),
		},
	)

	assert.Equal(t, []*Finding{survivor}, kept)
	assert.Equal(t, Drops{Dropped: 3, Waivers: []string{"wr1", "wp1"}}, dropped,
		"three findings, two waivers, each named once in the order it first applied")
}
