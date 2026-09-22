package brief

import (
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/intent"
)

// TrackerSource is source with the reader §3.1.8 gives a `github` tracker:
// the issue's title and body through client, cr's one gh door, rather than a
// configured command.
//
// The door is the point. `intent.cmd` runs with the whole environment and the
// working directory, which is what a tracker cr knows nothing about needs; a
// `gh` started that way inherits GH_HOST, GH_REPO and GH_TOKEN, exactly the
// variables internal/gh's allowlist drops so that a read cannot be redirected
// at another host's repository of the same name. The issue text is the
// authority P1 reviews the change against, and it is read the way the pull
// request itself is. Any other tracker is returned as it came.
func TrackerSource(tracker string, client gh.Client, source intent.Source) intent.Source {
	if tracker != intent.TrackerGitHub {
		return source
	}
	source.Fetch = func(key string) (string, error) {
		owner, repo, number, ok := intent.SplitGitHubKey(key)
		if !ok {
			return "", &intent.IssueFlagError{Value: key}
		}
		issue, err := client.Issue(owner, repo, number)
		if err != nil {
			return "", err
		}
		return issue.Title + "\n\n" + issue.Body, nil
	}
	return source
}

// resolveIntent is §3.2 for the tracker the configuration names: the pattern
// over the four sources for a command tracker, GitHub's link for `github`.
func resolveIntent(src *Sources, pr *gh.PullRequest) (intent.Intent, error) {
	tracker := src.Config.String(intent.TrackerSetting)
	pattern := intent.KeyPattern(tracker, src.Config.String(keyPatternSetting))
	if tracker == intent.TrackerGitHub {
		return githubIntent(src, pr, pattern)
	}
	return intent.Resolve(intent.KeySources{
		Flag:   src.IssueFlag,
		Branch: pr.HeadRefName,
		Title:  pr.Title,
		Body:   pr.Body,
	}, pattern, src.Intent)
}

// keyPatternSetting is §3.2's `intent.key_pattern`, by §2.7's key.
const keyPatternSetting = "intent.key_pattern"

// githubIntent is §3.2 for a `github` tracker: `--issue` in any spelling a
// reviewer would type, or else the one issue GitHub links the pull request to
// as closing. No pattern is matched over the branch, the title or the body,
// because a `#12` in prose is not a reference and GitHub's own link is the
// authority on what the pull request closes.
func githubIntent(src *Sources, pr *gh.PullRequest, pattern string) (intent.Intent, error) {
	source := TrackerSource(intent.TrackerGitHub, src.GH, src.Intent)
	if src.IssueFlag != "" {
		key, err := intent.GitHubIssueFlag(src.IssueFlag, src.Owner, src.Repo)
		if err != nil {
			return intent.Intent{}, err
		}
		return intent.ResolveGiven(intent.Key{Value: key, Origin: intent.KeyFromFlag}, pattern, source)
	}
	closing := make([]string, 0, len(pr.ClosingIssues))
	for _, ref := range pr.ClosingIssues {
		closing = append(closing, intent.GitHubKey(ref.Owner, ref.Repo, ref.Number))
	}
	key, err := intent.LinkedKey(closing)
	if err != nil {
		return intent.Intent{}, err
	}
	return intent.ResolveGiven(key, pattern, source)
}
