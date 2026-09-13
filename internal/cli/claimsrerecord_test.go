package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// rerecordIssue is the issue text the re-record fixture's claims are drawn
// from: two sentences, so one claim can be mapped and the other left as a gap.
const rerecordIssue = fixtureIssue + ": load parses the file.\nRetries back off.\n"

// rerecordHome is a pull request whose head changes one source file, briefed
// through `cr brief` so the round and its one unit are the ones the command
// forms. It returns the layout, the issue file and the unit's id.
func rerecordHome(t *testing.T) (layout state.Layout, issue, unitID string) {
	t.Helper()
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.go"), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("package lib\n\nfunc Load() {}\n")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("package lib\n\nfunc Load() {\n\tparse()\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout = state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	issue = filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(rerecordIssue), 0o600))

	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	var briefed struct {
		Units []unit.Unit `json:"units"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	require.Len(t, briefed.Units, 1, "the fixture is only worth anything if its diff yields one unit")
	return layout, issue, briefed.Units[0].ID
}

// rerecordFile writes lines as an agent's NDJSON input and returns its path.
func rerecordFile(t *testing.T, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return path
}

// intentReport reads `cr status`'s §10.1.2 intent coverage.
func intentReport(t *testing.T) intentCoverage {
	t.Helper()
	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report struct {
		Intent intentCoverage `json:"intent"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report.Intent
}

// Audit round 5's list-25-5 through the commands: brief, claims record, map
// record, claims record again. The second `cr claims record` clears the
// mapping, so it also takes meta.json's mapping stamp off and the round's gap
// entries with it, and `cr review` refuses the remaining axes again with exit 4.
//
// The mapping leaves one claim unmapped, so the gap file holds an entry before
// the re-record: a fixture whose every claim was mapped would leave the gap
// file empty either way, and a command that never cleared it would pass.
func TestReRecordingClaimsClearsTheMappingStampAndTheGaps(t *testing.T) {
	layout, issue, u := rerecordHome(t)
	claims := rerecordFile(t, "claims.ndjson",
		`{"id":"`+fixtureIssue+`#c1","text":"Load parses.","source":"description","span":"load parses the file"}`,
		`{"id":"`+fixtureIssue+`#c2","text":"Retries back off.","source":"description","span":"Retries back off"}`,
	)
	_, err := runCLIPrinting(t, "claims", "record", fixturePR, claims,
		"--repo", fixtureSlug, "--intent-file", issue)
	require.NoError(t, err)
	pairs := rerecordFile(t, "mapping.ndjson", `{"claim":"`+fixtureIssue+`#c1","unit":"`+u+`"}`)
	_, err = runCLIPrinting(t, "map", "record", fixturePR, pairs, "--repo", fixtureSlug)
	require.NoError(t, err)

	mapped, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.True(t, mapped.MappingRecorded(), "the control: map record stamped the round")
	require.Equal(t, intentCoverage{Claims: 2, Mapped: 1, Gaps: []mapping.Gap{{
		Claim: fixtureIssue + "#c2", Stamp: state.Stamp{Head: mapped.Head, Round: mapped.Round},
	}}}, intentReport(t), "the control: c2 is the round's gap before the re-record")
	_, err = runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err, "the control: with the mapping recorded, cr review emits every axis")

	_, err = runCLIPrinting(t, "claims", "record", fixturePR, claims,
		"--repo", fixtureSlug, "--intent-file", issue)
	require.NoError(t, err)

	cleared, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	assert.Equal(t, 0, cleared.MappingRound, "§3.3.1 clears the mapping, and its stamp with it")
	assert.Empty(t, cleared.MappingHead, "and the stamp's head")
	assert.Equal(t, mapped.Round, cleared.Round, "the round itself is untouched")
	assert.Equal(t, mapped.Head, cleared.Head, "and so is its head")
	gaps, err := layout.ReadPR(fixtureOwner, fixtureProject, fixturePRNumber, state.FileIntentGaps)
	require.NoError(t, err)
	assert.Empty(t, string(gaps), "§4.1.7's gaps were derived from the mapping the re-record cleared")
	assert.Equal(t, intentCoverage{Claims: 2, Mapped: 0, Gaps: []mapping.Gap{}}, intentReport(t),
		"cr status reports the replaced claims and no mapped claim")

	_, err = runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	var required *review.MappingRequiredError
	require.ErrorAs(t, err, &required, "§4.6.5: no mapping exists for the round until cr map record runs again")
	assert.Equal(t, ExitState, exitCodeFor(err))
}

// Audit round 5's table-6-0 through the command: two claims holding one id are
// refused with exit 1 naming the file, the second line and the id, and the
// round's claims are left as they were.
func TestClaimsRecordRefusesARepeatedClaimID(t *testing.T) {
	layout := claimedHome(t)
	before, err := layout.ReadPR(claimsOwner, claimsRepo, claimsPRNum, state.FileClaims)
	require.NoError(t, err)
	file := aClaimFile(t,
		`{"id":"`+claimsIssue+`#c1","text":"Back off.","source":"acceptance","span":"backs off exponentially"}`,
		`{"id":"`+claimsIssue+`#c1","text":"Give up.","source":"acceptance","span":"abandoned after five attempts"}`,
	)

	err = runClaimsRecord(t, claimsPR, file, "--repo", claimsSlug, "--intent-file", anIssueFile(t))

	var rejected *intent.RejectedClaimError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, intent.RejectedClaimError{
		File: file, Line: 2, Field: "id",
		Problem: `"` + claimsIssue + `#c1" repeats the id of line 1; a mapping names a claim by its id alone, ` +
			"so give each claim an id of its own",
	}, *rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	after, err := layout.ReadPR(claimsOwner, claimsRepo, claimsPRNum, state.FileClaims)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a refused file replaces nothing")
}
