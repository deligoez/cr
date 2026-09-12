package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// waivedHome is statusHome's briefed round with one waiver in each of §7.4.4's
// files: a `wrong` the repository-wide file holds, and a `not-here` the pull
// request's own `waivers.ndjson` holds. It returns the two ids in that order.
func waivedHome(t *testing.T) (wide, here string) {
	t.Helper()
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	provenance := finding.WaiverProvenance{Round: meta.Round, PR: fixturePRNumber, Head: meta.Head}
	waive := func(class string, disposition finding.Disposition, reason string) string {
		t.Helper()
		provenance.Reason = reason
		recorded, err := finding.Waive(layout, fixtureOwner, fixtureProject, &finding.Waiver{
			WaiverKey: finding.WaiverKey{
				Path: "lib.go", Side: "RIGHT", Class: class, ContentHash: "0123456789abcdef",
			},
			Disposition: disposition,
		}, provenance)
		require.NoError(t, err)
		return recorded.ID
	}
	return waive("long-function", finding.DispositionWrong, "the class misfires on generated code"),
		waive("missing-test", finding.DispositionNotHere, "")
}

// listedWaivers is `cr waivers list`'s document as a reader decodes it.
type listedWaivers struct {
	Scopes  []string `json:"scopes"`
	Waivers []struct {
		ID          string `json:"id"`
		Scope       string `json:"scope"`
		Disposition string `json:"disposition"`
		Class       string `json:"class"`
		Round       int    `json:"round"`
		PR          int    `json:"pr"`
		Head        string `json:"head"`
		Reason      string `json:"reason"`
	} `json:"waivers"`
	Honesty []string `json:"honesty"`
}

// listWaiversOf runs `cr waivers list` with args and decodes what it printed.
func listWaiversOf(t *testing.T, args ...string) listedWaivers {
	t.Helper()
	printed, err := runCLIPrinting(t, append([]string{"waivers", "list", "--repo", fixtureSlug}, args...)...)
	require.NoError(t, err)
	var listed listedWaivers
	require.NoError(t, json.Unmarshal([]byte(printed), &listed))
	return listed
}

// §7.4.7: without `--pr` the listing reads the repository-wide file alone, so
// the pull request's `not-here` waiver is invisible; with it, both files are
// read and it is listed — each waiver with its scope and its disposition.
//
// The listing with `--pr` also carries §9.3.1's head comparison, because the
// pull request's file is per-PR state under §2.3, and the one without it
// carries none, because it read no round.
func TestAPullRequestScopedWaiverIsListedOnlyWithPR(t *testing.T) {
	wide, here := waivedHome(t)

	repositoryOnly := listWaiversOf(t)
	both := listWaiversOf(t, "--pr", fixturePR)

	assert.Equal(t, []string{"repository"}, repositoryOnly.Scopes)
	require.Len(t, repositoryOnly.Waivers, 1, "§7.4.7: without --pr, the repository-wide file alone")
	assert.Equal(t, wide, repositoryOnly.Waivers[0].ID)
	assert.Equal(t, "repository", repositoryOnly.Waivers[0].Scope)
	assert.Equal(t, "wrong", repositoryOnly.Waivers[0].Disposition)
	assert.Equal(t, "the class misfires on generated code", repositoryOnly.Waivers[0].Reason,
		"§7.4.8's reason is printed with the waiver")
	assert.Empty(t, repositoryOnly.Honesty, "no pull request's state was read, so no head was compared")

	assert.Equal(t, []string{"repository", "pull-request"}, both.Scopes)
	require.Len(t, both.Waivers, 2, "§7.4.7: with --pr, both files of §7.4.4")
	assert.Equal(t, wide, both.Waivers[0].ID)
	assert.Equal(t, here, both.Waivers[1].ID)
	assert.Equal(t, "pull-request", both.Waivers[1].Scope)
	assert.Equal(t, "not-here", both.Waivers[1].Disposition)
	assert.Equal(t, fixturePRNumber, both.Waivers[1].PR, "§7.4.8's provenance is printed with the waiver")
	require.Len(t, both.Honesty, 1)
	assert.Contains(t, both.Honesty[0], "§9.3.1", "the pull request's file is per-PR state")

	shown := throughATerminal(t, "waivers", "list", "--repo", fixtureSlug, "--pr", fixturePR, "--no-color")
	assert.Contains(t, shown, "2 active waiver(s) in the repository and pull-request scope(s)")
	assert.Contains(t, shown, here+" pull-request not-here: missing-test at lib.go RIGHT 0123456789abcdef")
	assert.Contains(t, shown, "    reason: the class misfires on generated code")
	assert.NotContains(t, throughATerminal(t, "waivers", "list", "--repo", fixtureSlug, "--no-color"), here,
		"the terminal rendering without --pr does not show the pull request's waiver either")
}
