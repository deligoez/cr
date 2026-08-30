package brief

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// The pull request every test here orients on.
const (
	testOwner = "acme"
	testRepo  = "api"
	testPR    = 7
	testIssue = "CR-7"
)

// runGit runs one command inside dir with a pinned environment, for the reason
// internal/git pins its own: a machine whose owner configures git is a machine
// where an unpinned read answers differently.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"TMPDIR=" + os.TempDir(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
		"LANG=C",
		"GIT_AUTHOR_NAME=cr test",
		"GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=cr test",
		"GIT_COMMITTER_EMAIL=test@example.invalid",
	}
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// repository builds the repository under review: a base commit on `main` and a
// change on top of it, so §3.4.1 has a merge base and a diff to take against
// it. It returns the directory, the head commit, and the base commit.
func repository(t *testing.T) (dir, head, base string) {
	t.Helper()
	dir = t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}

	runGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("order.go", "package shop\n\nfunc Total() int { return 0 }\n")
	runGit(t, dir, "add", "order.go")
	runGit(t, dir, "commit", "--quiet", "-m", "the commit under review")
	base = runGit(t, dir, "rev-parse", "HEAD")

	runGit(t, dir, "checkout", "--quiet", "-b", "feature/"+testIssue+"-total")
	write("order.go", "package shop\n\nfunc Total() int { return subtotal() + shipping() }\n")
	runGit(t, dir, "add", "order.go")
	runGit(t, dir, "commit", "--quiet", "-m", "sum the order")
	head = runGit(t, dir, "rev-parse", "HEAD")

	return dir, head, base
}

// answering is a gh.Runner replying to §3.7's two reads out of canned payloads,
// so the whole of internal/gh runs — the §2.1.2 boundary included — with no
// network, no token, and no pull request that has to exist.
func answering(head, base, threads string) gh.Runner {
	return func(args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "reviewThreads") {
			return threads, nil
		}
		return `{"data":{"repository":{"pullRequest":{"number":7,` +
			`"title":"sum the order","body":"",` +
			`"headRefName":"feature/` + testIssue + `-total",` +
			`"headRefOid":"` + head + `","baseRefName":"main",` +
			`"baseRefOid":"` + base + `"}}}}`, nil
	}
}

// oneThread is a page holding one human thread on the changed file.
const oneThread = `{"data":{"repository":{"pullRequest":{"reviewThreads":{` +
	`"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{` +
	`"id":"PRRT_1","isResolved":false,"isOutdated":false,"path":"order.go",` +
	`"line":3,"startLine":3,"originalLine":3,"originalStartLine":3,"diffSide":"RIGHT",` +
	`"comments":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{` +
	`"id":"PRRC_1","url":"https://example.invalid/1","body":"Is the shipping arm covered?",` +
	`"createdAt":"2026-08-30T09:00:00Z","author":{"__typename":"User","login":"reviewer"}}]}}]}}}}}`

// sources assembles one run's inputs against a fresh state root.
func sources(t *testing.T, dir string, runner gh.Runner) *Sources {
	t.Helper()
	layout := state.New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, layout.Init())

	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue,
		[]byte("The order total sums the subtotal and the shipping.\n"), 0o600))

	resolved, err := config.Resolve(config.Sources{})
	require.NoError(t, err)

	return &Sources{
		Layout:    layout,
		GH:        gh.WithRunner(runner),
		Config:    resolved,
		Owner:     testOwner,
		Repo:      testRepo,
		PR:        testPR,
		RepoDir:   dir,
		IssueFlag: testIssue,
		Intent:    intent.Source{File: issue},
	}
}

// The first brief opens round 1 at the current head and records the derived
// inputs the rest of cr reads as authoritative.
//
// This is brief-creates-state's first criterion, and it is asserted off disk
// rather than off the returned payload. §4.1.6 and §4.5.6 reject a mapping or a
// cell naming an unknown unit id, §9.3.1 and §9.3.3 read meta.json's recorded
// head, and §3.5.3 attaches the ingested threads — none of them can see a value
// that was computed and not written, so a test satisfied by the payload would
// pass on a brief that persisted nothing.
func TestAFirstBriefOpensRoundOneAndRecordsTheDerivedInputs(t *testing.T) {
	dir, head, base := repository(t)
	src := sources(t, dir, answering(head, base, oneThread))

	assembled, err := Run(src)
	require.NoError(t, err)

	recorded, err := src.Layout.Briefed(testOwner, testRepo, testPR)
	require.NoError(t, err, "§9.3.1 and §9.3.3 read meta.json, so a brief has to write one")
	assert.Equal(t, 1, recorded.Round, "§9.3.3 numbers rounds from 1")
	assert.Equal(t, head, recorded.Head, "the recorded head is the current head")
	assert.Equal(t, testIssue, recorded.IssueKey)
	assert.Equal(t, testOwner, recorded.Owner)
	assert.Equal(t, testPR, recorded.PR)

	stored, err := state.ReadRecords[unit.Record](
		src.Layout, testOwner, testRepo, testPR, state.FileUnits)
	require.NoError(t, err)
	require.Len(t, stored, 1, "the change touches one file in one place")
	assert.Equal(t, "u1", stored[0].ID)
	assert.Equal(t, "order.go", stored[0].Path)
	assert.NotEmpty(t, stored[0].Hash, "§3.4.6 records the unit hash")
	assert.Equal(t, head, stored[0].Head, "§2.3.3 stamps head onto every unit")
	assert.Equal(t, 1, stored[0].Round, "§2.3.3 stamps round onto every unit")

	// Every file of the §2.3 table exists, not only the three this command
	// fills: an absent one reads as a failure to the command that opens it
	// next, and §3.7 is where the state directory comes into being.
	for _, name := range state.PRFiles() {
		assert.FileExists(t, src.Layout.PRFile(testOwner, testRepo, testPR, name))
	}

	ingested, err := gh.ReadThreads(src.Layout, testOwner, testRepo, testPR)
	require.NoError(t, err)
	require.Len(t, ingested, 1, "§3.5.1 ingests every existing thread")
	assert.Equal(t, "PRRT_1", ingested[0].ID)
	assert.Equal(t, gh.AuthorHuman, ingested[0].AuthorType)

	// The payload and the files agree, so neither is a second answer.
	assert.Equal(t, head, assembled.Head)
	assert.Equal(t, base, assembled.MergeBase, "the two branches share the base commit")
	assert.Equal(t, 1, assembled.Round)
	assert.Len(t, assembled.Units, 1)
	assert.Len(t, assembled.Threads, 1)
}
