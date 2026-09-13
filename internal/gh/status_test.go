package gh

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The status a failed `gh api` names on standard error, read off the lines gh
// 2.97.0 wrote when a local server answered it with each response: a JSON
// document carrying a message, a body that is not JSON, a JSON document
// carrying no message, and a connection dropped before any response.
func TestTheStatusIsReadOffWhatGhWroteOnStandardError(t *testing.T) {
	for _, row := range []struct {
		name   string
		stderr string
		status string
	}{
		{"a validation failure", "gh: Validation Failed (HTTP 422)", "422"},
		{"a timeout body", "gh: We couldn't respond to your request in time. Sorry about that. " +
			"Please try resubmitting your request and contact us if the problem persists. (HTTP 504)", "504"},
		{"a body that is not JSON", "gh: HTTP 502", "502"},
		{"a document naming no message", "gh: a.go is not part of the diff", ""},
		{"a dropped connection", `Post "https://api.github.com/repos/acme/web/pulls/1/reviews": EOF`, ""},
		{"advice after the status", "gh: Not Found (HTTP 404)\ngh: This API operation needs the \"repo\" scope.", "404"},
		{"a message ending a line in the same words", "gh: first (HTTP 404)\nsecond (HTTP 422)", "422"},
		{"a status inside a line", "gh: HTTP 502 and more", ""},
		{"nothing written", "", ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			failed := &CommandError{Stderr: row.stderr}
			assert.Equal(t, row.status, failed.HTTPStatus())
		})
	}
}
