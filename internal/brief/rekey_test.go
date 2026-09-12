package brief

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/state"
)

// recordClaim stores one claim of the round under the round's issue key, the
// way `cr claims record` stores one: stamped with the head and the round, and
// with §3.3's `<ISSUE-KEY>#c<n>` id.
func recordClaim(t *testing.T, src *Sources, key, head string, round int) {
	t.Helper()
	held, err := src.Layout.LockPR(testOwner, testRepo, testPR)
	require.NoError(t, err)
	require.NoError(t, state.ReplaceStamped(held, state.FileClaims,
		state.Stamp{Head: head, Round: round}, []*intent.Claim{{
			ID: key + "#c1", Text: "The order total sums the subtotal and the shipping.",
			Source: intent.ClaimFromAcceptance,
			Span:   "The order total sums the subtotal and the shipping.",
		}}))
	require.NoError(t, held.Unlock())
}

// withPattern returns the sources' configuration with `intent.key_pattern`
// replaced, which is the layer §3.2 reads the shape of a key from.
func withPattern(t *testing.T, pattern string) config.Config {
	t.Helper()
	resolved, err := config.Resolve(config.Sources{
		Flags: map[string]any{"intent.key_pattern": pattern},
	})
	require.NoError(t, err)
	return resolved
}

// A brief that would replace a recorded issue key refuses, and leaves meta.json
// and claims.ndjson exactly as it found them.
//
// This is brief-payload's dogfood finding: an `intent.key_pattern` that stopped
// matching rewrote `issue_key` to empty and said nothing, while every claim of
// the round stayed recorded as `CR-7#c1` — an id §3.3 forms out of the key that
// is no longer there. Both files are read byte for byte afterwards, because a
// rewrite that re-derived the same values would pass a test that only compared
// the fields it thought to check.
func TestABriefRefusesToReplaceARecordedIssueKey(t *testing.T) {
	dir, head, base := repository(t)
	src := sources(t, dir, answering(head, base, oneThread))
	assembled, err := Run(src)
	require.NoError(t, err)
	require.Equal(t, testIssue, assembled.Issue.Key)
	recordClaim(t, src, testIssue, head, assembled.Round)

	metaPath := src.Layout.PRFile(testOwner, testRepo, testPR, state.FileMeta)
	claimsPath := src.Layout.PRFile(testOwner, testRepo, testPR, state.FileClaims)
	meta, err := os.ReadFile(metaPath)
	require.NoError(t, err)
	claims, err := os.ReadFile(claimsPath)
	require.NoError(t, err)
	require.Contains(t, string(claims), testIssue+"#c1")

	for name, rekey := range map[string]func(){
		"a key_pattern that no longer matches": func() {
			src.IssueFlag = ""
			src.Config = withPattern(t, `ZZZ-[0-9]+`)
		},
		"a different key named on the command line": func() {
			src.IssueFlag = "OTHER-1"
			src.Config = withPattern(t, `[A-Z]+-[0-9]+`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			rekey()

			_, err := Run(src)

			var refused *KeyRewriteError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, testIssue, refused.Recorded)
			assert.Contains(t, err.Error(), "--issue "+testIssue,
				"§12.4: the refusal names the step that keeps the recorded key")
			assert.Contains(t, err.Error(), testIssue+"#c<n>",
				"§3.3: and says what the rewrite would orphan")

			after, err := os.ReadFile(metaPath)
			require.NoError(t, err)
			assert.Equal(t, string(meta), string(after), "§9.3.2: the refusal wrote nothing")
			afterClaims, err := os.ReadFile(claimsPath)
			require.NoError(t, err)
			assert.Equal(t, string(claims), string(afterClaims),
				"and the claims recorded under the old key are untouched")
		})
	}
}

// The refusal is a rewrite's, not a brief's. A round re-briefed under the key it
// already carries runs, and so does the first brief of a pull request cr holds
// no state for — §9.3.3 opens round 1 there, and a round opened before any key
// resolved has nothing to orphan.
func TestARoundRecordsItsFirstKeyAndReBriefsUnderIt(t *testing.T) {
	dir, head, base := repository(t)
	src := sources(t, dir, answering(head, base, oneThread))
	src.IssueFlag = ""
	src.Config = withPattern(t, `ZZZ-[0-9]+`)

	first, err := Run(src)
	require.NoError(t, err, "§4.5.3 marks the intent axis unavailable; it does not refuse the brief")
	require.Empty(t, first.Issue.Key)

	// The key that resolves next is the round's first, so it is recorded
	// rather than refused.
	src.IssueFlag = testIssue
	src.Config = withPattern(t, `[A-Z]+-[0-9]+`)
	second, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, testIssue, second.Issue.Key)

	// And a brief under the key now recorded is the idempotent one §9.3.3
	// promises.
	third, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, second.Round, third.Round)
}
