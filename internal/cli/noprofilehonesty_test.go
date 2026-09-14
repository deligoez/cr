package cli

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
)

// noProfileTestAxis is the sentence §2.4.4's repository reports its test axis
// with, per §4.5.2, wherever an axis report is printed.
const noProfileTestAxis = "axis test disabled, per §4.5.2: no profile matched this repository, so no " +
	"tests.cmd is declared for this axis to run; set `profile` in the per-repository config to name the " +
	"profile this repository is"

// noProfileNoIndex is the reason §2.4.4's repository reports both lens halves
// with, per §4.3.1 and §4.5.4: there is no symbols.lang to build an index from.
const noProfileNoIndex = "no profile matched this repository, so §4.3.1's symbol index cannot be built; " +
	"set `profile` in the per-repository config to name the profile this repository is"

// A repository no profile matches still runs every axis that needs none, and all
// three commands say which lenses did not run.
//
// keylessHome configures `generic`; this test empties that configuration, so the
// checkout — one Go file, no marker file — is §2.4.4's repository, and with no
// issue key §4.5.3 leaves the intent axis unavailable too. What is left to run
// is correctness and convention, and before the fix a brief here recorded no
// active role at all and `cr review` emitted no prompt while the honesty report
// named only the test axis and the reinvention half.
//
// `cr review`'s honesty report is asserted equal to the lens section of
// `cr status`'s, because §4.5.4's report is one report whichever command prints
// it: a disabled or unavailable axis `cr status` states and `cr review` leaves
// out is the silence that section forbids.
func TestWithNoProfileEveryCommandRunsTheAxesThatNeedNoneAndSaysWhichDidNot(t *testing.T) {
	layout := keylessHome(t)
	require.NoError(t, os.WriteFile(layout.RepoConfig(fixtureOwner, fixtureProject), []byte(`{}`), 0o600))

	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var briefed struct {
		Axes struct {
			Active []string `json:"active"`
		} `json:"axes"`
		ActiveRoles []string `json:"active_roles"`
		Honesty     []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	assert.Equal(t, []string{axis.Correctness, axis.Convention}, briefed.Axes.Active)
	assert.Equal(t, []string{"convention", "correctness"}, briefed.ActiveRoles)

	fannedOut, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	reported, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var fanout struct {
		Prompts []struct {
			Role string `json:"role"`
		} `json:"prompts"`
		Honesty []string `json:"honesty"`
	}
	var status struct {
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(fannedOut), &fanout))
	require.NoError(t, json.Unmarshal([]byte(reported), &status))

	roles := make([]string, 0, len(fanout.Prompts))
	for _, prompt := range fanout.Prompts {
		roles = append(roles, prompt.Role)
	}
	slices.Sort(roles)
	assert.Equal(t, []string{"convention", "correctness"}, roles, "one prompt per running role on the one unit")

	require.GreaterOrEqual(t, len(fanout.Honesty), 2)
	intentAxis := fanout.Honesty[1]
	assert.Equal(t, []string{noProfileTestAxis, intentAxis}, fanout.Honesty[:2],
		"§4.5.4: `cr review` states the disabled and the unavailable axis first")
	assert.Equal(t, []string{
		noProfileTestAxis,
		intentAxis,
		"lens convention/reinvention unavailable, per §4.3.1: " + noProfileNoIndex,
		"lens test/symbols unavailable, per §4.5.4: " + noProfileNoIndex,
		"role intent-coverage skipped, per §4.6.4: " + intentAxis,
		"role test-adequacy skipped, per §4.6.4: " + noProfileTestAxis,
	}, briefed.Honesty, "`cr brief` states §4.5.4's report whole")
	assert.Equal(t, fanout.Honesty, briefed.Honesty, "`cr brief` and `cr review` give one report")
	at := slices.Index(status.Honesty, noProfileTestAxis)
	require.GreaterOrEqual(t, at, 0, "`cr status` states the disabled axis")
	require.LessOrEqual(t, at+len(fanout.Honesty), len(status.Honesty))
	assert.Equal(t, fanout.Honesty, status.Honesty[at:at+len(fanout.Honesty)],
		"§4.5.4's report is the same run of sentences in `cr review` and `cr status`")
}
