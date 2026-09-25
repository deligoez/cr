package cli

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// queueARecord writes one queued record into the briefed round of the
// fixture pull request.
func queueARecord(t *testing.T) {
	t.Helper()
	layout := state.New(crHomeOf(t))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(
		`{"id":"f1","state":"queued","head":"`+meta.Head+`","round":`+strconv.Itoa(meta.Round)+`}`+"\n")))
	require.NoError(t, held.Unlock())
}

// draftStepOf is the `draft` step `cr next` reports.
func draftStepOf(t *testing.T) nextStep {
	t.Helper()
	for _, step := range nextSteps(t) {
		if step.Step == "draft" {
			return step
		}
	}
	require.FailNow(t, "cr next reports no draft step")
	return nextStep{}
}

// §10.4.8: when the round's last `cr brief` found the pull request not open,
// the draft step says so and lists no `cr post`; `cr draft` stays.
func TestTheDraftStepOfAMergedPullRequestProposesNoPost(t *testing.T) {
	stateHome(t, gh.StateMerged)
	queueARecord(t)

	step := draftStepOf(t)

	assert.Equal(t, []string{"cr draft " + fixturePR + " --repo " + fixtureSlug}, step.Commands)
	assert.Contains(t, step.Why, "the pull request merged")
	assert.Equal(t, []string{"f1"}, step.Items)
}

