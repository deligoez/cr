package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §5.1.6: a post-setup baseline recording a profile other than the round's is
// not Clean. The field report behind it: a sandbox built under a profile that
// copied `storage/*.key` survived the repository's switch to one that does not,
// so `cr test` kept running with keys the new profile never copies and said
// nothing until the sandbox was destroyed by hand.
//
// The sandbox is built under `qa`, which copies `.env`, and the round then
// resolves `plain`, which copies nothing. The next `cr test` must rebuild it,
// say why, and leave a sandbox holding what `plain` brings — no `.env` — so
// §5.1.2's missing-file notice, the one the tester never saw, is printed.
func TestASandboxBuiltUnderAnotherProfileIsRecreated(t *testing.T) {
	root, sandboxPath, runner, _ := envFixture(t, []string{".env"}, ".env")
	streams(t, "sandbox", "create", fixturePR, "--repo", fixtureSlug)
	require.FileExists(t, filepath.Join(sandboxPath, ".env"), "the control: qa copied .env in")

	layout := state.New(crHomeOf(t))
	require.NoError(t, layout.EnsureProfile("plain", `{"id":"plain",`+
		`"match":{"files":[],"globs":[]},"axes":{"test":true},`+
		`"tests":{"cmd":["`+runner+`"],"globs":["*_test.txt"],"filter_flag":"--only",`+
		`"count_pattern":"Tests:  ([0-9]+) (?:failed|passed)","failed_pattern":"Tests:  ([0-9]+) failed"}}`))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.ProfileID = "plain"
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())

	missing := ".env is gitignored at the clone root " + root +
		" and absent from the sandbox, so the suite runs without it; add it to sandbox.copy in " +
		layout.Profile("plain") + " to copy it in"
	stdout, _ := streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, []any{"sandbox " + sandboxPath + " recreated, per §5.1.6: " +
		"it was built under profile qa, and the round's is plain", missing}, honestyList(t, stdout))
	_, err = os.Stat(filepath.Join(sandboxPath, ".env"))
	assert.ErrorIs(t, err, os.ErrNotExist, "the rebuilt sandbox holds only what plain copies")

	stdout, _ = streams(t, "test", fixturePR, "--repo", fixtureSlug)
	assert.Equal(t, []any{missing}, honestyList(t, stdout), "the sandbox built under plain stands")
}

