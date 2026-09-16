package role

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// shippedReleases are the releases whose role files cr carries, oldest first.
// Each one is a directory under builtin/shipped holding the role files that
// release's `cr init --eject-roles` wrote, extracted byte for byte from the
// release tag's builtin directory rather than retyped: a file on disk that
// equals one of them is a file nobody has edited.
//
// A release's file is embedded only when it differs from the role this build
// ships. TestEveryReleaseTagCarryingABuiltinRoleIsCovered proves that an
// unembedded one equals the current built-in, and StandingOf answers Current
// for a file holding those bytes — so embedding a byte-identical copy would
// add a second name for the same answer. That test fails until a release whose
// role text later changed is added here.
var shippedReleases = []string{"v0.1.0", "v0.2.0", "v0.2.1", "v0.2.2"}

// shippedDirName is the directory inside builtin/ holding one subdirectory per
// release, and shippedDir is the same directory as the embedded filesystem
// names it. Both are spelled once so the embed directive, the reads below and
// the guard over builtin/'s own listing cannot drift apart.
const (
	shippedDirName = "shipped"
	shippedDir     = "builtin/" + shippedDirName
)

// shipped holds the previous releases' role files.
//
//go:embed builtin/shipped
var shipped embed.FS

// Standing is how one role file on disk relates to the roles cr has shipped
// under its id.
type Standing struct {
	// Current is true when the file equals the role this build ships.
	Current bool
	// Release names the newest earlier release whose shipped file for the
	// id equals the file, and is empty when none does.
	Release string
}

// Stale reports a file that is a previous release's shipped role and not this
// build's. It carries no edit of the user's, so replacing it with the current
// role loses nothing, which is what §2.5.2 has `cr init` do.
func (s Standing) Stale() bool { return !s.Current && s.Release != "" }

// StandingOf compares content, a role file read from disk, against the role
// this build ships under id and against every earlier release's.
//
// A file equal to the current role is Current whatever else it equals: a role
// that did not change between two releases is the same bytes in both, and a
// file holding them is up to date rather than stale.
func StandingOf(id string, content []byte) Standing {
	current, ships := Builtins()[id]
	standing := Standing{Current: ships && current == string(content)}
	for _, release := range slices.Backward(shippedReleases) {
		earlier, err := shipped.ReadFile(path.Join(shippedDir, release, id+fileExt))
		if err != nil {
			// That release shipped no role under this id, or shipped
			// the one this build ships.
			continue
		}
		if bytes.Equal(earlier, content) {
			standing.Release = release
			break
		}
	}
	return standing
}

// staleNotice is §2.5.2's honesty sentence for the role file at file when its
// content is an earlier release's shipped role, and empty for every other file:
// one equal to this build's role, one somebody edited, and one under an id cr
// never shipped.
//
// It names the release and `cr init`, which is what §2.5.2 requires of the
// report, and what the shipped role changed since, because the reader's
// question is whether the difference matters to this round: a role's
// `instructions` are carried into every prompt §4.6.1 emits for it, so a
// reviewer reading an earlier release's framing is judging by an earlier
// release's standard without being told.
func staleNotice(file string, content []byte) string {
	id := strings.TrimSuffix(filepath.Base(file), fileExt)
	standing := StandingOf(id, content)
	if !standing.Stale() {
		return ""
	}
	since := "has changed since"
	if changed := changedFields(content, []byte(Builtins()[id])); len(changed) > 0 {
		since = "has since changed " + strings.Join(changed, ", ")
	}
	return fmt.Sprintf("%s is the %s role cr %s shipped, unedited, and the shipped role %s; "+
		"cr init updates the file to it, and until then every prompt cr emits for this role "+
		"carries the earlier release's framing", file, id, standing.Release, since)
}

// changedFields names, sorted, every field whose value differs between two role
// documents. §2.5's table is flat — every row is a scalar or a list of them —
// so a top-level comparison names everything a role file can hold, and a list
// that gained an entry is named once as the list. A document that does not
// decode names nothing.
func changedFields(before, after []byte) []string {
	var was, is map[string]any
	if json.Unmarshal(before, &was) != nil || json.Unmarshal(after, &is) != nil {
		return nil
	}
	changed := make([]string, 0)
	for _, key := range slices.Sorted(maps.Keys(union(was, is))) {
		if !reflect.DeepEqual(was[key], is[key]) {
			changed = append(changed, key)
		}
	}
	return changed
}

// union is the set of keys either document carries, so a key on one side only
// is compared rather than passed over.
func union(was, is map[string]any) map[string]struct{} {
	keys := make(map[string]struct{}, len(was)+len(is))
	for key := range was {
		keys[key] = struct{}{}
	}
	for key := range is {
		keys[key] = struct{}{}
	}
	return keys
}

// StaleDisclosures are the honesty sentences §2.5.2 owes the reader of a
// command that resolved this corpus: one per role file that is, byte for byte,
// a role an earlier release shipped and not this build's, naming that release
// and `cr init`.
//
// They arrive in §2.5.5's corpus order, which is the order the roles themselves
// are reported in, and a built-in role contributes none: §2.5.2's report is
// about an *ejected* file, and the roles inside the binary are this build's by
// construction.
func StaleDisclosures(corpus []Resolved) []string {
	sentences := make([]string, 0, len(corpus))
	for i := range corpus {
		if corpus[i].stale != "" {
			sentences = append(sentences, corpus[i].stale)
		}
	}
	return sentences
}
