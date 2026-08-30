package intent

import "fmt"

// NoIssueKeyError reports a pull request that resolved to no issue key, raised
// by a command that cannot do its work without one.
//
// §3.2 leaves the key empty when none of its four sources yields a match and
// has the run continue with an empty intent, so this is recorded state rather
// than an unusable file or a mistyped command line: §11.2 codes it 1.
//
// Two separate things fail without a key, and either alone is enough to refuse.
// §3.3 forms every claim id as `<ISSUE-KEY>#c<n>`, so there is no id a claim
// could carry that would name this pull request's issue. And §3.1.1 puts a
// `{key}` placeholder in the tracker command, so reading the issue text for an
// empty key would run that command with the placeholder substituted to nothing
// — Resolve avoids exactly that by never reaching the source without a key, and
// a command that read the key out of §2.3's metadata instead has to refuse in
// the same place.
type NoIssueKeyError struct {
	// Owner, Repo, and PR name the pull request that resolved to no key.
	Owner string
	Repo  string
	PR    int
}

func (e *NoIssueKeyError) Error() string {
	return fmt.Sprintf(
		"%s/%s#%d resolved to no issue key, so §3.3 has no issue to draw claims from "+
			"and §3.1.1 has no key to read the issue text for: "+
			"re-run `cr brief %d --issue <KEY>` to name one",
		e.Owner, e.Repo, e.PR, e.PR,
	)
}
