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

// The review body `cr post` builds states each lens that did not run in words
// the pull request's author can read — which lens, which files, and that cr did
// not look at them — and carries no operator instruction and no section
// number, while `cr status` keeps the reviewer's wording for the same round.
//
// The round is unindexedHome's, briefed with the shipped laravel-pest profile
// switching its test axis off and a role whose profiles list names only
// generic, so one body carries an unindexed-files lens, a disabled axis and two
// skipped roles.
//
// Measured on release QA before the fix (D-S09-5): the body sent to the author
// said "add a glob covering them to the profile's match.globs" for both lens
// halves, and every entry carried a § reference.
func TestTheReviewBodyWordsEveryLensThatDidNotRunForTheAuthor(t *testing.T) {
	briefed := unindexedHomeWith(t, func(layout state.Layout) {
		shipped := profile.Builtins()["laravel-pest"]
		switchedOff := strings.Replace(shipped, `"test": true`, `"test": false`, 1)
		require.NotEqual(t, shipped, switchedOff, "the fixture switches the test axis off")
		require.NoError(t, os.WriteFile(layout.Profile("laravel-pest"), []byte(switchedOff), 0o600))
		require.NoError(t, os.MkdirAll(layout.RolesDir(), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(layout.RolesDir(), "money-safety.json"), []byte(
			`{"id":"money-safety","title":"Money safety","axis":"correctness",`+
				`"instructions":"Check money handling.","profiles":["generic"]}`), 0o600))
	})
	var round struct {
		Units []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"units"`
	}
	require.NoError(t, json.Unmarshal([]byte(briefed), &round))
	money := ""
	for _, formed := range round.Units {
		if formed.Path == "src/Money.php" {
			money = formed.ID
		}
	}
	require.NotEmpty(t, money, "src/Money.php forms a unit")
	merged := filepath.Join(t.TempDir(), "merged.ndjson")
	require.NoError(t, os.WriteFile(merged, []byte(`{"id":"f1","kind":"question","role":"correctness",`+
		`"class":"unchecked-input","severity":"medium","unit":"`+money+`",`+
		`"anchor":{"path":"src/Money.php","side":"RIGHT","start_line":13,"line":13,"content_hash":"0123456789abcdef"},`+
		`"summary":"Should round() round half up rather than truncate?",`+
		`"evidence":"intdiv drops the remainder."}`+"\n"), 0o600))
	_, err := runCLIPrinting(t, "record", fixturePR, merged, "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runCLIPrinting(t, "draft", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	posted, err := runPost(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report dryRun
	require.NoError(t, json.Unmarshal([]byte(posted), &report))
	require.NotNil(t, report.Payload)
	entries := make([]string, 0)
	for line := range strings.SplitSeq(report.Payload.Body, "\n") {
		if entry, ok := strings.CutPrefix(line, "- "); ok {
			entries = append(entries, entry)
		}
	}

	assert.Equal(t, []string{
		"axis test did not run: it is switched off for this repository, so cr did not check " +
			"whether the change is adequately tested",
		authorReinvention,
		authorTestSymbols,
		"role money-safety did not look at this change: it reviews only other kinds of repository than this one",
		"role test-adequacy did not look at this change: axis test did not run",
	}, entries)
	for _, entry := range entries {
		assert.NotContains(t, entry, "match.globs", "the author is given no operator instruction")
		assert.NotContains(t, entry, "§", "the author is given no section number")
	}

	reported, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var status struct {
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(reported), &status))
	assert.Contains(t, status.Honesty, unindexedReinvention, "cr status keeps the reviewer's wording")
}
