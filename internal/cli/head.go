package cli

import (
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// currentPullRequest reads the pull request as GitHub reports it now: the head
// §9.3.1 compares, and the state a closed or merged pull request is disclosed
// with.
//
// It is a package-level variable for the reason ghClient and repoDir are: the
// §9.3.1 comparison now happens on every command that reads per-PR state, so
// every test of every such command would otherwise have to stand up a GitHub
// answer to exercise code that has nothing to do with the head. crHome installs
// a default that echoes the round's recorded head, which is the not-stale case,
// and a test that is about §9.3 replaces it with a head of its own.
//
// It does not widen what cr can do. gh.Client is the read client and §2.1.2's
// boundary refuses a write whichever runner is behind it.
var currentPullRequest = githubPullRequest

// githubPullRequest is GitHub's answer about the pull request. Its head,
// `headRefOid`, is §9.3.1's current head, which gh/pr.go argues is the only
// honest answer to "the current head" — a local branch of the same name may sit
// anywhere, and on a fork it names a different history altogether.
func githubPullRequest(owner, repo string, pr int) (gh.PullRequest, error) {
	return ghClient().PullRequest(owner, repo, pr)
}

// briefedRound is internal/cli's one call of state.Layout.Briefed: the round
// `cr brief` recorded, read beside §9.3.1's comparison against the current
// head.
//
// Every command under internal/cli that reads §2.3's per-round state comes
// through here, which is what makes "on every command" a property of the tree
// rather than a list somebody keeps. TestBriefedIsReachedThroughOneDoor reads
// the callers out of the source and holds it to that, and
// TestEveryCommandHoldingARoundAnswersSection93 requires each of them to say
// what it does with the answer — refuse under §9.3.2, or disclose under §9.3.1.
//
// A caller that also needs the pull request's state passes opened, which is
// filled from the same read the head came from, so a command reports the state
// of the pull request whose head it compared rather than asking GitHub twice.
func briefedRound(l state.Layout, owner, repo string, pr int, opened ...*gh.PullRequest) (state.Round, error) {
	return l.Briefed(owner, repo, pr, func() (string, error) {
		read, err := currentPullRequest(owner, repo, pr)
		for _, into := range opened {
			*into = read
		}
		return read.Head, err
	})
}
