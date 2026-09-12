package cli

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// theRejection is gh's failure for a review-creation call GitHub refused, in
// the shape measured against `gh api search/issues`: the response document on
// standard output, a one-line summary on standard error, and a non-zero exit.
func theRejection(body string) error {
	return &gh.CommandError{
		Args:   []string{"api", "repos/acme/web/pulls/7/reviews", "--method", "POST"},
		Stdout: body,
		Stderr: "gh: Validation Failed (HTTP 422)",
		Err:    errors.New("exit status 1"),
	}
}

// §8.4.2 end to end over the round `cr draft` left queued: the call is
// rejected, every position GitHub named is reported with the record it belongs
// to, the refusal codes 4, and nothing of the round moved.
//
// The two halves of the last sentence are asserted together because they are
// one fact told twice. §9.1's only edge into `posted` and §7.3.1's outcome
// events both live past the network call, so a rejection that left either
// behind would have the round claiming a review the author never received —
// and §7.3.4's demotion rate would then count an outcome for a comment that was
// never posted.
func TestARejectedCallNamesEveryPositionAndMovesNothing(t *testing.T) {
	records := []*finding.Finding{aCitedRecord("f1"), aCitedRecord("f2")}
	records[1].Anchor.Path = "internal/api/f2.go"
	layout := draftedHome(t, records...)
	redraft(t)
	before := ledger(t, layout)

	round := state.Meta{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum,
		Round: draftRound, Head: draftHead,
	}
	queued, err := roundFindingsOf(layout, draftOwner, draftRepo, draftPRNum, draftRound)
	require.NoError(t, err)
	review, err := buildPayload(
		layout, draftOwner, draftRepo, draftPRNum, &round, queued, map[string]string{})
	require.NoError(t, err)

	refused := rejectedPost(review, theRejection(
		`{"message":"Validation Failed","errors":[{"resource":"PullRequestReviewComment",`+
			`"field":"line","code":"custom","message":"line must be part of the diff"}],`+
			`"status":"422"}`))

	var rejected *post.RejectedError
	require.ErrorAs(t, refused, &rejected)
	assert.Equal(t, ExitState, exitCodeFor(refused), "§8.4.2 exits 4")
	assert.Contains(t, refused.Error(), "f1 internal/api/handler.go:44:")
	assert.Contains(t, refused.Error(), "f2 internal/api/f2.go:44:")
	assert.Contains(t, refused.Error(), "line must be part of the diff")

	stored, err := state.ReadRecords[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err)
	for i := range stored {
		assert.Equal(t, finding.StateQueued, stored[i].State,
			"§8.4.2: no state is marked posted when the call is rejected")
	}
	assert.Equal(t, before, ledger(t, layout),
		"§7.3.1: the outcome events are written after the call, so a rejection leaves none")
	assert.Equal(t, []string{"f1:raised", "f2:raised"}, before)
}

// The two failures that are not §8.4.2's are passed through as they arrived,
// so neither is reported as a rejection.
//
// A response cr cannot parse is §8.4.4's unknown outcome — cr does not know
// whether the review was created — and a gh that failed some other way is
// §3.1.3's external command failure. Both code 3, which is what says cr is not
// claiming the review was refused.
func TestOnlyGitHubSayingNoIsARejection(t *testing.T) {
	review := post.Build(
		[]*finding.Finding{{ID: "f1", Anchor: finding.Anchor{Path: "a.go", Line: 1}}},
		map[string]string{"f1": "body"})

	for name, failure := range map[string]error{
		"unparseable response": theRejection("<html>502 Bad Gateway</html>"),
		"gh never started":     theRejection(""),
	} {
		t.Run(name, func(t *testing.T) {
			passed := rejectedPost(review, failure)

			assert.Same(t, failure, passed, "the caller sees the error it would have seen")
			assert.Equal(t, ExitFile, exitCodeFor(passed), "§3.1.3 codes an external command failure 3")
		})
	}

	unrelated := errors.New("something else entirely")
	assert.Same(t, unrelated, rejectedPost(review, unrelated))
}
