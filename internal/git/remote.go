package git

import (
	"fmt"
	"strings"
)

// Remote is one remote the repository under review declares, by the URL it
// fetches from.
type Remote struct {
	// Name is the remote's name, such as `origin`.
	Name string
	// URL is the URL git fetches the remote from.
	URL string
}

// Remotes returns every remote the repository at dir declares, once each, in
// the order git lists them.
//
// It reads `git remote -v`, which prints a fetch and a push line per remote;
// the fetch URL is the one kept, since it names the repository the checkout
// came from. A repository with no remote returns an empty list rather than an
// error, so the caller can say which of the two it was.
func Remotes(dir string) ([]Remote, error) {
	listing, err := run(dir, "remote", "-v")
	if err != nil {
		return nil, err
	}
	return parseRemotes(listing)
}

// PullHead returns the commit the remote's `refs/pull/<pr>/head` names, read
// with `git ls-remote`, and an empty string when the remote holds no such ref.
//
// It is a network read and writes nothing, not even the clone's refs. It is
// read beside gh's answer because the two can disagree: measured 2026-09-22 on
// deligoez/cr-qa, GitHub's API reported the pre-push head for a few seconds
// after a push while the pushed commit was already on the remote.
func PullHead(dir, remote string, pr int) (string, error) {
	out, err := run(dir, "ls-remote", "--end-of-options", remote, fmt.Sprintf("refs/pull/%d/head", pr))
	if err != nil {
		return "", err
	}
	sha, _, _ := strings.Cut(strings.TrimSpace(out), "\t")
	return sha, nil
}

// parseRemotes reads `git remote -v` lines: `<name>\t<url> (fetch|push)`.
func parseRemotes(listing string) ([]Remote, error) {
	remotes := make([]Remote, 0)
	for line := range strings.SplitSeq(strings.TrimSuffix(listing, "\n"), "\n") {
		if line == "" {
			continue
		}
		name, rest, ok := strings.Cut(line, "\t")
		url, kind, spaced := strings.Cut(rest, " ")
		if !ok || !spaced {
			return nil, fmt.Errorf("git remote -v wrote %q, which is not a name, a URL and a direction", line)
		}
		if kind == "(fetch)" {
			remotes = append(remotes, Remote{Name: name, URL: url})
		}
	}
	return remotes, nil
}
