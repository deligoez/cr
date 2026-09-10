package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// §6.2.5 through the commands: a citation naming a real rule id at a line that
// rule never hit is stamped `origin: agent`, and the same rule's citation of a
// line it did hit is stamped `origin: rule`.
//
// Both records name no-panic and both cite a line of the head inside their own
// unit, so the rule id and the unit cannot be what separates them — only the
// ledger's hit at line 4, and its absence at line 3. That is the forgery the
// positional match closes: were the rule id enough, line 3 would buy §6.2's
// `cited` row as surely as line 4, and a record could be graded as though a
// detector had found what it only asserted. So the grades are read too — the
// hit's record keeps its finding register, and the other is forced to a
// question by §6.3.
func TestACitationOfARealRuleAtALineItNeverHitIsOfAgentOrigin(t *testing.T) {
	layout := detectedHome(t)
	runRulesCheck(t)

	_, err := runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson",
		confirming("f1", 4), confirming("f2", 3)), "--repo", fixtureSlug)
	require.NoError(t, err)

	stored := storedFindings(t, layout)
	require.Len(t, stored, 2)
	hit, never := stored[0], stored[1]
	require.Equal(t, []string{"f1", "f2"}, []string{hit.ID, never.ID})

	require.Len(t, hit.Citations, 1)
	assert.Equal(t, finding.OriginRule, hit.Citations[0].Origin, "no-panic hit line 4")
	assert.Equal(t, finding.GradeCited, hit.Grade)
	assert.Equal(t, finding.KindFinding, hit.Kind)

	require.Len(t, never.Citations, 1)
	assert.Equal(t, finding.OriginAgent, never.Citations[0].Origin,
		"no-panic is a real rule, and it never hit line 3")
	assert.Equal(t, finding.GradeArgued, never.Grade)
	assert.Equal(t, finding.KindQuestion, never.Kind, "§6.3 forces what the citation could not buy")
}
