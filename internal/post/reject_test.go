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
		`"message":"line must be part of the diff"}],"status":"422"}`, "")

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
		`{"message":"Validation Failed","errors":["src/TaxRate.php is not part of the diff"]}`, "422")

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
	rejected := Rejection(aRejectedReview(), `{"message":"Not Found","status":"404"}`, "")

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
		assert.Nil(t, Rejection(aRejectedReview(), body, "422"), "%q", body)
	}
}

// A refusal naming no position still renders GitHub's message. A review carrying
// no comment is the input that gives one, and it is the input where a buffer
// sized one short of the positions would have a negative capacity, which `make`
// panics on instead of reporting the refusal.
func TestARefusalNamingNoPositionStillRendersItsMessage(t *testing.T) {
	rejected := Rejection(Build(nil, nil), `{"message":"Unprocessable Entity","status":"422"}`, "")

	require.NotNil(t, rejected)
	require.Empty(t, rejected.Positions)
	require.NotPanics(t, func() {
		assert.Equal(t,
			"§8.4.2: GitHub rejected the review, so nothing was posted: Unprocessable Entity (HTTP 422)",
			rejected.Error())
	})
}

// The HTTP status is named when GitHub answered with one and left out when it did
// not, so a response carrying none never reads as a status cr made up.
func TestTheRefusalNamesTheStatusOnlyWhenGitHubGaveOne(t *testing.T) {
	with := &RejectedError{Message: "Validation Failed", Status: "422"}
	assert.Contains(t, with.Error(), "Validation Failed (HTTP 422)")

	without := &RejectedError{Message: "Validation Failed"}
	assert.NotContains(t, without.Error(), "HTTP")
}

// §8.4.2's rejection is a response stating the review was not created, and
// only a client error states that. A server error, a timeout and a response
// naming no status leave cr not knowing whether the review exists, which is
// §8.4.4's unknown outcome, so each answers nil. The status gh reported on the
// transport is read first and the document's own field only in its absence.
func TestOnlyAClientErrorIsARejection(t *testing.T) {
	const timeout = `We couldn't respond to your request in time. Sorry about that.`
	for _, row := range []struct {
		name     string
		stated   string
		body     string
		refusing string
	}{
		{"a 422 the transport named", "422", `{"message":"Validation Failed"}`, "422"},
		{"a 422 only the document named", "", `{"message":"Validation Failed","status":"422"}`, "422"},
		{"the lowest client error", "400", `{"message":"Bad Request"}`, "400"},
		{"the highest client error", "499", `{"message":"Client Closed"}`, "499"},
		{"the transport over the document", "422", `{"message":"Validation Failed","status":"504"}`, "422"},
		{"a 504 timeout body", "504", `{"message":"` + timeout + `","status":"504"}`, ""},
		{"a 504 only the document named", "", `{"message":"` + timeout + `","status":"504"}`, ""},
		{"a 502 server error", "502", `{"message":"Server Error"}`, ""},
		{"the lowest server error", "500", `{"message":"Internal Server Error"}`, ""},
		{"the highest informational status", "399", `{"message":"Unknown"}`, ""},
		{"a 408 request timeout", "408", `{"message":"Request Timeout"}`, ""},
		{"no status at all", "", `{"message":"Validation Failed"}`, ""},
		{"a status that is not a number", "", `{"message":"Validation Failed","status":"four"}`, ""},
		{"a transport 504 over a document 422", "504", `{"message":"Validation Failed","status":"422"}`, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			rejected := Rejection(aRejectedReview(), row.body, row.stated)
			if row.refusing == "" {
				assert.Nil(t, rejected, "§8.4.4: this response does not say the review was not created")
				return
			}
			require.NotNil(t, rejected, "§8.4.2: a client error is GitHub refusing the review")
			assert.Equal(t, row.refusing, rejected.Status)
		})
	}
}

// Lines names a range as `33-35` and a single line as `35`, and a start that is
// no earlier than the line names that one line rather than a range of one.
//
// gremlins found both halves open: every refusal fixture's comment was on one
// line, which Build leaves without a start line, so nothing printed a range.
func TestLinesNamesARangeByBothEndsAndOneLineByItself(t *testing.T) {
	assert.Equal(t, "33-35", Lines(33, 35))
	assert.Equal(t, "35", Lines(0, 35))
	assert.Equal(t, "35", Lines(35, 35))
}
