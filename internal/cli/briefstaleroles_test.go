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

// briefHonesty runs `cr brief` over the fixture's pull request and returns the
// `honesty` sentences of the document it printed.
func briefHonesty(t *testing.T, issue string) []string {
	t.Helper()
	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	var briefed struct {
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	return briefed.Honesty
}

// ejectRole writes onDisk as the corpus's correctness role and returns its path.
func ejectRole(t *testing.T, onDisk []byte) string {
	t.Helper()
	roles := filepath.Join(os.Getenv(state.HomeEnv), "roles")
	require.NoError(t, os.MkdirAll(roles, 0o700))
	file := filepath.Join(roles, "correctness.json")
	require.NoError(t, os.WriteFile(file, onDisk, 0o600))
	return file
}

// §2.5.2 through `cr brief`: the command resolves the role corpus — §3.7.6
// prints the roles the round's fan-out will use — so it owes the same report
// every other command that loads the corpus owes, naming the file, the release
// and `cr init`.
//
// The sentence is asserted whole and built from role.staleNotice's own wording
// through the same fixture `cr status` is held to, so `cr brief`'s report is
// byte-identical to theirs rather than a second phrasing of the same fact. It
// is read off the printed document and off the terminal, because a report the
// reader never sees is not a report and the two rendering paths are separate.
//
// Both directions: an edited file and a file already at this release are not
// named, so the report measures the stale file and not the presence of a role.
func TestBriefReportsAnEjectedRoleAnEarlierReleaseShipped(t *testing.T) {
	previous, current := staleRoleBytes(t)
	edited := strings.Replace(string(previous), `"title": "Correctness`, `"title": "Sharpness`, 1)
	require.NotEqual(t, string(previous), edited)

	for _, standing := range []struct {
		name   string
		onDisk []byte
		named  bool
	}{
		{name: "an earlier release's role", onDisk: previous, named: true},
		{name: "a role somebody edited", onDisk: []byte(edited)},
		{name: "a role at this release", onDisk: []byte(current)},
	} {
		t.Run(standing.name, func(t *testing.T) {
			emptyRoundHome(t)
			issue := writeOutside(t, "issue.txt", fixtureIssue+": load the configuration.\n")
			file := ejectRole(t, standing.onDisk)
			said := file + " is the correctness role cr v0.2.2 shipped, unedited, and the shipped " +
				"role has since changed instructions; cr init updates the file to it, and until then " +
				"every prompt cr emits for this role carries the earlier release's framing"

			honesty := briefHonesty(t, issue)
			shown := throughATerminal(t, "brief", fixturePR, "--repo", fixtureSlug,
				"--issue", fixtureIssue, "--intent-file", issue, "--no-color")

			if !standing.named {
				assert.NotContains(t, strings.Join(honesty, "\n"), "correctness role cr v0.2.2 shipped",
					"§2.5.2 reports the file an earlier release shipped, unedited, and no other")
				assert.NotContains(t, shown, "correctness role cr v0.2.2 shipped")
				return
			}
			assert.Contains(t, honesty, said, "§2.5.2: the document owes the report")
			assert.Contains(t, shown, said, "§2.5.2: and so does the terminal")
		})
	}
}

// A round whose corpus is the built-in roles alone reports none: §2.5.2's
// report is about an ejected file, and an unejected role is resolved from the
// binary, which is this build's by construction.
func TestBriefReportsNoStaleRoleWhenNoneIsEjected(t *testing.T) {
	emptyRoundHome(t)
	issue := writeOutside(t, "issue.txt", fixtureIssue+": load the configuration.\n")

	assert.NotContains(t, strings.Join(briefHonesty(t, issue), "\n"), "correctness role cr")
}
