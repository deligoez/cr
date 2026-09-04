package rule

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// fileExt is the suffix of a rule file in a rules directory, and the extension
// §2.6 strips to get the file stem the id must equal.
const fileExt = ".json"

// Source is one of the three places §2.6 item 1 resolves a rule from. The
// constants are declared in that item's order, highest precedence first, so
// their sequence is the contract rather than a convenience.
//
// §2.6 calls these layers, and so does every comment below. The identifiers do
// not: internal/role fences `Layer`, `RepoLayer`, `GlobalLayer` and
// `BuiltinLayer` to itself, so that §6.4.2 cannot rebuild §2.5.5's corpus order
// instead of calling role.Order. That fence reads identifiers rather than
// types, so a rule layer spelled with those names would trip it — and this is a
// different thing wearing the same word, not the role layer leaking out.
type Source int

const (
	// RepoSource is a per-repository rule file at
	// ~/.cr/repos/<owner>/<repo>/rules/<id>.json (§2.2).
	RepoSource Source = iota
	// GlobalSource is a global rule file at ~/.cr/rules/<id>.json (§2.2).
	GlobalSource
	// ProfileSource is a rule object in the resolved profile's `rules`
	// array (§2.4).
	ProfileSource
)

// Resolved is one rule of the corpus together with the layer §2.6 item 1
// resolved it from. The layer is carried rather than recomputed, because a
// caller holding only the Rule could not tell a per-repository override from
// the profile rule it replaced — and §2.6.3's pruning has to be able to say
// which file a dead rule is written in.
type Resolved struct {
	// Rule is the winning rule, whole.
	Rule Rule
	// Source is where that rule came from.
	Source Source
	// Path is the file it was read out of: the rule file itself for the two
	// on-disk layers, and the profile file for a rule embedded in one.
	//
	// It is carried because the answers §2.6 owes a user are all files to
	// open. §2.6.3's pruning has to say where a dead rule is written, and
	// §2.6.1.2's abort has to name the file holding the pattern that would
	// not compile. Neither Source nor the id is a file: two layers can spell
	// one id, and a profile's array has no file name of its own.
	Path string
}

// Resolve returns the rule corpus for one repository, in §2.6 item 1's order:
// per-repository first, then global, then the resolved profile's `rules` array,
// and ascending by rule id inside each layer.
//
// repoRulesDir and globalRulesDir are the two on-disk layers of §2.2. Either may
// be absent, which is the ordinary case rather than a fault — the
// per-repository directory exists only once a repository has been seen, and
// neither is needed by a run that has customised nothing. profileRules is the
// third layer, carried verbatim by §2.4's `rules` array, and profilePath is the
// profile file that carries it, which every fault found in one of those objects
// is reported against.
//
// Every rule of every layer is read and validated before anything is resolved,
// including one a higher layer will shadow. §2.6.5 aborts on a malformed rule
// without qualifying which one, and skipping the ones that would lose is
// exactly the silence that abort exists to prevent: an unparseable
// per-repository rule passed over would hand the user the global rule while
// they believe their own standard is the one being enforced, and cr would never
// say so.
//
// The order inside a layer is ascending by id rather than the order the files
// or the array happen to be in. §2.6 fixes no corpus order the way §2.5.5 does
// for roles, but §2.1.1 requires the same inputs to give the same result, and a
// directory listing is the operating system's order rather than the rule set's.
func Resolve(repoRulesDir, globalRulesDir, profilePath string, profileRules []json.RawMessage) ([]Resolved, error) {
	repo, err := loadDir(repoRulesDir)
	if err != nil {
		return nil, err
	}
	global, err := loadDir(globalRulesDir)
	if err != nil {
		return nil, err
	}
	shipped, err := parseProfileRules(profilePath, profileRules)
	if err != nil {
		return nil, err
	}

	// Every rule read is a candidate and at most one entry, so the three
	// layers' total is the exact upper bound for both.
	total := len(repo) + len(global) + len(shipped)
	c := corpus{
		taken: make(map[string]struct{}, total),
		rules: make([]Resolved, 0, total),
	}
	c.add(RepoSource, repo)
	c.add(GlobalSource, global)
	c.add(ProfileSource, shipped)
	return c.rules, nil
}

// corpus accumulates the resolved rules in §2.6 item 1's order.
//
// It is the structural half of §2.6 item 2's "overridden whole, never merged
// field by field". A rule reaches the corpus only through add, which appends
// one parsed Rule entire or discards it entire; no field of any rule is read
// anywhere in resolution, so there is nowhere for a per-repository rule that
// omits `globs` to inherit the global rule's, and nowhere for one that omits
// `severity` to inherit anything but §2.6's own default. That is the difference
// from §2.7, where the configuration layers merge key by key: a resolver that
// built a Rule field by field could half-merge two layers while still passing
// every test written about which layer wins.
//
// The distinction is not academic for a rule. A `detect` block inherited from a
// layer the user thought they had replaced would run a pattern they did not
// write, against paths they did not name, and every hit would carry the id of
// the rule they did write.
type corpus struct {
	// taken holds the ids some layer has already resolved.
	taken map[string]struct{}
	// rules is the corpus so far, in §2.6 item 1's order.
	rules []Resolved
}

// add appends every rule of one layer whose id no higher layer resolved first.
// The rules arrive sorted by id, so appending layer by layer produces the
// corpus order without a second sort.
//
// An id present at two layers appears once, at the position of the layer that
// won it. §2.6 item 2 makes the id unique after resolution, so the shadowed
// rule is left out of the corpus rather than demoted, and there is no second
// entry to place.
func (c *corpus) add(source Source, rules []Resolved) {
	for at := range rules {
		if _, shadowed := c.taken[rules[at].Rule.ID]; shadowed {
			continue
		}
		c.taken[rules[at].Rule.ID] = struct{}{}
		resolved := rules[at]
		resolved.Source = source
		c.rules = append(c.rules, resolved)
	}
}

// loadDir reads and validates every rule file in dir, ascending by rule id.
//
// A directory that does not exist holds no rule, exactly like an empty one.
// Anything else that stops the listing is reported as a malformed layer, so a
// rules directory cr cannot read aborts with the code §2.6.5 gives an unusable
// rule rather than resolving to the layer below it.
//
// Entries that are not rule files — a subdirectory, an editor's backup, a
// README — are skipped rather than rejected, because §2.2 fixes the rule file
// name as <id>.json and says nothing about what else may sit beside it.
func loadDir(dir string) ([]Resolved, error) {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, &MalformedError{File: dir, Problem: "cannot be listed: " + err.Error()}
	}
	rules := make([]Resolved, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), fileExt) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		r, err := Load(path)
		if err != nil {
			return nil, err
		}
		rules = append(rules, Resolved{Rule: r, Path: path})
	}
	sortByID(rules)
	return rules, nil
}

// parseProfileRules validates the rule objects a profile ships in its `rules`
// array (§2.4) and returns them ascending by id.
//
// They go through the same parse a rule file does, minus the file-stem equality
// they have no file for. §2.6 item 5 covers them too: a profile carrying an
// unusable rule aborts on the same code as a rules directory carrying one, so
// the third layer cannot be the quiet one.
//
// Two elements carrying one id is refused rather than resolved. §2.6 item 2
// settles a collision between layers by having the higher win whole, and says
// nothing about a collision inside one layer because there is no answer to
// give: neither element is above the other, and taking the first would enforce
// one of two standards its author wrote and never mention the other.
func parseProfileRules(profilePath string, raw []json.RawMessage) ([]Resolved, error) {
	rules := make([]Resolved, 0, len(raw))
	first := make(map[string]int, len(raw))
	for at, data := range raw {
		r, err := parse(profilePath, data, fromProfile)
		if err != nil {
			return nil, locate(at, err)
		}
		if before, duplicate := first[r.ID]; duplicate {
			return nil, &MalformedError{
				File:  profilePath,
				Field: element(at) + ".id",
				Problem: fmt.Sprintf(
					"is %q, which %s already carries; §2.6 item 2 makes a rule id unique "+
						"after resolution, and a layer cannot override itself",
					r.ID, element(before),
				),
			}
		}
		first[r.ID] = at
		rules = append(rules, Resolved{Rule: r, Path: profilePath})
	}
	sortByID(rules)
	return rules, nil
}

// locate re-points a fault found in an embedded rule at the element carrying
// it. §2.6's messages name a field of a rule — `class`, `detect.mode` — and the
// user reading this one has a profile file open with an array in it, so the
// array position is the difference between a message they can act on and a
// search. The file already names the profile, because that is the path the
// parse was given.
//
// A fault no field can be blamed for keeps that shape and gains only the
// position, so the sentence never trails a bare dot.
func locate(at int, err error) error {
	var malformed *MalformedError
	if !errors.As(err, &malformed) {
		return err
	}
	field := element(at)
	if malformed.Field != "" {
		field += "." + malformed.Field
	}
	return &MalformedError{File: malformed.File, Field: field, Problem: malformed.Problem}
}

// element names one entry of a profile's `rules` array the way its author reads
// it, so the two places that build such a name cannot spell it differently.
func element(at int) string {
	return fmt.Sprintf("rules[%d]", at)
}

// sortByID puts rules into ascending lexicographic id order.
//
// It is by id and not by file name, which are not the same order: `a.json`
// sorts before `a-b.json` because `.` is above `-`, while by id `a` comes
// before `a-b`. os.ReadDir hands back file-name order, so relying on it would
// order most corpora right and one shape of id wrong.
func sortByID(rules []Resolved) {
	slices.SortFunc(rules, func(a, b Resolved) int { return strings.Compare(a.Rule.ID, b.Rule.ID) })
}
