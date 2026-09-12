package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// statusHome is a round with one of everything §10.1's first three items have
// to report.
//
// The round holds two units: `u1`, with a cell from every active role, and
// `u2`, which §3.4.5 flagged `oversized` and which no role reported on — so the
// same fixture carries a complete row, a gap, and the flag. It holds three
// claims: one the mapping maps to `u1`, and two §4.1.3 raised entries for, one
// of which §4.1.8 set aside. And it resolves the shipped `generic` profile,
// which declares no `tests.cmd` and no `symbols.lang`, so §4.5.2 disables the
// test axis, §4.6.4 skips the role on it, and both lens halves of §4.3.1 and
// §4.4.1 report themselves unavailable.
//
// The checkout is real, because §10.1.3's lens halves are computed the way
// `cr review` computes them: a symbol index over the round's head and the
// round's own diff. A fixture that faked either would leave the report
// measuring nothing about the round.
func statusHome(t *testing.T) {
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
	write("package lib\n\nfunc Load() {\n\tparse()\n\tstore()\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	writeStatusRound(t, layout, head)
}

// writeStatusRound publishes the round's §2.3 files.
func writeStatusRound(t *testing.T, layout state.Layout, head string) {
	t.Helper()
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "generic", Round: 1, Head: head,
		// The three roles whose axes `generic` leaves active. The test
		// axis is not one: §4.5.2 disables it for a profile declaring
		// no tests.cmd, whatever `axes.test` says.
		ActiveRoles: []string{"convention", "correctness", "intent-coverage"},
	}))
	// The range is the head-side range the round's own diff gives, context
	// included: `@@ -1,3 +1,6 @@` over the two commits statusHome makes.
	// `cr status` counts units and never rebuilds them, so it reads any
	// range at all; `cr review` matches every recorded range against the
	// diff and refuses a unit it cannot rebuild, so a fixture that invents
	// one is a round only half the commands can run.
	require.NoError(t, held.Write(state.FileUnits,
		[]byte(`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":1,"end":6}],`+
			`"hash":"h1","oversized":false,"head":"`+head+`","round":1}`+"\n"+
			`{"id":"u2","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":1,"end":6}],`+
			`"hash":"h2","oversized":true,"head":"`+head+`","round":1}`+"\n")))
	cells := ""
	for _, role := range []string{"convention", "correctness", "intent-coverage"} {
		cells += `{"unit":"u1","role":"` + role + `","result":"pass","unit_hash":"h1",` +
			`"head":"` + head + `","round":1}` + "\n"
	}
	require.NoError(t, held.Write(state.FileCoverage, []byte(cells)))
	require.NoError(t, held.Write(state.FileClaims,
		[]byte(`{"id":"`+fixtureIssue+`#c1","text":"Load parses.","source":"acceptance",`+
			`"span":"parses","head":"`+head+`","round":1}`+"\n"+
			`{"id":"`+fixtureIssue+`#c2","text":"Load retries.","source":"acceptance",`+
			`"span":"retries","head":"`+head+`","round":1}`+"\n"+
			`{"id":"`+fixtureIssue+`#c3","text":"Load logs.","source":"acceptance",`+
			`"span":"logs","head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Write(state.FileMapping,
		[]byte(`{"claim":"`+fixtureIssue+`#c1","unit":"u1","head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Write(state.FileIntentGaps,
		[]byte(`{"claim":"`+fixtureIssue+`#c2","set_aside_note":"`+fixtureIssue+`#n1",`+
			`"head":"`+head+`","round":1}`+"\n"+
			`{"claim":"`+fixtureIssue+`#c3","set_aside_note":"","head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
}

// §10.1.1 through §10.1.3, over a round holding an oversized unit, a coverage
// gap, an unimplemented claim, a set-aside claim, and a skipped role.
//
// The three sections are asserted from one run rather than from three, because
// §10.1 is one report: a command that could produce any of them without the
// others would let a reader act on coverage while a lens that never looked went
// unstated, which is exactly what §4.5.4 forbids.
func TestStatusReportsCoverageIntentAndTheLensesThatDidNotRun(t *testing.T) {
	statusHome(t)

	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	var report struct {
		Round    int           `json:"round"`
		Coverage coverage.Rows `json:"coverage"`
		Intent   struct {
			Claims   int           `json:"claims"`
			Mapped   int           `json:"mapped"`
			Gaps     []mapping.Gap `json:"gaps"`
			SetAside int           `json:"set_aside"`
		} `json:"intent"`
		Axes    activation.Activation  `json:"axes"`
		Skipped []coverage.SkippedRole `json:"skipped_roles"`
		Honesty []string               `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))

	// §10.1.1.
	assert.Equal(t, 1, report.Round)
	assert.Equal(t, coverage.Rows{Units: 2, Complete: 1, Gaps: 1, Oversized: 1, Roles: 3},
		report.Coverage,
		"§10.1.1: units total, complete rows, gaps, and §3.4.5's flag")

	// §10.1.2. The entries are listed and not only counted, because §4.1.3
	// keeps them out of findings.ndjson and out of every draft — this is
	// the only place a reviewer can learn which claims they stand for.
	assert.Equal(t, 3, report.Intent.Claims)
	assert.Equal(t, 1, report.Intent.Mapped)
	require.Len(t, report.Intent.Gaps, 2, "§4.1.3 raised an entry for each unmapped claim")
	assert.Equal(t, fixtureIssue+"#c2", report.Intent.Gaps[0].Claim)
	assert.Equal(t, fixtureIssue+"#n1", report.Intent.Gaps[0].SetAsideNote)
	assert.Equal(t, fixtureIssue+"#c3", report.Intent.Gaps[1].Claim)
	assert.Equal(t, 1, report.Intent.SetAside,
		"§4.1.7's carry-forward is reported from the surviving side, since "+
			"`cr map record` reports only the stamps it dropped")

	// §10.1.3, first clause.
	assert.Equal(t, []string{axis.Intent, axis.Correctness, axis.Convention}, report.Axes.Active,
		"`generic` enables all four axes and §4.5.2 takes the test axis back out")
	require.Len(t, report.Axes.Disabled, 1)
	assert.Equal(t, axis.Test, report.Axes.Disabled[0].Axis)
	assert.Equal(t, activation.RuleNoTestCommand, report.Axes.Disabled[0].Rule)
	assert.Empty(t, report.Axes.Unavailable, "the round resolved an issue key, so §4.5.3 does not apply")

	// §10.1.3, second clause: §4.6.4's skipped role, with its reason.
	require.Len(t, report.Skipped, 1)
	assert.Equal(t, "test-adequacy", report.Skipped[0].Role)
	assert.Contains(t, report.Skipped[0].Reason, "tests.cmd",
		"§4.5.4's reason names what would make the lens run")

	// §4.5.4's four kinds reach the reader through the one channel §11.1
	// exempts from `--quiet`, beside §9.3.1's comparison.
	honesty := strings.Join(report.Honesty, "\n")
	for _, expected := range []string{
		"§9.3.1: round 1 was opened at head",
		"axis test disabled, per §4.5.2",
		"lens convention/reinvention unavailable, per §4.3.1",
		"lens test/symbols unavailable, per §4.5.4",
		"role test-adequacy skipped, per §4.6.4",
	} {
		assert.Contains(t, honesty, expected)
	}
}

// The terminal rendering carries the same three sections as the document, so a
// reader of one and a reader of the other are told the same thing.
//
// §12.1 gives the two shapes one payload, and the honesty channel is what makes
// this worth asserting separately: §11.1 exempts those lines from `--quiet`,
// and a rendering that printed the counts and dropped them would satisfy every
// count above while leaving a lens that never looked unstated.
func TestTheStatusTextCarriesTheCountsAndEveryLensThatDidNotRun(t *testing.T) {
	statusHome(t)

	// `--no-color` because the round number is accented in a terminal, and
	// what is being asserted is the sentence rather than the escape codes
	// around it. §12.1 leaves the shape alone either way.
	printed := throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color")

	for _, expected := range []string{
		"round 1 at ",
		"units: 2 total, 1 with a complete row of 3 active role(s), 1 with gaps, 1 oversized",
		"claims: 3 total, 1 mapped to a unit, 2 unimplemented (1 set aside)",
		fixtureIssue + "#c2 (set aside by " + fixtureIssue + "#n1)",
		"axes active: intent, correctness, convention",
		"axis test disabled, per §4.5.2",
		"lens convention/reinvention unavailable, per §4.3.1",
		"lens test/symbols unavailable, per §4.5.4",
		"role test-adequacy skipped, per §4.6.4",
	} {
		assert.Contains(t, printed, expected)
	}
	assert.NotContains(t, printed, fixtureIssue+"#c3 (set aside",
		"§4.1.3 writes `set_aside_note` on every entry, so an empty one is a claim nobody judged")
}
