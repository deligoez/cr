package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// §9.4.2: a migration reads the current head's tree, and finds an anchor where
// the push moved it.
//
// v0.6.0 read the round's recorded head instead — the commit the record was
// stamped against — so every anchor was found exactly where it had been and
// nothing ever moved. Measured on deligoez/cr-qa#24: three lines inserted
// above a queued record reported `33 → 33`. Here the push inserts three lines
// above the anchored one, and the migration has to report the line it now
// holds.
func TestAMigrationReadsTheHeadThePushMovedTo(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.go"), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("package lib\n")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("package lib\n\nfunc Fee() int {\n\treturn 499\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change the round was opened on")
	recorded := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	write("package lib\n\n// Fee is the flat\n// shipping fee,\n// in cents.\nfunc Fee() int {\n\treturn 499\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the push that moves the anchored line")
	moved := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), moved, base)+string(os.PathListSeparator)+os.Getenv("PATH"))
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	movedHead(t, moved)
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 1, Head: recorded,
	}))
	require.NoError(t, held.Unlock())

	hash, err := finding.AnchorContentHash([]string{"\treturn 499"})
	require.NoError(t, err)
	holdRecords(t, layout, fixtureOwner, fixtureProject, fixturePRNumber,
		`{"id":"f1","kind":"question","role":"correctness","unit":"u1","class":"flat-fee",`+
			`"anchor":{"path":"lib.go","side":"RIGHT","start_line":4,"line":4,"content_hash":"`+hash+`",`+
			`"context_before":["package lib","","func Fee() int {"],"context_after":["}"]},`+
			`"summary":"is the fee meant to be flat?","state":"queued","head":"`+recorded+`","round":1}`)

	printed, err := runCLIPrinting(t, "recheck", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	// §9.5.7: the head has moved past the round, so this is the preview of
	// what `cr brief` would do, and nothing is written.
	var report struct {
		Preview []struct {
			Record string `json:"record"`
			From   string `json:"from"`
			To     string `json:"to"`
			Placed bool   `json:"placed"`
		} `json:"preview"`
		Migrated []any `json:"migrated"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	require.Len(t, report.Preview, 1)
	assert.Equal(t, "lib.go:4", report.Preview[0].From)
	assert.Equal(t, "lib.go:7", report.Preview[0].To, "§9.4.2: the anchored line is where the push moved it")
	assert.True(t, report.Preview[0].Placed)
	assert.Empty(t, report.Migrated, "no brief has opened the round the move belongs to")
	migrations, err := os.ReadFile(layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileMigrations))
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(migrations)), "§9.5.7: a preview writes nothing")
}

// §9.4.3: after the record's own file, a migration searches every file the
// diff touches, so an anchor whose code the push moved to another file is
// found there. The preview searches the same files `cr brief` would.
func TestAMigrationFollowsCodeThePushMovedToAnotherFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("lib.go", "package lib\n")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("lib.go", "package lib\n\nfunc Fee() int {\n\treturn 499\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change the round was opened on")
	recorded := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	write("lib.go", "package lib\n")
	write("fee.go", "package lib\n\nfunc Fee() int {\n\treturn 499\n}\n")
	mustGit(t, dir, "add", "fee.go")
	mustGit(t, dir, "commit", "--quiet", "-am", "the push that moves the function to its own file")
	moved := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), moved, base)+string(os.PathListSeparator)+os.Getenv("PATH"))
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	// The base is what makes fee.go a file the diff touches: a pull
	// request whose base is not reported has no touched files to search.
	restorePR := currentPullRequest
	currentPullRequest = func(_, _ string, _ int) (gh.PullRequest, error) {
		return gh.PullRequest{Head: moved, Base: base}, nil
	}
	t.Cleanup(func() { currentPullRequest = restorePR })
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 1, Head: recorded,
	}))
	require.NoError(t, held.Unlock())

	hash, err := finding.AnchorContentHash([]string{"\treturn 499"})
	require.NoError(t, err)
	holdRecords(t, layout, fixtureOwner, fixtureProject, fixturePRNumber,
		`{"id":"f1","kind":"question","role":"correctness","unit":"u1","class":"flat-fee",`+
			`"anchor":{"path":"lib.go","side":"RIGHT","start_line":4,"line":4,"content_hash":"`+hash+`",`+
			`"context_before":["package lib","","func Fee() int {"],"context_after":["}"]},`+
			`"summary":"is the fee meant to be flat?","state":"queued","head":"`+recorded+`","round":1}`)

	printed, err := runCLIPrinting(t, "recheck", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report struct {
		Preview []struct {
			From   string `json:"from"`
			To     string `json:"to"`
			Placed bool   `json:"placed"`
		} `json:"preview"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	require.Len(t, report.Preview, 1)
	assert.Equal(t, "lib.go:4", report.Preview[0].From)
	assert.Equal(t, "fee.go:4", report.Preview[0].To, "§9.4.3: the anchored line is in the file the push moved it to")
	assert.True(t, report.Preview[0].Placed)
}
