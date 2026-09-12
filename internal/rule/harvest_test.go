package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// posted is one comment of the scan, named by what makes it group.
func posted(pr, round int, record, class, body string) Comment {
	return Comment{PR: pr, Round: round, Record: record, Class: class, Body: body}
}

// classesOf renders the candidates as `class×occurrences`, which is the whole
// of what §2.6.3.2's threshold decided.
func classesOf(candidates []Candidate) []string {
	rendered := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		rendered = append(rendered, candidate.Class+"×"+itoa(candidate.Occurrences))
	}
	return rendered
}

// itoa keeps the rendering above readable without pulling strconv into a test
// that is otherwise about grouping.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + itoa(n%10)
}

// §2.6.3.1 groups by class and by §1.4's normal form of the body, and §2.6.3.2
// reports the group that reached the threshold.
//
// Every way a group can fail to form sits beside the group that does. The three
// `dropped-error` comments differ only in interior whitespace runs, trailing
// whitespace and line endings — §1.4 collapses exactly those, so they are one
// group of three. Leading indentation is deliberately not among them: §1.4's
// step 4 collapses a leading run to one SPACE rather than removing it, so a
// body that is merely indented does not group with an unindented one, which is
// a real limit of this grouping and not a defect in it. The fourth carries
// the same words under a different class, which §2.6.3.1 keeps apart because
// the class is what a harvested rule would declare. The fifth carries a
// different wording under the same class, which §1.4 keeps apart because it
// does not case-fold or strip punctuation.
func TestCommentsGroupByClassAndNormalisedBody(t *testing.T) {
	candidates, err := Harvest([]Comment{
		posted(7, 1, "f1", "dropped-error", "The error is dropped."),
		posted(7, 2, "f2", "dropped-error", "The   error\tis dropped.  "),
		posted(9, 1, "f3", "dropped-error", "The error is dropped.\r\n"),
		posted(9, 1, "f4", "unchecked-cast", "The error is dropped."),
		posted(9, 2, "f5", "dropped-error", "the error is dropped"),
	}, 3)
	require.NoError(t, err)

	assert.Equal(t, []string{"dropped-error×3"}, classesOf(candidates),
		"§1.4 collapses whitespace and line endings and nothing else")
	require.Len(t, candidates, 1)
	assert.Equal(t, "The error is dropped.", candidates[0].Body,
		"the group's key is §1.4's normal form, with no trailing terminator")
	assert.Equal(t, []string{"f1", "f2", "f3"}, recordsOf(candidates[0]),
		"§2.6.3.2 reports the comments that formed the group, in scan order")
}

// recordsOf is a candidate's comments by record id.
func recordsOf(candidate Candidate) []string {
	records := make([]string, 0, len(candidate.Comments))
	for _, comment := range candidate.Comments {
		records = append(records, comment.Record)
	}
	return records
}

// §2.6.3.2's threshold is reached, not exceeded: a group of exactly
// `rules.harvest_min` is a candidate and one short of it is not.
//
// The boundary is asserted from both sides in one run, because an off-by-one
// here is invisible from either side alone — a `>` would report nothing at all
// on the default of 3 only when no group ever reached 4, and a `>=` on the
// wrong number would report every pair.
func TestAGroupReachingTheThresholdIsACandidateAndOneShortIsNot(t *testing.T) {
	comments := []Comment{
		posted(7, 1, "f1", "reached", "a"),
		posted(7, 2, "f2", "reached", "a"),
		posted(7, 3, "f3", "reached", "a"),
		posted(8, 1, "f4", "short", "b"),
		posted(8, 2, "f5", "short", "b"),
	}

	candidates, err := Harvest(comments, 3)
	require.NoError(t, err)

	assert.Equal(t, []string{"reached×3"}, classesOf(candidates))
}

// A repository with nothing posted harvests an empty list rather than a nil
// one, per §12.3: a caller serialising the answer prints `[]`, which is the
// difference between no candidate and a question never asked.
func TestARepositoryWithNothingPostedHarvestsAnEmptyList(t *testing.T) {
	candidates, err := Harvest(nil, 3)
	require.NoError(t, err)

	require.NotNil(t, candidates)
	assert.Empty(t, candidates)
}

// A body §1.4 refuses stops the harvest rather than being skipped.
//
// §1.4 gives invalid UTF-8 exit code 1, and the normal form is the group key:
// a scan that dropped the comment would report an occurrence count that
// silently excluded it, which is the one number §2.6.3.2's threshold is read
// against.
func TestABodyNormalisationRefusesStopsTheHarvest(t *testing.T) {
	_, err := Harvest([]Comment{posted(7, 1, "f1", "dropped-error", "\xff")}, 3)

	require.Error(t, err)
}
