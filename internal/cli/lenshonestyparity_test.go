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

// lensParityHome briefs release QA's deligoez/cr-qa#1 in miniature with the
// per-repository config given: a PHP repository carrying no marker file, whose
// pull request adds src/Money.php and a test calling it. It records an empty
// mapping, so `cr review` emits every axis, and returns the brief's printed
// document.
func lensParityHome(t *testing.T, repoConfig string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("README.md", "# money\n")
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("src/Money.php", "<?php\n\nnamespace App;\n\nfinal class Money\n{\n"+
		"    public function format(int $cents): string\n    {\n        return number_format($cents / 100, 2);\n    }\n}\n")
	write("tests/MoneyTest.php", "<?php\n\nuse App\\Money;\n\n"+
		"test('formats', function () {\n    expect((new Money())->format(150))->toBe('1.50');\n});\n")
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
	require.NoError(t, os.MkdirAll(filepath.Dir(layout.RepoConfig(fixtureOwner, fixtureProject)), 0o700))
	require.NoError(t, os.WriteFile(layout.RepoConfig(fixtureOwner, fixtureProject), []byte(repoConfig), 0o600))
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

// lensEntries are the entries of an honesty list that are §4.5.4's report — an
// axis, a lens half, or a role that did not run — in the order they were given.
// `cr status` prints its own §9.3.1, §10.2 and §6.4 lines beside them.
func lensEntries(honesty []string) []string {
	out := make([]string, 0, len(honesty))
	for _, line := range honesty {
		if strings.HasPrefix(line, "axis ") || strings.HasPrefix(line, "lens ") || strings.HasPrefix(line, "role ") {
			out = append(out, line)
		}
	}
	return out
}

// `cr brief`, `cr review` and `cr status` give §4.5.4's report for one round as
// one list, whether the round's profile is `generic` — which declares no
// symbols.lang — or no profile matched at all.
//
// Measured on release QA before the fix (D-S02-2): with `generic`, the brief's
// honesty named the test axis and the test-adequacy role and left out the
// reinvention half and the test/symbols lens that `cr review` and `cr status`
// both named; with no profile, it named the reinvention half inside §2.4.4's
// sentence and never the test/symbols lens.
func TestBriefReviewAndStatusNameTheSameLensesThatDidNotRun(t *testing.T) {
	generic := "lens convention/reinvention unavailable, per §4.3.1: profile \"generic\" declares no " +
		"symbols.lang, so §4.3.1's symbol index cannot be built; set symbols.lang in the profile to name " +
		"this repository's language"
	genericSymbols := "lens test/symbols unavailable, per §4.5.4: profile \"generic\" declares no " +
		"symbols.lang, so §4.3.1's symbol index cannot be built; set symbols.lang in the profile to name " +
		"this repository's language"
	genericTest := "axis test disabled, per §4.5.2: the resolved profile generic declares no tests.cmd, " +
		"so this axis has no runner to reach; add tests.cmd and tests.globs to the profile"
	for _, tc := range []struct {
		name, config string
		want         []string
	}{
		{"profile generic", `{"profile":"generic"}`, []string{
			genericTest, generic, genericSymbols,
			"role test-adequacy skipped, per §4.6.4: " + genericTest,
		}},
		{"no matching profile", `{}`, []string{
			noProfileTestAxis,
			"lens convention/reinvention unavailable, per §4.3.1: " + noProfileNoIndex,
			"lens test/symbols unavailable, per §4.5.4: " + noProfileNoIndex,
			"role test-adequacy skipped, per §4.6.4: " + noProfileTestAxis,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var briefed, fanout, status struct {
				Honesty []string `json:"honesty"`
			}
			require.NoError(t, json.Unmarshal([]byte(lensParityHome(t, tc.config)), &briefed))
			reviewed, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal([]byte(reviewed), &fanout))
			reported, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal([]byte(reported), &status))

			assert.Equal(t, tc.want, briefed.Honesty, "cr brief")
			assert.Equal(t, briefed.Honesty, fanout.Honesty, "cr review")
			assert.Equal(t, briefed.Honesty, lensEntries(status.Honesty), "cr status")
		})
	}
}
