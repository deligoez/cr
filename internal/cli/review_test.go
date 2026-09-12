package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/review"
)

// An `--axis` naming no axis of §1.5 is refused as the malformed invocation it
// is, with §11.2's code 2 and the closed set named, before any state is read.
func TestAnAxisOutsideSection15IsAUsageError(t *testing.T) {
	crHome(t)

	err := runCLI(t, "review", "7", "--repo", "acme/api", "--axis", "style")

	var unknown *unknownAxisFlagError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, ExitUsage, exitCodeFor(err))
	assert.Contains(t, err.Error(), "intent, correctness, convention, test")
}

// A terminal prints every prompt whole under the role and unit it is for, and
// then the lenses that did not run.
func TestATerminalReviewPrintsEveryPromptWholeUnderItsRoleAndUnit(t *testing.T) {
	result := &reviewResult{Fanout: &review.Fanout{
		Round: 2, Head: "abc123",
		Prompts: []review.Prompt{
			{Role: "correctness", Axis: "correctness", Unit: "u1", Text: "# Correctness on u1\n\nthe whole prompt\n"},
			{Role: "convention", Axis: "convention", Unit: "u1", Text: "# Convention on u1\n"},
		},
		Honesty: []string{"lens test/symbols unavailable, per §4.5.4: no index"},
	}}

	var printed bytes.Buffer
	require.NoError(t, (&writer{out: &printed, mode: ModeText}).emit(result))

	text := printed.String()
	assert.Contains(t, text, "review round 2 at head abc123: 2 prompt(s)")
	assert.Contains(t, text, "prompt correctness on u1\n\n# Correctness on u1\n\nthe whole prompt\n")
	assert.Contains(t, text, "prompt convention on u1\n\n# Convention on u1\n")
	assert.Contains(t, text, "lens test/symbols unavailable, per §4.5.4: no index")
}

// §4.6.4's skipped role reaches the reader from `cr review` and from `cr
// status`, as one sentence rather than two.
//
// The round resolves the shipped `generic` profile, which declares no
// `tests.cmd`, so §4.5.2 disables the test axis and §4.5.1 leaves the role on
// it out of the active set. §4.6.4 has that role reported with its reason
// rather than omitted silently, and §4.5.4 has the report reach the reader
// through the channel §11.1 exempts from `--quiet`, which is the `honesty`
// array both commands print.
//
// The two commands are read from one fixture and compared against each other,
// because the failure worth guarding is not either of them saying nothing. It
// is the two deriving the set separately and naming different roles, or the
// same role for different reasons: each command would read as internally
// consistent, and no reader of either alone could see the disagreement.
func TestASkippedRoleReachesBothTheFanOutAndTheStatusReport(t *testing.T) {
	statusHome(t)

	fannedOut, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	quieted, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug, "--quiet")
	require.NoError(t, err)
	reported, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	var fanout, quiet, status struct {
		Skipped []coverage.SkippedRole `json:"skipped_roles"`
		Honesty []string               `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(fannedOut), &fanout))
	require.NoError(t, json.Unmarshal([]byte(quieted), &quiet))
	require.NoError(t, json.Unmarshal([]byte(reported), &status))

	// §4.6.4, from `cr review`: the role is named and so is what kept it
	// from looking. The reason is the axis's own entry rather than a
	// rewording, so a reader acting on this line and one acting on the
	// axis's line are sent to the same fix.
	require.Len(t, fanout.Skipped, 1, "§4.6.4: the role on the disabled test axis is reported skipped")
	assert.Equal(t, "test-adequacy", fanout.Skipped[0].Role)
	assert.Contains(t, fanout.Skipped[0].Reason, "axis test disabled, per §4.5.2",
		"§4.5.4: the reason names what would make the role run")

	// One derivation, read twice. coverage.Skipped is the only place
	// either command answers this, so the two reports are the same value.
	assert.Equal(t, status.Skipped, fanout.Skipped,
		"§4.6.4 is one derivation: `cr review` and `cr status` name the same roles for the same reasons")

	// §4.5.4's channel, on both sides. The field above is the report as
	// data; this is the sentence a person is shown, and §11.1 exempts it
	// from `--quiet` — so a run carrying the flag prints the same list.
	line := fanout.Skipped[0].Disclosure()
	assert.Contains(t, fanout.Honesty, line, "§4.5.4: `cr review` discloses it beside the lens halves")
	assert.Contains(t, status.Honesty, line, "§10.1.3: `cr status` discloses it in the lens list")
	assert.Equal(t, fanout.Honesty, quiet.Honesty, "§11.1: `--quiet` may suppress no honesty disclosure")
}
