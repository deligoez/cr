package profile

import (
	"bytes"
	"embed"
	"path"
	"slices"
)

// shippedReleases are the releases whose profile files cr carries, oldest
// first. Each one is a directory under builtin/shipped holding the profile
// files that release's `cr init` wrote, extracted byte for byte from the
// release tag's builtin directory rather than retyped: a file on disk that
// equals one of them is a file nobody has edited.
//
// A release is added here when a later release changes a shipped profile, and
// TestEveryReleaseTagCarryingABuiltinProfileIsCovered fails until it is.
var shippedReleases = []string{"v0.1.0", "v0.2.0", "v0.2.1"}

// shipped holds the previous releases' profile files.
//
//go:embed builtin/shipped
var shipped embed.FS

// Standing is how one profile file on disk relates to the profiles cr has
// shipped under its id.
type Standing struct {
	// Current is true when the file equals the profile this build ships.
	Current bool
	// Release names the newest earlier release whose shipped file for the
	// id equals the file, and is empty when none does.
	Release string
}

// Stale reports a file that is a previous release's shipped profile and not
// this build's. It carries no edit of the user's, so replacing it with the
// current profile loses nothing.
func (s Standing) Stale() bool { return !s.Current && s.Release != "" }

// StandingOf compares content, a profile file read from disk, against the
// profile this build ships under id and against every earlier release's.
//
// A file equal to the current profile is Current whatever else it equals: a
// profile that did not change between two releases is the same bytes in both,
// and a file holding them is up to date rather than stale.
func StandingOf(id string, content []byte) Standing {
	current, ships := Builtins()[id]
	standing := Standing{Current: ships && current == string(content)}
	for _, release := range slices.Backward(shippedReleases) {
		earlier, err := shipped.ReadFile(path.Join("builtin/shipped", release, id+fileExt))
		if err != nil {
			// That release shipped no profile under this id.
			continue
		}
		if bytes.Equal(earlier, content) {
			standing.Release = release
			break
		}
	}
	return standing
}
