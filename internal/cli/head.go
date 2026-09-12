package cli

import (
	"github.com/deligoez/cr/internal/state"
)

// currentHead reads the head GitHub reports for one pull request.
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
var currentHead = githubHead

// githubHead is §9.3.1's current head: GitHub's `headRefOid` for the pull
// request, which gh/pr.go argues is the only honest answer to "the current
// head" — a local branch of the same name may sit anywhere, and on a fork it
// names a different history altogether.
func githubHead(owner, repo string, pr int) (string, error) {
	opened, err := ghClient().PullRequest(owner, repo, pr)
	if err != nil {
		return "", err
	}
	return opened.Head, nil
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
func briefedRound(l state.Layout, owner, repo string, pr int) (state.Round, error) {
	return l.Briefed(owner, repo, pr, func() (string, error) {
		return currentHead(owner, repo, pr)
	})
}
