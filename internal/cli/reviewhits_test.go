package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// reviewedHome is detectedHome with the round's unit recorded as `cr brief`
// records it, which `cr review` checks every unit against and `cr rules check`
// does not.
//
// §3.4.1's diff is taken with three lines of context, so the one hunk the
// change forms is lib.go's head lines 1 to 7 rather than the changed lines 3 to
// 7 detectedHome's unit names, and review.Run refuses a unit whose ranges the
// diff does not give. The three hits sit on lines 4 to 6 either way.
func reviewedHome(t *testing.T) state.Layout {
	t.Helper()
	layout := detectedHome(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":1,"end":7}],`+
			`"head":"`+meta.Head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// runReview runs `cr review` against the fixture's pull request and returns the
// JSON document it printed.
func runReview(t *testing.T) []byte {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"review", fixturePR, "--repo", fixtureSlug})
	require.NoError(t, cmd.Execute())
	return out.Bytes()
}

// §2.6.1.6 through `cr review`: the hits it attaches to its prompts reach the
// repository's ledger with the rule id, the matched path and line, the round
// and the head, and a second run at the same head leaves one entry per hit.
//
// `cr rules check` is never run here, so every entry the ledger holds is one
// `cr review` wrote.
func TestReviewRunTwiceAtOneHeadLeavesOneLedgerEntryPerHit(t *testing.T) {
	layout := reviewedHome(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)

	runReview(t)
	runReview(t)

	lines := ledgerLines(t, layout)
	require.Len(t, lines, 3, "three hits, attached twice, are three entries")
	for at, entry := range lines {
		assert.Equal(t, "no-panic", entry["rule"])
		assert.Equal(t, "lib.go", entry["path"])
		assert.InDelta(t, float64(4+at), entry["line"], 0)
		assert.Equal(t, "hit", entry["event"])
		assert.InDelta(t, float64(fixturePRNumber), entry["pr"], 0)
		assert.InDelta(t, float64(meta.Round), entry["round"], 0)
		assert.Equal(t, meta.Head, entry["head"])
	}
}

// §6.2.5 across the two commands that meet at the ledger: a record citing a
// line only `cr review` hit is stamped `origin: rule`, because `cr record`
// matches the citation positionally against the entry `cr review` wrote.
//
// Without that entry the same record is the forgery
// TestACitationOfARealRuleAtALineItNeverHitIsOfAgentOrigin describes: a real
// rule id at a line the ledger holds no hit for, stamped `origin: agent` and
// forced to a question.
func TestACitationOfALineOnlyReviewHitIsOfRuleOrigin(t *testing.T) {
	layout := reviewedHome(t)
	runReview(t)

	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", confirming("f1", 4)), "--repo", fixtureSlug)
	require.NoError(t, err)

	stored := storedFindings(t, layout)
	require.Len(t, stored, 1)
	require.Len(t, stored[0].Citations, 1)
	assert.Equal(t, finding.OriginRule, stored[0].Citations[0].Origin, "cr review hit no-panic at line 4")
	assert.Equal(t, finding.GradeCited, stored[0].Grade)
	assert.Equal(t, finding.KindFinding, stored[0].Kind)
}
