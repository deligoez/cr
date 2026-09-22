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
	var report struct {
		Migrated []struct {
			Record string `json:"record"`
			From   string `json:"from"`
			To     string `json:"to"`
			Placed bool   `json:"placed"`
		} `json:"migrated"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	require.Len(t, report.Migrated, 1)
	assert.Equal(t, "lib.go:4", report.Migrated[0].From)
	assert.Equal(t, "lib.go:7", report.Migrated[0].To, "§9.4.2: the anchored line is where the push moved it")
	assert.True(t, report.Migrated[0].Placed)
}
