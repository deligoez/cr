package cli

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/deligoez/cr/internal/git"
)

// RepositoryDetectionError reports a repository under review cr could not name
// on its own, and why.
//
// §11.1 makes `--repo` the override for repository detection, so detection
// refuses rather than guesses: a pull request is addressed by owner/repo, and a
// guess would read or write another repository's state under §2.2.
type RepositoryDetectionError struct {
	// Reason is what made the remote unusable, in a clause the message
	// completes.
	Reason string
}

func (e *RepositoryDetectionError) Error() string {
	return "cannot detect the repository under review: " + e.Reason
}

// githubHost is the one host detection reads an owner/repo from.
const githubHost = "github.com"

// detectRepo derives owner/repo from the repository under review: the one
// remote it declares, when that remote is a GitHub repository.
//
// The remote is read through internal/git's pinned runner. No remote, more
// than one, and a remote on another host are each refused by name, because each
// has a different fix and none has an answer cr could pick without choosing
// which repository's state to touch.
func detectRepo() (owner, repo string, err error) {
	dir, err := repoDir()
	if err != nil {
		return "", "", err
	}
	remotes, err := git.Remotes(dir)
	if err != nil {
		return "", "", &RepositoryDetectionError{Reason: fmt.Sprintf(
			"git could not list the remotes of %s (%v)", dir, err)}
	}
	switch len(remotes) {
	case 0:
		return "", "", &RepositoryDetectionError{Reason: dir + " declares no remote"}
	case 1:
	default:
		names := make([]string, 0, len(remotes))
		for _, remote := range remotes {
			names = append(names, remote.Name)
		}
		return "", "", &RepositoryDetectionError{Reason: fmt.Sprintf(
			"%s declares %d remotes (%s), and cr does not choose between them",
			dir, len(remotes), strings.Join(names, ", "))}
	}
	owner, repo, ok := githubSlug(remotes[0].URL)
	if !ok {
		return "", "", &RepositoryDetectionError{Reason: fmt.Sprintf(
			"remote %s is %s, which is not a %s owner/repo", remotes[0].Name, remotes[0].URL, githubHost)}
	}
	return owner, repo, nil
}

// githubSlug reads owner and repo out of a GitHub remote URL in any of the
// forms git accepts: `git@github.com:owner/repo.git`, and `ssh://`, `https://`
// or `git://` URLs, with or without a user, a port or the `.git` suffix.
func githubSlug(remote string) (owner, repo string, ok bool) {
	var host, path string
	if strings.Contains(remote, "://") {
		parsed, err := url.Parse(remote)
		if err != nil {
			return "", "", false
		}
		host, path = parsed.Hostname(), parsed.Path
	} else {
		address, rest, found := strings.Cut(remote, ":")
		if !found {
			return "", "", false
		}
		_, host, _ = strings.Cut(address, "@")
		if !strings.Contains(address, "@") {
			host = address
		}
		path = rest
	}
	if !strings.EqualFold(host, githubHost) {
		return "", "", false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	owner, repo, found := strings.Cut(path, "/")
	if !found || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", false
	}
	return owner, repo, true
}
