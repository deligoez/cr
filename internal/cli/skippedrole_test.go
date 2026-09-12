package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/state"
)

// A role with an empty profiles list, on an axis that ran, that the round's
// recorded active roles do not name, is reported skipped for that reason by
// both `cr review` and `cr status` — and the sentence names no empty list.
//
// The role is written after the round was briefed, which is how the dogfood
// met it: statusHome's meta.json records the three roles `cr brief` counted,
// and a global role added afterwards on the running correctness axis is one
// the definition would admit today and the round never counted. Before the fix
// both commands said "its profiles list names  and this round resolved profile
// generic; the role looks only under a profile it names".
func TestAnEmptyProfilesRoleIsSkippedForTheReasonThatDecidedInBothReports(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	require.NoError(t, os.WriteFile(layout.Role("error-paths"), []byte(
		`{"id":"error-paths","title":"Error paths","axis":"correctness",`+
			`"instructions":"Look at every returned error.","focus":[],"profiles":[]}`), 0o600))

	fannedOut, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	reported, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var fanout, status struct {
		Skipped []coverage.SkippedRole `json:"skipped_roles"`
		Honesty []string               `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(fannedOut), &fanout))
	require.NoError(t, json.Unmarshal([]byte(reported), &status))

	want := coverage.SkippedRole{
		Role: "error-paths",
		Reason: "this round's active roles, which `cr brief` recorded per §4.5.1, do not name it, " +
			"though its axis correctness ran and its profiles list is empty, which admits every profile; " +
			"run `cr brief` again to record the round's active roles anew",
	}
	assert.Contains(t, fanout.Skipped, want, "§4.6.4: the role is reported skipped with the reason that decided")
	assert.Equal(t, status.Skipped, fanout.Skipped, "§4.6.4 is one derivation, read by both commands")
	assert.NotContains(t, want.Reason, "names  ", "the sentence carries no empty name")
	assert.Contains(t, fanout.Honesty, want.Disclosure(), "§4.5.4: `cr review` discloses it")
	assert.Contains(t, status.Honesty, want.Disclosure(), "§10.1.3: `cr status` discloses it")
}
