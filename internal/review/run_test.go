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

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/axis"
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

// briefed opens round 1 on the shop repository, records a note, a claim, and a
// mapping of that claim to u1, and returns the fan-out's sources.
//
// It is the round after §4.6.5's first pass has been run and its mapping
// stored, which is the state every lens but the intent pass reads.
func briefed(t *testing.T) *Sources {
	t.Helper()
	src, head := briefedWithoutMapping(t)
	recordMapping(t, src, head)
	return src
}

// recordMapping stores §4.1.6's mapping of the round's one claim to u1, as
// `cr map record` stores it: replacing the round's pairs under the §2.3.1 lock.
func recordMapping(t *testing.T, src *Sources, head string) {
	t.Helper()
	held, err := src.Layout.LockPR(runOwner, runRepo, runPR)
	require.NoError(t, err)
	require.NoError(t, state.ReplaceStamped(held, state.FileMapping,
		state.Stamp{Head: head, Round: 1},
		[]*mapping.Pair{{Claim: runIssue + "#c1", Unit: "u1"}}))
	require.NoError(t, held.Unlock())
}

// briefedWithoutMapping opens round 1 on the shop repository through `cr
// brief`'s own package, records a note and a claim but no mapping, and returns
// the fan-out's sources beside the round's head.
//
// That is where §4.6.5's first pass runs: the claims are recorded and the join
// is not, because producing it is what the pass is for.
func briefedWithoutMapping(t *testing.T) (src *Sources, head string) {
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
	require.NoError(t, held.Unlock())

	return &Sources{
		Layout: layout, GH: client, Config: cfg, Owner: runOwner, Repo: runRepo, PR: runPR, RepoDir: dir,
	}, head
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

// A hit's standard names the file the rule is written in — for a rule the
// profile ships, the profile's own file — because rule.Resolved's Path is the
// file a user opens to change the standard. The case above asserts the rule
// and its rationale but not where it came from, so a round that lost the path
// on the way into rule.Resolve would print "from " with nothing after it.
func TestAHitsStandardNamesTheProfileFileItShipsIn(t *testing.T) {
	src := briefed(t)
	fan, err := Run(src)
	require.NoError(t, err)

	assert.Contains(t, promptOf(t, fan, "correctness", "u1"), "\n  from "+src.Layout.Profile("shop")+"\n")
}

// The round's active roles each get a prompt for each unit, and the halves that
// could not run are reported beside them rather than left out.
func TestRunEmitsEveryActiveRoleOverEveryUnitOfTheRound(t *testing.T) {
	fan, err := Run(briefed(t))
	require.NoError(t, err)

	assert.Equal(t, 1, fan.Round)
	at := make([]string, 0, len(fan.Prompts))
	for _, prompt := range fan.Prompts {
		at = append(at, prompt.Role+"/"+prompt.Unit)
	}
	assert.Equal(t, []string{
		"convention/u1", "convention/u2", "correctness/u1", "correctness/u2",
		"intent-coverage/u1", "intent-coverage/u2", "test-adequacy/u1", "test-adequacy/u2",
	}, at)
	assert.Equal(t, []string{
		"lens test/symbols unavailable, per §4.5.4: cr built no symbol index for symbols.lang \"go\"",
	}, fan.Honesty, "§4.4.1's symbol half has no index to read, and says so")
	assert.Contains(t, promptOf(t, fan, "intent-coverage", "u2"), "Unmapped unit (§4.1.2)",
		"the test file's unit is mapped to no claim, so the intent role raises it")
}

// `--axis` narrows the fan-out to one axis's roles.
//
// The round is the one before a mapping is stored, so what is measured here is
// the role narrowing alone: §4.6.5's second pass narrows the units as well, and
// a round carrying both narrowings could pass this while emitting one axis over
// the wrong units.
func TestAnAxisNarrowsTheFanOutToItsRoles(t *testing.T) {
	src, _ := briefedWithoutMapping(t)
	src.Axis = axis.Intent

	fan, err := Run(src)
	require.NoError(t, err)
	require.Len(t, fan.Prompts, 2)
	for _, prompt := range fan.Prompts {
		assert.Equal(t, axis.Intent, prompt.Axis)
	}
}

// §4.6.5's two passes, driven in order over one round.
//
// The first pass emits over every unit and carries the round's claims with no
// join, because the join is what it exists to produce. The mapping is then
// stored the way `cr map record` stores it, and the second pass re-emits one
// prompt for the single unit that mapping maps to zero claims — carrying
// §4.1.2's item and §4.1.5's notes, which the first pass could not have carried
// about any unit.
func TestTheIntentAxisRunsInTwoPasses(t *testing.T) {
	src, head := briefedWithoutMapping(t)
	src.Axis = axis.Intent
	claim := "- " + runIssue + "#c1: The total sums the subtotal and the shipping."

	first, err := Run(src)
	require.NoError(t, err)
	require.Len(t, first.Prompts, 2, "§4.6.5: unmapped-ness is unknowable, so every unit is emitted")
	for _, prompt := range first.Prompts {
		assert.Contains(t, prompt.Text, claim, "§4.6.5: the first pass carries the claims")
		assert.Contains(t, prompt.Text, "No mapping is recorded for round 1 yet",
			"and no mapping")
		assert.NotContains(t, prompt.Text, "Unmapped unit (§4.1.2)")
	}

	recordMapping(t, src, head)

	second, err := Run(src)
	require.NoError(t, err)
	require.Len(t, second.Prompts, 1, "§4.6.5: one prompt per unit mapped to zero claims")
	assert.Equal(t, "u2", second.Prompts[0].Unit, "u1 is the unit the mapping covers")
	text := second.Prompts[0].Text
	assert.Contains(t, text, "Unmapped unit (§4.1.2)")
	assert.Contains(t, text, runIssue+"#n1 (chat, from pull request 7): shipping is free above 50.00",
		"§4.1.5: the notes the agent decides against")
	assert.NotContains(t, text, claim, "u2 is mapped to no claim, so none is shown as mapped to it")
}

// The remaining axes are refused with §11.2's code 4 until a mapping exists for
// the round, and the intent pass is the one invocation that is not — it is what
// produces the mapping the others are waiting for.
func TestTheRemainingAxesAreRefusedUntilAMappingExists(t *testing.T) {
	src, head := briefedWithoutMapping(t)

	for _, only := range []string{"", axis.Correctness, axis.Convention, axis.Test} {
		src.Axis = only
		_, err := Run(src)

		var required *MappingRequiredError
		require.ErrorAsf(t, err, &required, "--axis %q", only)
		assert.Equal(t, 1, required.Round)
		assert.Contains(t, err.Error(), "cr review 7 --repo acme/shop --axis intent")
	}
}

// A unit the round recorded that the diff at the recorded head does not give is
// refused, naming the unit and the brief that records the units again, rather
// than emitted over code its cell would not be counted against.
func TestAUnitTheDiffNoLongerGivesIsRefused(t *testing.T) {
	src := briefed(t)
	meta, err := src.Layout.ReadMeta(runOwner, runRepo, runPR)
	require.NoError(t, err)
	held, err := src.Layout.LockPR(runOwner, runRepo, runPR)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"order.go","side":"RIGHT","hunk_ranges":[{"start":40,"end":44}],`+
			`"head":"`+meta.Head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())

	_, err = Run(src)
	var stale *StaleUnitError
	require.ErrorAs(t, err, &stale)
	assert.Equal(t, "u1", stale.Unit)
	assert.Contains(t, err.Error(), "cr brief 7 --repo acme/shop")
}

// A review fan-out leaves findings.ndjson untouched (§4.6.3): the file carries
// the same bytes and the same modification time afterwards. What the fan-out
// does write is the directories its prompts name, under the state root (§2.2),
// and nothing inside them — the records are the roles' to write.
func TestAReviewFanOutLeavesFindingsUntouched(t *testing.T) {
	src := briefed(t)
	held, err := src.Layout.LockPR(runOwner, runRepo, runPR)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(`{"id":"f1","kind":"question","role":"correctness",`+
		`"class":"unchecked-shipping","severity":"low","unit":"u1","summary":"s","evidence":"e","round":1,"head":"h"}`+"\n")))
	require.NoError(t, held.Unlock())
	path := src.Layout.PRFile(runOwner, runRepo, runPR, state.FileFindings)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	stamped, err := os.Stat(path)
	require.NoError(t, err)

	fan, err := Run(src)
	require.NoError(t, err)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "§4.6.3: cr review writes nothing to findings.ndjson")
	restamped, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, stamped.ModTime(), restamped.ModTime(), "and does not rewrite it with the same bytes either")

	require.NotEmpty(t, fan.Prompts)
	for _, prompt := range fan.Prompts {
		assert.True(t, strings.HasPrefix(prompt.Output, src.Layout.Root()+string(filepath.Separator)),
			"§2.2: %s is under the state root", prompt.Output)
		entries, err := os.ReadDir(filepath.Dir(prompt.Output))
		require.NoError(t, err, "the directory a role writes into exists")
		assert.Empty(t, entries, "and cr put no file in it")
	}
}

// A fan-out directory cr cannot create fails the review. Every prompt names its
// directory as where the role writes, so a run that carried on without one
// would hand each role a path it cannot write to; fanOut has the write's own
// failure be what the caller is told about, and no case above makes the write
// fail.
func TestAFanOutDirectoryThatCannotBeCreatedFailsTheReview(t *testing.T) {
	src := briefed(t)
	blocker := filepath.Join(src.Layout.PRDir(runOwner, runRepo, runPR), state.DirFanOut)
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0o600))

	fan, err := Run(src)

	require.Error(t, err)
	assert.Nil(t, fan)
}
