package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/gh"
)

// closureDisclosure is a pull request that is no longer open, as `cr brief`,
// `cr status` and `cr post` tell their reader before anything else: the state
// and the time GitHub reports. It is nothing for an open pull request, and for
// one whose state no read of GitHub answered.
//
// It warns and never refuses. A review of a merged pull request can still be
// wanted, and whether to send one stays the reviewer's decision at `--confirm`;
// what cr owes that decision is that it is not taken without being told.
func closureDisclosure(owner, repo string, pr int, opened *gh.PullRequest) []string {
	state, at, closed := opened.Closure()
	if !closed {
		return nil
	}
	if at == "" {
		at = "a time GitHub did not report"
	}
	return []string{fmt.Sprintf(
		"%s/%s#%d is %s: GitHub reports it %s at %s, so a review posted now reaches a pull request "+
			"that is no longer open; cr does not refuse the post, and whether to send it stays --confirm's",
		owner, repo, pr, state, state, at,
	)}
}
