package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/text"
)

// The pull request `cr claims record` is exercised against, and the round it
// stands in. The round is past its first for the reason `cr record`'s fixture
// is: §9.3.5 scopes a replacement to the current round, and a fixture in round
// 1 could not tell a command that replaced one round from one that replaced the
// file.
const (
	claimsOwner = "acme"
	claimsRepo  = "api"
	claimsSlug  = claimsOwner + "/" + claimsRepo
	claimsPR    = "21"
	claimsPRNum = 21
	claimsRound = 2
	claimsIssue = "CR-21"
	claimsHead  = "5e6d7c8b9a807162534455667788990a1b2c3d4e"
)

// issueText is the tracker text the fixture's claims are drawn from. Every span
// a passing test uses is a substring of it, and §3.3's issue_hash is its
// normalised hash.
const issueText = `Retry the upload on a 5xx response.

Acceptance:
- The retry backs off exponentially.
- The upload is abandoned after five attempts.
`

// claimedHome puts a state root behind CR_HOME holding one pull request in
// round claimsRound, with one claim and one mapping entry already recorded in
// the round before.
//
// The earlier round's lines are written as bytes rather than through a record
// type, and carry only an id and the stamp. §9.3.5 makes them history, so what
// this fixture has to prove is that a line the current command never wrote and
// does not fully understand survives the write — which a fixture built out of
// the current claim struct could not show.
func claimedHome(t *testing.T) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(claimsOwner, claimsRepo, claimsPRNum))

	held, err := layout.LockPR(claimsOwner, claimsRepo, claimsPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: claimsOwner, Repo: claimsRepo, PR: claimsPRNum,
		IssueKey: claimsIssue, Round: claimsRound, Head: claimsHead,
	}))
	require.NoError(t, held.Write(state.FileClaims,
		[]byte(`{"id":"`+claimsIssue+`#c9","head":"1f2e3d4c","round":1}`+"\n")))
	require.NoError(t, held.Write(state.FileMapping,
		[]byte(`{"claim":"`+claimsIssue+`#c9","unit":"u4","head":"1f2e3d4c","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// anIssueFile writes the issue text §3.1.4 reads in place of the tracker
// command, and returns its path.
func anIssueFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// aClaimFile writes lines as the NDJSON file an agent hands `cr claims record`,
// and returns its path. It sits outside the state tree: the file is the agent's
// own output, and cr only ever reads it.
func aClaimFile(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claims.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return path
}

// runClaimsRecord runs `cr claims record` with args against whatever CR_HOME
// points at, and returns what it printed and what it refused.
func runClaimsRecord(t *testing.T, args ...string) (printed string, err error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"claims", "record"}, args...))
	err = cmd.Execute()
	return out.String(), err
}

// §3.3.1 through the command: every claim of an accepted file reaches
// claims.ndjson carrying the two hashes cr computed and the stamp cr wrote.
//
// The four fields asserted on each stored claim are the four the agent could
// not have written. §3.3's table makes `span_hash` and `issue_hash` computed
// and §2.3.3 reserves head and round, and all four are refused on the wire, so
// a claim carrying them on disk got them from cr.
//
// The issue hash is compared against text.NormalisedHash of the whole issue
// text rather than against a literal. §3.3's table names that pre-image
// exactly, and §3.3.3 will compare a fresh hash of the same text against this
// stored value next round: a literal here would still match a command that
// hashed the spans, or the file, or the claims.
func TestClaimsRecordStoresTheHashesCrComputes(t *testing.T) {
	layout := claimedHome(t)
	file := aClaimFile(t,
		`{"id":"`+claimsIssue+`#c1","text":"Back off exponentially.",`+
			`"source":"acceptance","span":"backs off exponentially"}`,
		`{"id":"`+claimsIssue+`#c2","text":"Give up after five attempts.",`+
			`"source":"acceptance","span":"abandoned after five attempts"}`,
	)

	_, err := runClaimsRecord(t, claimsPR, file,
		"--repo", claimsSlug, "--intent-file", anIssueFile(t, issueText))
	require.NoError(t, err)

	stored, err := state.ReadRecords[intent.Claim](
		layout, claimsOwner, claimsRepo, claimsPRNum, state.FileClaims,
	)
	require.NoError(t, err)
	thisRound := make([]intent.Claim, 0, len(stored))
	for _, claim := range stored {
		if claim.Round == claimsRound {
			thisRound = append(thisRound, claim)
		}
	}
	require.Len(t, thisRound, 2, "§3.3.1 stores the claims the file carried")

	wholeIssue, err := text.NormalisedHash(issueText)
	require.NoError(t, err)
	for _, claim := range thisRound {
		span, err := text.NormalisedHash(claim.Span)
		require.NoError(t, err)
		assert.Equal(t, span, claim.SpanHash,
			"§3.3: span_hash is the normalised hash of this claim's own span")
		assert.Equal(t, wholeIssue, claim.IssueHash,
			"§3.3: issue_hash is the normalised hash of the whole issue text")
		assert.Equal(t, claimsHead, claim.Head, "§2.3.3: cr writes head on every write")
		assert.Equal(t, claimsRound, claim.Round, "§2.3.3: and round with it")
	}
}

// §3.3.1 replaces claims.ndjson and clears mapping.ndjson, and §9.3.5 scopes
// both to the current round: the round before this one comes out unchanged.
//
// The command is run twice on purpose. The second run is what tells a replace
// from an append — one extraction of one claim follows another of two, and a
// command that appended would leave three in the round. The earlier round's
// mapping line is read back as bytes, because that is the only assertion a
// writer that decoded and re-encoded it could fail.
func TestClaimsRecordReplacesThisRoundAndLeavesTheOneBefore(t *testing.T) {
	layout := claimedHome(t)
	issue := anIssueFile(t, issueText)
	first := aClaimFile(t,
		`{"id":"`+claimsIssue+`#c1","text":"Back off.","source":"acceptance",`+
			`"span":"backs off exponentially"}`,
		`{"id":"`+claimsIssue+`#c2","text":"Give up.","source":"acceptance",`+
			`"span":"abandoned after five attempts"}`,
	)
	_, err := runClaimsRecord(t, claimsPR, first, "--repo", claimsSlug, "--intent-file", issue)
	require.NoError(t, err)

	second := aClaimFile(t,
		`{"id":"`+claimsIssue+`#c1","text":"Retry a 5xx.","source":"description",`+
			`"span":"Retry the upload on a 5xx response."}`,
	)
	_, err = runClaimsRecord(t, claimsPR, second, "--repo", claimsSlug, "--intent-file", issue)
	require.NoError(t, err)

	stored, err := state.ReadRecords[intent.Claim](
		layout, claimsOwner, claimsRepo, claimsPRNum, state.FileClaims,
	)
	require.NoError(t, err)
	require.Len(t, stored, 2, "§3.3.1 replaces the round's claims rather than adding to them")
	assert.Equal(t, 1, stored[0].Round, "§9.3.5: the round before is left intact")
	assert.Equal(t, claimsIssue+"#c9", stored[0].ID)
	assert.Equal(t, claimsRound, stored[1].Round)
	assert.Equal(t, claimsIssue+"#c1", stored[1].ID)

	mapping, err := layout.ReadPR(claimsOwner, claimsRepo, claimsPRNum, state.FileMapping)
	require.NoError(t, err)
	assert.Equal(t,
		`{"claim":"`+claimsIssue+`#c9","unit":"u4","head":"1f2e3d4c","round":1}`+"\n",
		string(mapping),
		"§3.3.1 clears the mapping, and §9.3.5 scopes the clearing to this round")
}

// §3.3.1's rejection through the command: a refused line takes the whole file
// with it, and neither claims.ndjson nor mapping.ndjson moves.
//
// The fault is put on the second line of three, so a command that wrote as it
// validated would already have stored the line above it and a command that
// carried on would have stored the one below. Neither may reach the file: the
// agent is about to correct the input and hand the whole of it in again.
//
// The mapping is asserted on as well as the claims, and that is the half a
// replace-then-clear can get wrong on its own. §3.3.1 states the two as one
// act, so a run that clears the mapping and then refuses the claims has thrown
// away a mapping for an extraction that never happened.
func TestARefusedClaimLeavesTheRoundExactlyAsItWas(t *testing.T) {
	layout := claimedHome(t)
	claims := layout.PRFile(claimsOwner, claimsRepo, claimsPRNum, state.FileClaims)
	mapping := layout.PRFile(claimsOwner, claimsRepo, claimsPRNum, state.FileMapping)
	claimsBefore, err := os.ReadFile(claims)
	require.NoError(t, err)
	mappingBefore, err := os.ReadFile(mapping)
	require.NoError(t, err)

	file := aClaimFile(t,
		`{"id":"`+claimsIssue+`#c1","text":"Back off.","source":"acceptance",`+
			`"span":"backs off exponentially"}`,
		`{"id":"`+claimsIssue+`#c2","text":"Give up.","source":"acceptance",`+
			`"span":"abandoned after five attempts","span_hash":"0123456789abcdef"}`,
		`{"id":"`+claimsIssue+`#c3","text":"Retry a 5xx.","source":"description",`+
			`"span":"Retry the upload"}`,
	)

	_, err = runClaimsRecord(t, claimsPR, file,
		"--repo", claimsSlug, "--intent-file", anIssueFile(t, issueText))
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§3.3 rejects with exit code 1")
	assert.Contains(t, err.Error(), file, "the refusal names the file")
	assert.Contains(t, err.Error(), "line 2", "and the one-based line the claim sits on")
	assert.Contains(t, err.Error(), "span_hash", "and the field at fault")

	claimsAfter, err := os.ReadFile(claims)
	require.NoError(t, err)
	assert.Equal(t, string(claimsBefore), string(claimsAfter),
		"§3.3.1 refuses the file, so the round's previous extraction stands")
	mappingAfter, err := os.ReadFile(mapping)
	require.NoError(t, err)
	assert.Equal(t, string(mappingBefore), string(mappingAfter),
		"and the mapping it would have cleared stands with it")
}

