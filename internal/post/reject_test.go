package post

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// aRejectedReview is the payload GitHub refused: two comments on two files, so
// a response naming one path can be told from one naming none.
func aRejectedReview() *Review {
	return Build([]*finding.Finding{
		{ID: "f1", Anchor: finding.Anchor{Path: "src/Order.php", Side: "RIGHT", StartLine: 8, Line: 8}},
		{ID: "f2", Anchor: finding.Anchor{Path: "src/TaxRate.php", Side: "RIGHT", StartLine: 12, Line: 12}},
	}, map[string]string{"f1": "one", "f2": "two"})
}

// §8.4.2's report over GitHub's own document, in the shape measured against
// `gh api search/issues`: the message, the status, and an `errors` array of
// objects.
//
// The entry names a field and no comment, which is what GitHub's validation
// errors do, so every comment of the payload is named with its record id. That
// is the criterion read honestly: the alternative is picking one of the two on
// no evidence, and a refusal that names the wrong record costs exactly what
// P2 says a wrong assertion costs.
func TestARejectionNamesEveryPositionWithItsRecord(t *testing.T) {
	rejected := Rejection(aRejectedReview(), `{"message":"Validation Failed","errors":[`+
		`{"resource":"PullRequestReviewComment","field":"line","code":"custom",`+
		`"message":"line must be part of the diff"}],"status":"422"}`)

	require.NotNil(t, rejected)
	assert.Equal(t, "Validation Failed", rejected.Message)
	assert.Equal(t, "422", rejected.Status)
	assert.Equal(t, []InvalidPosition{
		{Record: "f1", Path: "src/Order.php", Line: 8, Field: "line", Message: "line must be part of the diff"},
		{Record: "f2", Path: "src/TaxRate.php", Line: 12, Field: "line", Message: "line must be part of the diff"},
	}, rejected.Positions)
	assert.Contains(t, rejected.Error(), "f1 src/Order.php:8: line: line must be part of the diff")
	assert.Contains(t, rejected.Error(), "f2 src/TaxRate.php:12:")
	assert.Contains(t, rejected.Error(), "§8.4.2")
	assert.Contains(t, rejected.Error(), "nothing was posted")
}

// A response whose words name a path is narrowed to the comments on it, and
// never further: GitHub identified the file, so reporting the other comment
// would send the reviewer to a position the API said nothing about.
func TestAResponseNamingAPathIsNarrowedToIt(t *testing.T) {
	rejected := Rejection(aRejectedReview(),
		`{"message":"Validation Failed","errors":["src/TaxRate.php is not part of the diff"]}`)

	require.NotNil(t, rejected)
	require.Len(t, rejected.Positions, 1)
	assert.Equal(t, "f2", rejected.Positions[0].Record)
	assert.Equal(t, "src/TaxRate.php is not part of the diff", rejected.Positions[0].Message)
	assert.Empty(t, rejected.Positions[0].Field, "a string entry names no field")
}

// A refusal carrying no `errors` array still refused the whole review, because
// §8.4.1 makes the call atomic, so every comment is named under GitHub's own
// message.
func TestARefusalWithNoErrorArrayStillNamesEveryComment(t *testing.T) {
	rejected := Rejection(aRejectedReview(), `{"message":"Not Found","status":"404"}`)

	require.NotNil(t, rejected)
	require.Len(t, rejected.Positions, 2)
	for _, position := range rejected.Positions {
		assert.Equal(t, "Not Found", position.Message)
	}
}

// §8.4.4 and not §8.4.2: a response cr cannot parse is an unknown outcome, and
// cr does not know whether the review was created. Answering "rejected" would
// assert the one thing the run failed to establish, and the caller would report
// nothing posted about a review that may be on the pull request.
func TestAnUnreadableResponseIsNoRejection(t *testing.T) {
	for _, body := range []string{
		"", "<html>502 Bad Gateway</html>", "{}", `{"documentation_url":"https://docs.github.com"}`,
	} {
		assert.Nil(t, Rejection(aRejectedReview(), body), "%q", body)
	}
}
