package review

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/brief"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// The pull request the fan-out tests run against.
const (
	runOwner = "acme"
	runRepo  = "shop"
	runPR    = 7
	runIssue = "CR-7"
)

// runGit runs one git command in dir with a pinned environment, for the reason
// internal/git pins its own.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.TempDir(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"LC_ALL=C", "LANG=C",
		"GIT_AUTHOR_NAME=cr test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=cr test", "GIT_COMMITTER_EMAIL=test@example.invalid",
	}
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// shop builds the repository under review. The base holds a money formatter;
// the change edits the order total, adds a formatter whose name folds to within
// one letter of the existing one, and adds a test file. It returns the
// directory, the head, and the base.
func shop(t *testing.T) (dir, head, base string) {
	t.Helper()
	dir = t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	runGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("money.go", "package shop\n\nfunc FormatMoney(amount, cents int) string { return \"\" }\n")
	write("order.go", "package shop\n\nfunc Total() int { return 0 }\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "--quiet", "-m", "the code before the change")
	base = runGit(t, dir, "rev-parse", "HEAD")

	runGit(t, dir, "checkout", "--quiet", "-b", "feature/"+runIssue)
	write("order.go", "package shop\n\nfunc Total() int { return subtotal() + shipping() }\n\n"+
		"func formatMoneys(amount, cents int) string { return \"\" }\n")
	write("order_test.go", "package shop\n\nfunc TestTotal() {}\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "--quiet", "-m", "sum the order")
	head = runGit(t, dir, "rev-parse", "HEAD")
	return dir, head, base
}

// shopProfile is a Go profile turning every axis on, naming the test files, and
// shipping one rule whose detector matches the call the change adds.
const shopProfile = `{"id":"shop","match":{"files":[],"globs":["**/*.go"]},` +
	`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},` +
	`"tests":{"cmd":["go","test"],"globs":["**/*_test.go"]},"symbols":{"lang":"go"},` +
	`"rules":[{"id":"no-shipping-call","title":"Shipping is read from the quote",` +
	`"rationale":"The quote already carries the shipping amount.","class":"shipping-recomputed",` +
	`"detect":{"pattern":"shipping\\(\\)","mode":"regex"}}]}`

// oneHumanThread is a page holding one human thread on the edited line.
const oneHumanThread = `{"data":{"repository":{"pullRequest":{"reviewThreads":{` +
	`"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{` +
	`"id":"PRRT_1","isResolved":false,"isOutdated":false,"path":"order.go",` +
	`"line":3,"startLine":3,"originalLine":3,"originalStartLine":3,"diffSide":"RIGHT",` +
	`"comments":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{` +
	`"id":"PRRC_1","url":"https://example.invalid/1","body":"Is the shipping arm covered?",` +
	`"createdAt":"2026-08-30T09:00:00Z","author":{"__typename":"User","login":"reviewer"}}]}}]}}}}}`

// briefed opens round 1 on the shop repository through `cr brief`'s own
// package, records a note, a claim, and a mapping of that claim to u1, and
// returns the fan-out's sources.
func briefed(t *testing.T) *Sources {
	t.Helper()
	dir, head, base := shop(t)
	layout := state.New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsureProfile("shop", shopProfile))
	cfg, err := config.Resolve(config.Sources{Flags: map[string]any{"profile": "shop"}})
	require.NoError(t, err)
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte("The total sums the subtotal and the shipping.\n"), 0o600))

	client := gh.WithRunner(func(args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "reviewThreads") {
			return oneHumanThread, nil
		}
		return `{"data":{"repository":{"pullRequest":{"number":7,"title":"sum the order","body":"",` +
			`"headRefName":"feature/` + runIssue + `","headRefOid":"` + head + `",` +
			`"baseRefName":"main","baseRefOid":"` + base + `"}}}}`, nil
	})
	_, err = brief.Run(&brief.Sources{
		Layout: layout, GH: client, Config: cfg, Owner: runOwner, Repo: runRepo, PR: runPR,
		RepoDir: dir, IssueFlag: runIssue, Intent: intent.Source{File: issue},
	})
	require.NoError(t, err)

	_, err = note.Append(layout, runIssue, "shipping is free above 50.00", note.SourceChat, runPR, time.Now())
	require.NoError(t, err)
	held, err := layout.LockPR(runOwner, runRepo, runPR)
	require.NoError(t, err)
	stamp := state.Stamp{Head: head, Round: 1}
	require.NoError(t, state.ReplaceStamped(held, state.FileClaims, stamp, []*intent.Claim{{
		ID: runIssue + "#c1", Text: "The total sums the subtotal and the shipping.",
		Source: intent.ClaimFromAcceptance, Span: "The total sums the subtotal and the shipping.",
	}}))
	require.NoError(t, state.ReplaceStamped(held, state.FileMapping, stamp,
		[]*mapping.Pair{{Claim: runIssue + "#c1", Unit: "u1"}}))
	require.NoError(t, held.Unlock())

	return &Sources{
		Layout: layout, GH: client, Config: cfg, Owner: runOwner, Repo: runRepo, PR: runPR, RepoDir: dir,
	}
}

// promptOf finds the prompt one role emitted for one unit.
func promptOf(t *testing.T, fan *Fanout, roleID, unitID string) string {
	t.Helper()
	for _, prompt := range fan.Prompts {
		if prompt.Role == roleID && prompt.Unit == unitID {
			return prompt.Text
		}
	}
	require.Failf(t, "no prompt", "%s emitted nothing for %s", roleID, unitID)
	return ""
}

// An emitted prompt carries all seven of §4.6.1's attachments: the unit's
// hunks, the claims mapped to it, the candidate symbols of §4.3.1, the rule
// hits of §4.3.6, the test files of §4.4.1, and the threads and notes of §3.5.3
// and §4.1.5 — with the role's instructions and focus above them.
//
// The round is a real one. `cr brief`'s own package clusters the diff and
// records the units, the thread and the active roles, and every attachment is
// then located by the package that owns it, over the same repository, so what
// is asserted is the fan-out cr runs rather than a fixture shaped like one.
func TestAnEmittedPromptCarriesAllSevenAttachments(t *testing.T) {
	fan, err := Run(briefed(t))
	require.NoError(t, err)
	text := promptOf(t, fan, "correctness", "u1")

	for attachment, want := range map[string][]string{
		"the role's instructions and focus": {"## Instructions", "## Focus\n\n- "},
		"the unit's hunks, both halves of the edit": {
			"-func Total() int { return 0 }", "+func Total() int { return subtotal() + shipping() }",
		},
		"the claims mapped to it":    {"- " + runIssue + "#c1: The total sums the subtotal and the shipping."},
		"the candidate symbols":      {"added function formatMoneys (2 params) at order.go:5", "candidate function FormatMoney (2 params) at money.go:3"},
		"the rule hits":              {"rule no-shipping-call at order.go:3", "The quote already carries the shipping amount."},
		"the test files":             {"Changed or added by the pull request: order_test.go."},
		"the threads":                {"PRRT_1 at order.go:3-3 (RIGHT), by reviewer", "Is the shipping arm covered?"},
		"the notes of the issue key": {runIssue + "#n1 (chat, from pull request 7): shipping is free above 50.00"},
	} {
		for _, part := range want {
			assert.Containsf(t, text, part, "§4.6.1: %s", attachment)
		}
	}
}

