package finding

import (
	"testing"

	"github.com/deligoez/cr/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §6.4.3 retains a suppressed duplicate rather than dropping it, and has it
// name the representative in `duplicate_of`. The pair is what makes the
// suppression readable afterwards: the record is still in the round, and the
// field says which record spoke in its place.
//
// The group here is three records at one anchored line in one class, from two
// roles, and the representative is settled by grade so nothing about the
// answer depends on which role filed first. What is asserted alongside the
// marking is the shape of the overlap summary §6.4.3 asks for: the
// representative, the contributing roles with the role that filed twice named
// once, and how many records were retired.
func TestEveryRecordButTheRepresentativeIsMarkedADuplicateOfIt(t *testing.T) {
	order := corpusOrderAcrossTwoLayers(t)

	first := duplicateRecord("f1", roleBelowInCorpus, GradeArgued, SeverityLow)
	strongest := duplicateRecord("f2", roleAboveInCorpus, GradeProbed, SeverityLow)
	third := duplicateRecord("f3", roleBelowInCorpus, GradeCited, SeverityLow)

	overlaps := MarkDuplicates([]*Finding{first, strongest, third}, order)

	assert.Empty(t, strongest.DuplicateOf,
		"§6.4.3 marks the suppressed records, and the representative is not one of them")
	assert.Equal(t, "f2", first.DuplicateOf, "§6.4.3: a suppressed duplicate names the representative")
	assert.Equal(t, "f2", third.DuplicateOf)

	require.Len(t, overlaps, 1, "one anchored line the roles converged on is one overlap")
	assert.Equal(t, DedupKeyOf(strongest), overlaps[0].Key,
		"the summary is filed under the key §6.4.1 grouped on")
	assert.Equal(t, "f2", overlaps[0].Representative)
	assert.Equal(t, []string{roleBelowInCorpus, roleAboveInCorpus}, overlaps[0].Roles,
		"§6.4.3 reports the contributing roles, and a role that filed twice contributed once")
	assert.Equal(t, 2, overlaps[0].Suppressed)
	assert.Equal(t, 2, overlaps.Suppressed(), "the round's count is the groups' counts")
}

// A record no other role duplicated is the representative of its own group, and
// §6.4.3 has nothing to say about it: no `duplicate_of` is written, and the
// overlap summary does not list it.
//
// The second half is the one worth fixing. Every finding is in some group, so a
// summary that reported groups of one would be the round's findings under
// another name — and a reader looking for where two roles agreed would have to
// find it inside a list of everything.
func TestARecordNoRoleDuplicatedIsNeitherMarkedNorReported(t *testing.T) {
	order := corpusOrderAcrossTwoLayers(t)

	here := duplicateRecord("f1", roleBelowInCorpus, GradeCited, SeverityHigh)
	elsewhere := duplicateRecord("f2", roleAboveInCorpus, GradeCited, SeverityHigh)
	elsewhere.Anchor.Line = here.Anchor.Line + 9
	require.NotEqual(t, DedupKeyOf(here), DedupKeyOf(elsewhere),
		"the two must fall in different groups for this to be about groups of one")

	overlaps := MarkDuplicates([]*Finding{here, elsewhere}, order)

	assert.Empty(t, here.DuplicateOf, "nothing suppressed it, so nothing named a representative")
	assert.Empty(t, elsewhere.DuplicateOf)
	assert.Empty(t, overlaps, "§6.4.3 reports where roles overlapped, and here they did not")
	assert.Zero(t, overlaps.Suppressed())
}

// §9.2 puts the side in the anchor and §6.4.1's key reads it, so two records at
// one line number on opposite sides are two groups — and §6.4.3 must therefore
// suppress neither of them.
//
// This is the same defect round 8 and round 12 reported against the key, seen
// from the end it costs something at: a key that dropped the side would retire
// the record about the deleted line here, naming a representative about a line
// the pull request added, and the reviewer would never see the first.
func TestTheTwoSidesOfOneLineNumberSuppressNeitherRecord(t *testing.T) {
	order := corpusOrderAcrossTwoLayers(t)

	removed := duplicateRecord("f1", roleBelowInCorpus, GradeCited, SeverityHigh)
	removed.Anchor.Side = git.Left
	added := duplicateRecord("f2", roleAboveInCorpus, GradeProbed, SeverityCritical)

	overlaps := MarkDuplicates([]*Finding{removed, added}, order)

	assert.Empty(t, removed.DuplicateOf,
		"§9.2: the merge base's line 44 and the head's line 44 are not the same code")
	assert.Empty(t, added.DuplicateOf)
	assert.Empty(t, overlaps)
}

// §10.1.6 reports duplicates suppressed in the current round, and §11.1 puts
// that count among the seven reports `--quiet` may never suppress. cap.go's
// HonestyDisclosure is the shape the writer holding that exemption consumes, so
// implementing it is what makes the count reachable through the exempt channel
// rather than through an ordinary informational line.
//
// The zero case is asserted as well, and it is not a formality: a count that
// appeared only when it was non-zero would read identically whether no two
// roles overlapped or the dedup step never ran, and the second is a round whose
// draft looks complete while carrying every duplicate the roles filed.
func TestTheDuplicateCountIsAnHonestyDisclosure(t *testing.T) {
	order := corpusOrderAcrossTwoLayers(t)

	var quietProof HonestyDisclosure = Overlaps{}
	assert.Equal(t, "0 records suppressed as duplicates across 0 anchored lines",
		quietProof.Disclosure(), "§11.1 prints the count whatever it is")

	overlaps := MarkDuplicates([]*Finding{
		duplicateRecord("f1", roleBelowInCorpus, GradeArgued, SeverityLow),
		duplicateRecord("f2", roleAboveInCorpus, GradeProbed, SeverityLow),
		duplicateRecord("f3", roleBelowInCorpus, GradeCited, SeverityLow),
	}, order)

	quietProof = overlaps
	assert.Equal(t, "2 records suppressed as duplicates across 1 anchored lines",
		quietProof.Disclosure(), "§10.1.6: how many records were retired, and at how many places")
}
