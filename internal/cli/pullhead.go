package cli

import (
	"fmt"
	"strings"

	"github.com/deligoez/cr/internal/git"
)

// remotePullHead is the commit the clone's remote for owner/repo names as the
// pull request's head, and empty when no remote of the clone points at owner/repo
// or the read fails. A failure says nothing: the read is a second opinion beside
// gh's, and a brief does not fail for want of one.
//
// It is a variable so the suite can fence it, as it fences gh: every brief in
// the tests runs against fixture remotes on github.com that must not be
// reached.
var remotePullHead = func(dir, owner, repo string, pr int) string {
	remotes, err := git.Remotes(dir)
	if err != nil {
		return ""
	}
	for _, remote := range remotes {
		slug, ok := githubSlug(remote.URL)
		if !ok || !strings.EqualFold(slug, owner+"/"+repo) {
			continue
		}
		head, err := git.PullHead(dir, remote.Name, pr)
		if err != nil {
			return ""
		}
		return head
	}
	return ""
}

// headLag is the disclosure for a head gh reports that the remote does not:
// both commits named, because the operator's next step is to compare them with
// the commit that was pushed. It is a disclosure and not a refusal, which would
// be a new exit-4 condition §9.3.2 does not have. Nothing when the two agree or
// the remote was not read.
func headLag(owner, repo string, pr int, reported, remote string) []string {
	if remote == "" || strings.EqualFold(remote, reported) {
		return nil
	}
	return []string{fmt.Sprintf(
		"%s/%s#%d: GitHub's API reports head %s while the remote's refs/pull/%d/head is %s; "+
			"GitHub can lag a push by a few seconds, so this brief may be of the older head. "+
			"Brief again once both name the commit that was pushed",
		owner, repo, pr, reported, pr, remote)}
}
