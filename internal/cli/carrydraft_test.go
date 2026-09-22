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

// §9.3.4, §9.4.5 and §7.1.7 through the commands, across a real push: a queued
// record whose body the reviewer edited survives the author's push that moves
// the code three lines down. A real `cr brief` opens round 2 and carries the
// record to the moved line; a real `cr draft` then renders it there with the
// reviewer's body, not a fresh one.
//
// Before v0.7.0 the same push staled the record, and the reviewer's edit was
// lost with it: the case the whole of reading (i) was chosen for.
func TestAnEditedDraftSurvivesThePushThatMovesItsCode(t *testing.T) {
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
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":2,"end":5}],`+
			`"head":"`+recorded+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())

	hash, err := finding.AnchorContentHash([]string{"\treturn 499"})
	require.NoError(t, err)
	holdRecords(t, layout, fixtureOwner, fixtureProject, fixturePRNumber,
		`{"id":"f1","kind":"question","role":"correctness","axis":"correctness","unit":"u1",`+
			`"class":"flat-fee","severity":"low","grade":"argued",`+
			`"anchor":{"path":"lib.go","side":"RIGHT","start_line":4,"line":4,"content_hash":"`+hash+`",`+
			`"context_before":["package lib","","func Fee() int {"],"context_after":["}"]},`+
			`"summary":"Is the fee meant to be flat?","evidence":"It is a literal.",`+
			`"state":"queued","head":"`+recorded+`","round":1}`)

	edited := "Should the fee follow the weight of the order? A flat 499 looks deliberate, but I could not tell."
	marker := `<!-- cr:record id="f1" kind="question" path="lib.go" side="RIGHT" start_line="4" line="4" ` +
		`severity="low" grade="argued" disposition="" -->`
	round1 := layout.RoundDir(fixtureOwner, fixtureProject, fixturePRNumber, 1)
	require.NoError(t, os.MkdirAll(round1, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(round1, state.FileDraft), []byte(marker+"\n\n"+edited+"\n"), 0o600))
	rendered, err := json.Marshal(map[string]string{"f1": "Is the fee meant to be flat?\n\nIt is a literal."})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(round1, state.FileRendered), rendered, 0o600))

	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte("Charge a shipping fee.\n"), 0o600))
	printed, err := runCLIPrinting(t, "brief", fixturePR, "--issue", fixtureIssue, "--intent-file", issue,
		"--repo", fixtureSlug)
	require.NoError(t, err)
	var briefed struct {
		Round   int      `json:"round"`
		Carried []string `json:"carried_records"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	require.Equal(t, 2, briefed.Round)
	require.Equal(t, []string{"f1"}, briefed.Carried, "§9.4.5: the code moved, so the record follows it")

	_, err = runCLIPrinting(t, "draft", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	drafted, err := os.ReadFile(layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileDraft))
	require.NoError(t, err)
	assert.Contains(t, string(drafted), edited, "§7.1.7: the reviewer's body is carried with the record")
	assert.Contains(t, string(drafted), `line="7"`, "§9.4.5: rendered at the line the push moved the code to")
}
