package gh

import (
	"regexp"
)

// statusLine is the HTTP status as a failed `gh api` names it on standard
// error. Measured against gh 2.97.0 answering from a local server: a JSON
// response carrying a message gives `gh: <message> (HTTP 504)`, a response that
// is not JSON gives `gh: HTTP 502`, and a JSON response carrying no message
// gives its error strings with no status at all. A connection that drops
// before any response gives Go's transport error, which names none either.
var statusLine = regexp.MustCompile(`(?m)(?:^gh: HTTP (\d{3})| \(HTTP (\d{3})\))$`)

// HTTPStatus is the HTTP status a failed `gh api` call reported on standard
// error, and "" when it reported none.
//
// The last status named is the one returned: gh appends it after GitHub's
// message, so a message that itself ends a line in the same words comes first.
func (e *CommandError) HTTPStatus() string {
	named := statusLine.FindAllStringSubmatch(e.Stderr, -1)
	if len(named) == 0 {
		return ""
	}
	last := named[len(named)-1]
	return last[1] + last[2]
}
