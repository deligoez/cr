package intent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The two values of §2.7's `intent.tracker`, per §3.1.8.
const (
	// TrackerSetting is the key, in §2.7's spelling.
	TrackerSetting = "intent.tracker"
	// TrackerCommand is §3.1.1's configured command, the default.
	TrackerCommand = "command"
	// TrackerGitHub reads the issue from GitHub through cr's own gh door
	// and takes the key from GitHub's link rather than from a pattern.
	TrackerGitHub = "github"
)

// GitHubKeyPattern is the shape §3.2 gives a `github` tracker's key:
// `<owner>.<repo>#<number>`.
//
// A GitHub issue number is unique within its repository and nowhere else, and
// §2.2 keeps the context store per key and not per repository, so a bare `#12`
// would share one store between every repository's twelfth issue and carry a
// note from one into another's review. The repository therefore rides in the
// key, joined by `.` because §2.2 forbids a path separator in a key: an owner
// is letters, digits and single inner hyphens and holds no `.`, so the first
// `.` is always the boundary, while a repository name may hold further dots.
const GitHubKeyPattern = `[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?\.[A-Za-z0-9._-]+#[1-9][0-9]*`

// KeyPattern is the pattern §3.2's key is held to for tracker: the configured
// `intent.key_pattern` for a command tracker, and GitHubKeyPattern for GitHub,
// whose keys cr forms itself and no configured pattern describes.
func KeyPattern(tracker, configured string) string {
	if tracker == TrackerGitHub {
		return GitHubKeyPattern
	}
	return configured
}

// GitHubKey is the key of one issue, in GitHubKeyPattern's form.
func GitHubKey(owner, repo string, number int) string {
	return owner + "." + repo + "#" + strconv.Itoa(number)
}

// githubKeyWhole is GitHubKeyPattern anchored, with the three parts captured.
var githubKeyWhole = regexp.MustCompile(
	`^([A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)\.([A-Za-z0-9._-]+)#([1-9][0-9]*)$`)

// SplitGitHubKey reads a key GitHubKey formed back into its repository and
// number, and reports false for anything else.
func SplitGitHubKey(key string) (owner, repo string, number int, ok bool) {
	parts := githubKeyWhole.FindStringSubmatch(key)
	if parts == nil {
		return "", "", 0, false
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil {
		return "", "", 0, false
	}
	return parts[1], parts[2], n, true
}

// The spellings `--issue` may take for a `github` tracker, besides a key.
var (
	issueNumber = regexp.MustCompile(`^#?([1-9]\d*)$`)
	issueRef    = regexp.MustCompile(`^([A-Za-z0-9-]+)/([A-Za-z0-9._-]+)#([1-9]\d*)$`)
	issueURL    = regexp.MustCompile(`^https://github\.com/([A-Za-z0-9-]+)/([A-Za-z0-9._-]+)/issues/([1-9][0-9]*)/?$`)
)

// IssueFlagError reports an `--issue` a `github` tracker cannot read as an
// issue. It is left unmapped, so §11.2 codes it 2: the command line is wrong.
type IssueFlagError struct{ Value string }

func (e *IssueFlagError) Error() string {
	return fmt.Sprintf(
		"--issue %q names no GitHub issue: §3.2 reads `12`, `#12`, `owner/repo#12`, the issue's URL, "+
			"or a key of the form owner.repo#12", e.Value)
}

// GitHubIssueFlag is §3.2's first source for a `github` tracker: `--issue` in
// any spelling a reviewer would type, normalised to the key. A bare number is
// an issue of the pull request's own repository.
func GitHubIssueFlag(flag, owner, repo string) (string, error) {
	flag = strings.TrimSpace(flag)
	if _, _, _, ok := SplitGitHubKey(flag); ok {
		return flag, nil
	}
	if parts := issueNumber.FindStringSubmatch(flag); parts != nil {
		return owner + "." + repo + "#" + parts[1], nil
	}
	for _, spelling := range []*regexp.Regexp{issueRef, issueURL} {
		if parts := spelling.FindStringSubmatch(flag); parts != nil {
			return parts[1] + "." + parts[2] + "#" + parts[3], nil
		}
	}
	return "", &IssueFlagError{Value: flag}
}

// AmbiguousIssueError reports a pull request GitHub links to more than one
// closing issue, so §3.2 has no one key to take. cr does not pick (P2): which
// issue the change is reviewed against is the reviewer's to say. §11.2 codes
// it 1.
type AmbiguousIssueError struct {
	// Keys are the candidates, in GitHub's order.
	Keys []string
}

func (e *AmbiguousIssueError) Error() string {
	return fmt.Sprintf(
		"the pull request closes %d issues (%s), and §3.2 reviews a change against one: "+
			"cr does not choose between them", len(e.Keys), strings.Join(e.Keys, ", "))
}

// Hint is §12.4's next actionable step.
func (e *AmbiguousIssueError) Hint() string {
	flags := make([]string, 0, len(e.Keys))
	for _, key := range e.Keys {
		flags = append(flags, "--issue "+key)
	}
	return "pass --issue naming the one to review against: " + strings.Join(flags, " or ")
}

// LinkedKey is §3.2's second source for a `github` tracker: the one issue
// GitHub links the pull request to as closing, or no key when it links none.
func LinkedKey(closing []string) (Key, error) {
	switch len(closing) {
	case 0:
		return Key{}, nil
	case 1:
		return Key{Value: closing[0], Origin: KeyFromClosingReference}, nil
	}
	return Key{}, &AmbiguousIssueError{Keys: closing}
}

// ResolveGiven is Resolve for a key found by some other rule than §3.2's
// pattern: the issue text is read for it the same way, and an absent key is
// §3.2's empty intent.
func ResolveGiven(key Key, pattern string, source Source) (Intent, error) {
	if key.Origin == KeyAbsent {
		return Intent{Pattern: pattern}, nil
	}
	reading, err := Read(source, key.Value)
	if err != nil {
		return Intent{}, err
	}
	return Intent{Key: key, Text: reading.Text, Pattern: pattern, read: &reading}, nil
}
