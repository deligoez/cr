package finding

import (
	"path/filepath"
	"strings"
)

// The §4.6.2 fan-out output path, and the role it binds a file's records to.
//
// §4.6.2 requires every prompt to name the NDJSON path its role writes to, but
// fixes no form for that path. Without one, `role` is a field the agent writes
// and `cr` has nothing to check it against. §6.1's `axis` is derived from it,
// and §6.2 grades a record `cited` only when its axis is not `test`, so a
// test-adequacy finding writing `role: correctness` clears a bar §4.4.2 put
// there on purpose: a test finding asserts with an experiment, never with a
// citation to a test file.
//
// Pinning the path closes that. One role writes one file, the file's name is
// the role, and §6.1.3's rejection compares the record's own field against the
// name the record arrived in rather than believing it. The binding holds at
// both ends only if `cr review` emits this same path, which is why the form
// lives here rather than inside either command.
const (
	fanOutPrefix = "review-"
	fanOutSuffix = ".ndjson"
)

// FanOutFile is the file §4.6.2 has one role write its records to.
func FanOutFile(role string) string {
	return fanOutPrefix + role + fanOutSuffix
}

// RoleForFile returns the role a file's records are attributable to, and
// reports whether the name binds them to one at all.
//
// Only the base name decides, so a role may write into any directory. A name
// that is not a fan-out file — `cr merge`'s own output among them — binds its
// records to no role and is reported as such rather than guessed at.
func RoleForFile(path string) (string, bool) {
	role, found := strings.CutPrefix(filepath.Base(path), fanOutPrefix)
	if !found {
		return "", false
	}
	role, found = strings.CutSuffix(role, fanOutSuffix)
	if !found || role == "" {
		return "", false
	}
	return role, true
}
