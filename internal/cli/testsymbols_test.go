package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// libProfile indexes Go and names its test files, so both halves of §4.4.1 can
// run: the test files through tests.globs, and the symbols they reference
// through the head symbol index symbols.lang builds.
const libProfile = `{"id":"lib","match":{"files":[],"globs":["**/*.go"]},` +
	`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},` +
	`"tests":{"cmd":["go","test"],"globs":["**/*_test.go"]},"symbols":{"lang":"go"}}`

// testSymbolsHome is a round whose head adds lib_test.go calling Load, which
// lib.go already declares, under libProfile.
//
// With deleted set, the base also holds legacy_test.go and the head removes it,
// so §4.4.1's file half attaches a test file the head no longer holds and the
// symbol half has nothing at the head to read for it.
func testSymbolsHome(t *testing.T, deleted bool) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("lib.go", "package lib\n\nfunc Load() {}\n")
	if deleted {
		write("legacy_test.go", "package lib\n\nfunc TestLegacy() {}\n")
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("lib_test.go", "package lib\n\nfunc TestLoad() {\n\tLoad()\n}\n")
	if deleted {
		mustGit(t, dir, "rm", "--quiet", "legacy_test.go")
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "--quiet", "-m", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsureProfile("lib", libProfile))
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, ProfileID: "lib", Round: 1, Head: head,
		ActiveRoles:  []string{"convention", "correctness", "intent-coverage", "test-adequacy"},
		MappingRound: 1, MappingHead: head,
	}))
	// The range is the head-side range the round's own diff gives for the
	// added test file, `@@ -0,0 +1,5 @@`, because `cr review` rebuilds every
	// recorded unit from the diff.
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib_test.go","side":"RIGHT","hunk_ranges":[{"start":1,"end":5}],`+
			`"hash":"h1","oversized":false,"head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
}

// §4.4.1 through `cr review`: a test file the pull request adds is attached
// together with the head symbols it references, and the symbol half discloses
// nothing because it ran.
//
// lib_test.go names two symbols the head declares, TestLoad and Load. TestLoad
// is the file declaring its own test on that line, which is not a reference;
// Load is the helper it calls. So the attachment is exactly Load, and a run
// handing testadequacy.Attach no References would print "none" beside an
// unavailability line instead.
func TestReviewAttachesTheHeadSymbolsATestFileReferences(t *testing.T) {
	testSymbolsHome(t, false)

	printed, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	var fanout struct {
		Prompts []struct {
			Role string `json:"role"`
			Unit string `json:"unit"`
			Text string `json:"prompt"`
		} `json:"prompts"`
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &fanout))
	require.NotEmpty(t, fanout.Prompts)
	for _, prompt := range fanout.Prompts {
		lines := strings.Split(prompt.Text, "\n")
		assert.Contains(t, lines, "Changed or added by the pull request: lib_test.go.",
			"%s on %s: §4.4.1's file half", prompt.Role, prompt.Unit)
		assert.Contains(t, lines, "Symbols they reference: Load.",
			"%s on %s: §4.4.1's symbol half", prompt.Role, prompt.Unit)
	}
	for _, line := range fanout.Honesty {
		assert.NotContains(t, line, "test/symbols", "the symbol half ran over the head's index")
	}
}
