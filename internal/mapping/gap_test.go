package mapping

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// gapsOf renders the derived entries as `claim@round` with the note each one
// carries, which is the whole of what §4.1.3 puts in the file apart from the
// head every record of it shares.
func gapsOf(gaps []*Gap) []string {
	rendered := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		rendered = append(rendered, gap.Claim+"/"+gap.SetAsideNote)
	}
	return rendered
}

// A claim the round's mapping maps to zero units raises §4.1.3's entry, and a
// claim it maps to any unit does not.
//
// Every way a pair can fail to cover a claim sits beside the pairs that do.
// c1 is mapped twice, which a derivation counting pairs would read as doubly
// covered and one keyed on the unit would read once; neither may raise it. c3 is
// mapped only by a pair of round 1, and round 2 is being asked about: §9.3.5
// makes an earlier round history and §3.4.6 makes round 1's `u1` a different
// piece of code, so a derivation over the whole file would hide round 2's
// unimplemented c3 behind it.
func TestAClaimMappedToZeroUnitsOfItsRoundRaisesAGap(t *testing.T) {
	pairs := []Pair{
		pair("CR-1#c1", "u1", 2),
		pair("CR-1#c1", "u2", 2),
		pair("CR-1#c3", "u1", 1),
		pair("CR-1#c4", "u2", 2),
	}

	gaps, dropped := Gaps([]string{"CR-1#c1", "CR-1#c2", "CR-1#c3", "CR-1#c4"}, pairs, 2, nil)

	assert.Equal(t, []string{"CR-1#c2/", "CR-1#c3/"}, gapsOf(gaps))
	assert.Empty(t, dropped, "a round that recorded no set-aside can drop none")
}

// A round whose every claim is mapped raises an empty list rather than a nil
// one, per §12.3: a caller serialising the answer prints `[]`, which is the
// difference between no gap and a question never asked.
func TestAFullyImplementedRoundRaisesAnEmptyGapListNotANilOne(t *testing.T) {
	gaps, dropped := Gaps([]string{"CR-1#c1"}, []Pair{pair("CR-1#c1", "u1", 1)}, 1, nil)

	require.NotNil(t, gaps)
	assert.Empty(t, gaps)
	require.NotNil(t, dropped, "§12.3 holds for the report as well as for the entries")
	assert.Empty(t, dropped)
}

// A re-derivation keeps the §4.1.8 set-aside of a claim that is still unmapped,
// and keeps nothing from a round that is not the one being derived.
//
// §4.1.6 permits the mapping to be re-recorded, so §4.1.7's derivation runs
// again over the same round. Round 9's `derived-state-clobbers-decision` and
// round 12's `derived-file-erases-recorded-decision` are the same finding twice:
// a literal re-derivation hands every claim a fresh entry with no
// `set_aside_note`, discarding the reviewer's judgement and silently
// re-imposing §10.2.3's completeness block with no report.
//
// c2's stamp is round 1's and c2 is unmapped in round 2, so a carry-forward
// that ignored the round would resurrect a decision made against a different
// diff.
//
// c4's entry is round 2's and carries no note, and c4 is now mapped. It is the
// difference between a stamp and a field: §4.1.3 writes `set_aside_note` on
// every entry, so a report that counted a vanished entry rather than a vanished
// decision would tell the reviewer they had lost a judgement nobody made.
//
// c5's stamp is round 1's and round 2 has no such claim, so round 2 raises no
// entry for it — and round 1's entry, stamp and all, is still in the file where
// §9.3.5 leaves it. It may not be reported dropped: nothing was dropped, and a
// report saying otherwise would send the reviewer to re-make a decision that
// still stands.
func TestAReDerivationKeepsTheSetAsideOfAClaimStillUnmapped(t *testing.T) {
	recorded := []Gap{
		{Claim: "CR-1#c1", Stamp: state.Stamp{Head: "abc123", Round: 2}, SetAsideNote: "CR-1#n1"},
		{Claim: "CR-1#c2", Stamp: state.Stamp{Head: "0f1e2d3", Round: 1}, SetAsideNote: "CR-1#n2"},
		{Claim: "CR-1#c3", Stamp: state.Stamp{Head: "abc123", Round: 2}, SetAsideNote: "CR-1#n3"},
		{Claim: "CR-1#c4", Stamp: state.Stamp{Head: "abc123", Round: 2}, SetAsideNote: ""},
		{Claim: "CR-1#c5", Stamp: state.Stamp{Head: "0f1e2d3", Round: 1}, SetAsideNote: "CR-1#n5"},
	}

	gaps, dropped := Gaps(
		[]string{"CR-1#c1", "CR-1#c2", "CR-1#c3", "CR-1#c4"},
		[]Pair{pair("CR-1#c3", "u1", 2), pair("CR-1#c4", "u2", 2)}, 2, recorded)

	assert.Equal(t, []string{"CR-1#c1/CR-1#n1", "CR-1#c2/"}, gapsOf(gaps),
		"§4.1.8's note survives a re-recorded mapping for a claim still unmapped, "+
			"and c3 raises no entry at all now that it is mapped")
	assert.Equal(t, []DroppedSetAside{{Claim: "CR-1#c3", SetAsideNote: "CR-1#n3"}}, dropped,
		"round 9's finding asks for the drop to be reported, and only for the drop: "+
			"c1 kept its stamp, c2's was never this round's to lose, c4 never had one, "+
			"and c5's entry is still in the file where §9.3.5 left it")
}
