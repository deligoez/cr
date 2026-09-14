package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// The two §4.5.4 entries a laravel-pest round owes when its changed source lies
// under src/, which the shipped profile's match.globs do not name.
const (
	unindexedReinvention = "lens convention/reinvention unavailable, per §4.3.1: profile \"laravel-pest\" " +
		"builds §4.3.1's symbol index over its match.globs, which cover none of src/Money.php, and those " +
		"files declare symbols at the head, so for their units nothing added is offered a candidate and " +
		"nothing they declare is offered as one; add a glob covering them to the profile's match.globs"
	unindexedTestSymbols = "lens test/symbols unavailable, per §4.5.4: profile \"laravel-pest\" builds " +
		"§4.3.1's symbol index over its match.globs, which cover none of src/Money.php, and those files " +
		"declare symbols at the head, so a symbol the test files reference from them is not attached; " +
		"add a glob covering them to the profile's match.globs"
	// authorReinvention and authorTestSymbols are the same two entries as the
	// review body words them for the pull request's author.
	authorReinvention = "lens convention/reinvention did not look at src/Money.php: cr's index of the " +
		"repository's existing code does not cover it, so cr did not check whether the change there " +
		"re-implements code the repository already has, and did not offer anything declared there as " +
		"an existing alternative"
	authorTestSymbols = "lens test/symbols did not look at src/Money.php: cr's index of the repository's " +
		"existing code does not cover it, so cr did not read which code there the changed tests exercise"
)

// unindexedHome is release QA's deligoez/cr-qa#1 in miniature: a plain PHP
// repository whose composer.json and phpunit.xml auto-select the shipped
// laravel-pest profile, and whose pull request adds src/Money.php — declaring
// class Money, round() and format() — a test calling it, and a README line.
// The profile indexes app/, config/, database/, routes/, tests/ and views, so
// src/Money.php is changed source the head index holds nothing of, and
// README.md is a changed file outside the globs that declares nothing.
//
// It briefs the round and records an empty mapping, so `cr review` emits every
// axis, and returns the brief's printed document.
func unindexedHome(t *testing.T) string {
	t.Helper()
	return unindexedHomeWith(t, func(state.Layout) {})
}

// unindexedHomeWith is unindexedHome with prepare run over the state root after
// the shipped profiles are written and before the brief, so a test can change
// the profile or the role corpus the round is briefed under.
func unindexedHomeWith(t *testing.T, prepare func(state.Layout)) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("composer.json", "{}\n")
	write("phpunit.xml", "<phpunit/>\n")
	write("README.md", "# money\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("src/Money.php", "<?php\n\nnamespace App;\n\nfinal class Money\n{\n"+
		"    public function __construct(private int $cents)\n    {\n    }\n\n"+
		"    public function round(): self\n    {\n        return new self(intdiv($this->cents, 100) * 100);\n    }\n\n"+
		"    public function format(): string\n    {\n        return number_format($this->cents / 100, 2);\n    }\n}\n")
	write("tests/MoneyTest.php", "<?php\n\nuse App\\Money;\n\n"+
		"test('formats', function () {\n    expect((new Money(150))->format())->toBe('1.50');\n});\n")
	write("README.md", "# money\n\nFormats amounts.\n")
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
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	prepare(layout)
	outside := t.TempDir()
	issue := filepath.Join(outside, "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": format money amounts.\n"), 0o600))
	briefed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	pairs := filepath.Join(outside, "mapping.ndjson")
	require.NoError(t, os.WriteFile(pairs, nil, 0o600))
	_, err = runCLIPrinting(t, "map", "record", fixturePR, pairs, "--repo", fixtureSlug)
	require.NoError(t, err)
	return briefed
}

// A unit whose file the head symbol index does not cover is disclosed as one
// neither lens half could look at — in `cr brief`, in every prompt and the
// honesty report of `cr review`, in `cr status`, and in the review body a
// dry-run `cr post` builds — and nothing says the diff declares nothing there,
// that its tests reference nothing, or that no lens was left unexamined.
//
// Measured on release QA before the fix: every src/Money.php prompt said "The
// diff declares no function, method, or class inside this unit.", the test
// section said "Symbols they reference: none.", honesty was [] in all three
// commands, and the body said "No lens was left unexamined." README.md changes
// too and declares nothing, so the entries name src/Money.php alone.
func TestAUnitTheSymbolIndexDoesNotCoverIsDisclosedEverywhere(t *testing.T) {
	var briefed struct {
		Profile struct {
			ID string `json:"id"`
		} `json:"profile"`
		Units []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"units"`
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(unindexedHome(t)), &briefed))
	require.Equal(t, "laravel-pest", briefed.Profile.ID, "the fixture reproduces QA's auto-selected profile")
	assert.Equal(t, []string{unindexedReinvention, unindexedTestSymbols}, briefed.Honesty, "cr brief")
	money := ""
	for _, formed := range briefed.Units {
		if formed.Path == "src/Money.php" {
			money = formed.ID
		}
	}
	require.NotEmpty(t, money, "src/Money.php forms a unit")

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
	assert.Equal(t, []string{unindexedReinvention, unindexedTestSymbols}, fanout.Honesty, "cr review")
	onMoney := 0
	for _, prompt := range fanout.Prompts {
		lines := strings.Split(prompt.Text, "\n")
		assert.NotContains(t, lines, "Symbols they reference: none.", "%s on %s", prompt.Role, prompt.Unit)
		assert.Contains(t, lines, "Unavailable: "+unindexedTestSymbols, "%s on %s", prompt.Role, prompt.Unit)
		assert.Contains(t, lines, "Unavailable: "+unindexedReinvention, "%s on %s", prompt.Role, prompt.Unit)
		if prompt.Unit == money {
			onMoney++
			assert.NotContains(t, lines, "The diff declares no function, method, or class inside this unit.",
				"%s on %s", prompt.Role, prompt.Unit)
		}
	}
	require.Positive(t, onMoney, "cr review emits prompts for the src/Money.php unit")

	reported, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var status struct {
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(reported), &status))
	assert.Contains(t, status.Honesty, unindexedReinvention, "cr status")
	assert.Contains(t, status.Honesty, unindexedTestSymbols, "cr status")

	merged := filepath.Join(t.TempDir(), "merged.ndjson")
	require.NoError(t, os.WriteFile(merged, []byte(`{"id":"f1","kind":"question","role":"correctness",`+
		`"class":"unchecked-input","severity":"medium","unit":"`+money+`",`+
		`"anchor":{"path":"src/Money.php","side":"RIGHT","start_line":13,"line":13,"content_hash":"0123456789abcdef"},`+
		`"summary":"Should round() round half up rather than truncate?",`+
		`"evidence":"intdiv drops the remainder."}`+"\n"), 0o600))
	_, err = runCLIPrinting(t, "record", fixturePR, merged, "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runCLIPrinting(t, "draft", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	posted, err := runPost(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report dryRun
	require.NoError(t, json.Unmarshal([]byte(posted), &report))
	require.NotNil(t, report.Payload)
	require.False(t, report.Posted)
	body := strings.Split(report.Payload.Body, "\n")
	assert.Contains(t, body, "- "+authorReinvention, "the review body, worded for the author")
	assert.Contains(t, body, "- "+authorTestSymbols, "the review body, worded for the author")
	assert.NotContains(t, body, "No lens was left unexamined.", "the review body")
}
