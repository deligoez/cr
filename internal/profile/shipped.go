package profile

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

// staleNotice is the honesty sentence for the profile file at file when its
// content is an earlier release's shipped profile, and empty for every other
// file: one equal to this build's profile, one somebody edited, and one under an
// id cr never shipped.
//
// It names what the shipped profile changed since, by the dotted field paths
// whose values differ, because the reader's question is whether the difference
// matters to this run: laravel-pest's `sandbox.copy` gaining `.env.testing` is
// the difference between a suite reaching the test database and one reaching
// the developer's own.
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
	return fmt.Sprintf("%s is the %s profile cr %s shipped, unedited, and the shipped profile %s; "+
		"cr init updates the file to it, and the next cr test or cr probe run then recreates a sandbox "+
		"lacking a file it copies", file, id, standing.Release, since)
}

// changedFields names, in dotted spelling and sorted, every field whose value
// differs between two profile documents. An object is descended into and
// anything else is compared whole, so a list that gained an entry is named once
// as the list. A document that does not decode names nothing.
func changedFields(before, after []byte) []string {
	var was, is any
	if json.Unmarshal(before, &was) != nil || json.Unmarshal(after, &is) != nil {
		return nil
	}
	changed := make([]string, 0)
	compareField("", was, is, &changed)
	slices.Sort(changed)
	return changed
}

// compareField appends to changed the dotted path of every leaf under name
// whose value differs between was and is.
func compareField(name string, was, is any, changed *[]string) {
	wasObject, wasIsObject := was.(map[string]any)
	isObject, isIsObject := is.(map[string]any)
	if !wasIsObject || !isIsObject {
		if !reflect.DeepEqual(was, is) {
			*changed = append(*changed, name)
		}
		return
	}
	keys := slices.Collect(maps.Keys(wasObject))
	for key := range isObject {
		if _, both := wasObject[key]; !both {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		field := key
		if name != "" {
			field = name + "." + key
		}
		compareField(field, wasObject[key], isObject[key], changed)
	}
}
