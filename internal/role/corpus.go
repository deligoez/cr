package role

import (
	"cmp"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// fileExt is the suffix of a role file in a roles directory, and the extension
// §2.5 strips to get the file stem the id must equal.
const fileExt = ".json"

// Layer is one of the three places a role can be resolved from. The constants
// are declared in §2.5.4's resolution order, highest precedence first, and
// §2.5.5 orders the corpus by that same sequence, so their order is the
// contract rather than a convenience.
type Layer int

const (
	// RepoLayer is a per-repository role at
	// ~/.cr/repos/<owner>/<repo>/roles/<id>.json.
	RepoLayer Layer = iota
	// GlobalLayer is a global role at ~/.cr/roles/<id>.json.
	GlobalLayer
	// BuiltinLayer is a role file cr ships inside the binary.
	BuiltinLayer
)

// Resolved is one role of the corpus together with the layer §2.5.4 resolved it
// from. The layer is carried rather than recomputed, because §2.5.5 orders the
// corpus by it and a caller holding only the Role could not tell a
// per-repository override from the built-in it replaced.
type Resolved struct {
	// Role is the winning role file, whole.
	Role Role
	// Layer is where that file came from.
	Layer Layer
}

// Resolve returns the role corpus for one repository, in the order §2.5.5
// fixes: resolution layer first, per-repository before global before built-in,
// then ascending lexicographic role id.
//
// repoRolesDir and globalRolesDir are the two on-disk layers of §2.2; the third
// is Builtins(). Either directory may be absent, which is the ordinary case
// rather than a fault — the per-repository one exists only once a repository
// has been seen, and neither is needed for a run that has customised nothing.
//
// Every file in both directories is read and validated before anything is
// resolved, including a file a higher layer will shadow. §2.5.3 aborts on a
// malformed role file without qualifying which one, and skipping the ones that
// would lose is exactly the silence that abort exists to prevent: an
// unparseable per-repository role passed over would hand the user the global
// role while they believe their override is in force, and cr would never say
// so.
func Resolve(repoRolesDir, globalRolesDir string) ([]Resolved, error) {
	repo, err := loadDir(repoRolesDir)
	if err != nil {
		return nil, err
	}
	global, err := loadDir(globalRolesDir)
	if err != nil {
		return nil, err
	}
	builtin, err := parseAll(Builtins())
	if err != nil {
		return nil, err
	}

	// Every role read is a candidate and at most one entry, so the three
	// layers' total is the exact upper bound for both.
	total := len(repo) + len(global) + len(builtin)
	c := corpus{
		taken: make(map[string]struct{}, total),
		roles: make([]Resolved, 0, total),
	}
	c.add(RepoLayer, repo)
	c.add(GlobalLayer, global)
	c.add(BuiltinLayer, builtin)
	return c.roles, nil
}

// Order returns §2.5.5's corpus order as a comparison over role ids, which is
// what §6.4.2 reads to pick a duplicate group's representative: "the earliest
// role in corpus order". The shape is cmp's, so slices.MinFunc, slices.SortFunc
// and cmp.Or take it as it stands.
//
// It reads the positions out of the corpus Resolve produced rather than sorting
// by layer and then by id a second time. So there is one definition of §2.5.5's
// order in cr — the sequence add appends — and a comparison derived from it
// cannot drift from it. The other half is
// TestNothingOutsideThisPackageCanNameTheLayerARoleResolvedFrom: outside this
// package the layer a role resolved from cannot be named, so no caller can
// rebuild the order instead of asking for it.
//
// A role id no layer resolved sorts after every resolved one, ties broken by
// id. Unknown is deliberately not rank zero: a bare map of positions read with
// the zero value would make an id nobody resolved the earliest role of every
// group, and §6.4.2 would hand the representative to a role that never looked
// at the code.
func Order(corpus []Resolved) func(a, b string) int {
	rank := make(map[string]int, len(corpus))
	for at, resolved := range corpus {
		rank[resolved.Role.ID] = at
	}
	// One past the last resolved role, so an unresolved id sits behind the
	// whole corpus and beside the other unresolved ones.
	unresolved := len(corpus)
	rankOf := func(id string) int {
		if at, known := rank[id]; known {
			return at
		}
		return unresolved
	}
	return func(a, b string) int {
		if order := cmp.Compare(rankOf(a), rankOf(b)); order != 0 {
			return order
		}
		return strings.Compare(a, b)
	}
}

// corpus accumulates the resolved roles in §2.5.5's order.
//
// It is the structural half of "the per-repository copy wins whole". A role
// reaches the corpus only through add, which appends one parsed Role entire or
// discards it entire; no field of any role is read anywhere in resolution, so
// there is nowhere for a per-repository role that omits `focus` to inherit the
// global role's. That is the difference from §2.7, where the configuration
// layers merge key by key: §2.5.4 resolves by file, and a resolver that built a
// Role field by field could half-merge two layers while still passing every
// test written about which layer wins.
type corpus struct {
	// taken holds the ids some layer has already resolved.
	taken map[string]struct{}
	// roles is the corpus so far, in §2.5.5's order.
	roles []Resolved
}

// add appends every role of one layer whose id no higher layer resolved first.
// The roles arrive sorted by id, so appending layer by layer produces §2.5.5's
// order without a second sort.
//
// An id present at two layers appears once, at the position of the layer that
// won it, because that is the only layer it was resolved from: §2.5.4 leaves
// the shadowed file out of the corpus rather than demoting it, so there is no
// second entry to place.
func (c *corpus) add(layer Layer, roles []Role) {
	for _, r := range roles {
		if _, shadowed := c.taken[r.ID]; shadowed {
			continue
		}
		c.taken[r.ID] = struct{}{}
		c.roles = append(c.roles, Resolved{Role: r, Layer: layer})
	}
}

// loadDir reads and validates every role file in dir, ascending by role id.
//
// A directory that does not exist holds no role, exactly like an empty one.
// Anything else that stops the listing is reported as a malformed layer, so a
// roles directory cr cannot read aborts with the code §2.5.3 gives an
// unreadable role rather than resolving to the layer below it.
//
// Entries that are not role files — a subdirectory, an editor's backup, a
// README — are skipped rather than rejected, because §2.2 fixes the role file
// name as <id>.json and says nothing about what else may sit beside it.
func loadDir(dir string) ([]Role, error) {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, &MalformedError{File: dir, Problem: "cannot be listed: " + err.Error()}
	}
	roles := make([]Role, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), fileExt) {
			continue
		}
		r, err := Load(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	sortByID(roles)
	return roles, nil
}

// parseAll validates a set of role files held in memory, keyed by id, and
// returns them ascending by id. Builtins() is the caller: the shipped roles go
// through the same loader a user's file does, so a default that would abort per
// §2.5.3 is caught rather than trusted unread, and they are parsed under the
// file name `cr init --eject-roles` writes them as, which §2.5 requires to
// equal the id.
func parseAll(files map[string]string) ([]Role, error) {
	roles := make([]Role, 0, len(files))
	for _, id := range slices.Sorted(maps.Keys(files)) {
		r, err := Parse(id+fileExt, []byte(files[id]))
		if err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, nil
}

// sortByID puts roles into the ascending lexicographic id order §2.5.5 wants.
//
// It is by id and not by file name, which are not the same order: `a.json`
// sorts before `a-b.json` because `.` is above `-`, while §2.5.5 puts `a`
// before `a-b`. os.ReadDir hands back file-name order, so relying on it would
// order most corpora right and one shape of id wrong.
func sortByID(roles []Role) {
	slices.SortFunc(roles, func(a, b Role) int { return strings.Compare(a.ID, b.ID) })
}
