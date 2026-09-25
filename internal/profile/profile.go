// Package profile models the mechanical configuration of spec/0.1.0.md §2.4.
//
// A profile names marker files, source globs, axis defaults, sandbox steps, and
// a test runner. It is data, never prompt text: nothing in it is addressed to an
// agent, and §2.4's field table is the whole surface. The struct is held to that
// table by a guard test, so no free-form instruction field can be added to it
// without the guard failing.
//
// A profile file cr cannot use aborts the command with exit code 3, naming the
// file and the offending field, per §2.5 item 3. The abort is a *MalformedError,
// or an *axis.InvalidError when an `axes` key is not one of the four ids of
// §1.5; the cli layer maps both onto the code.
//
// This package defends the file format and nothing else, plus the one derived
// value §2.4 documents: probepath.go resolves `tests.probe_path_template`, so a
// parsed profile always carries the template §5.1.6 and §5.4.2 consume, and
// `tests.count_pattern` and `tests.failed_pattern` are held to §2.4's group
// arity. Running them against runner output is §5.2.1's job, not this
// package's. Selecting a profile by its marker files is a separate obligation
// of §2.4, implemented elsewhere.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/glob"
)

// The defaults §2.4 gives the two numeric test fields.
const (
	// DefaultTimeoutSeconds bounds one test run.
	DefaultTimeoutSeconds = 900
	// DefaultOutputTailBytes bounds the retained runner output.
	DefaultOutputTailBytes = 4096
)

// The §2.4 constraints on the two test count patterns.
const (
	// countPatternField and failedPatternField are the fields in the dotted
	// §2.4 spelling every MalformedError about a pattern carries.
	countPatternField  = "tests.count_pattern"
	failedPatternField = "tests.failed_pattern"
	// patternGroups is the capture group count §2.4 fixes for each of them
	// in the sum mode: one, holding the count that match contributes to
	// §5.2.1's sum.
	patternGroups = 1
	// countModeField is `tests.count_mode`, in the same spelling.
	countModeField = "tests.count_mode"
)

// The two values of `tests.count_mode`, per §5.2.1.
const (
	// CountModeSum adds the one capture group of every match, for a runner
	// that prints a recap line with numbers on it. It is the default.
	CountModeSum = "sum"
	// CountModeOccurrences counts the matches, for a runner that prints one
	// line per test and no recap.
	CountModeOccurrences = "occurrences"
)

// Profile is one resolved §2.4 profile: every field of the table, with the
// documented defaults already applied and every absent list normalised to an
// empty one, so a profile never serialises a slice as null.
type Profile struct {
	// ID is the profile id, equal to the file stem.
	ID string `json:"id"`
	// Match selects the profile and names the source it owns.
	Match Match `json:"match"`
	// Axes is the default enabled state per axis id of §1.5.
	Axes map[string]bool `json:"axes"`
	// Sandbox prepares the probe worktree.
	Sandbox Sandbox `json:"sandbox"`
	// Tests configures the test runner.
	Tests Tests `json:"tests"`
	// Rules holds the rule objects shipped with the profile, the third layer
	// of §2.6 item 1. They are carried verbatim: §2.6 owns the rule schema,
	// so a profile passes them through rather than interpreting them.
	Rules []json.RawMessage `json:"rules"`
	// Symbols hints the reinvention search of §4.3.
	Symbols Symbols `json:"symbols"`
	// Units carries §4.6.7's unit kinds.
	Units Units `json:"units"`

	// stale is the sentence naming the loaded file as an earlier release's
	// shipped profile, and empty for every other file. It is unexported
	// because it is not a row of §2.4's table: it is what cr found out about
	// the file, not what the file says.
	stale string
}

// StaleDisclosures are the honesty sentences a command that loaded this
// profile owes its reader: one when the file it was read from is, byte for
// byte, a profile an earlier release shipped, naming that release, what the
// shipped profile has changed since, and that `cr init` updates it. A file
// equal to this build's profile, or one somebody edited, gets none, and
// nothing is ever written back: loading a profile only reads it.
func (p *Profile) StaleDisclosures() []string {
	if p.stale == "" {
		return []string{}
	}
	return []string{p.stale}
}

// Match is the selection block. Both fields are required, so a profile always
// states what it matches, even when the answer is nothing.
type Match struct {
	// Files are the marker files selecting this profile. Empty means the
	// profile is never selected automatically.
	Files []string `json:"files"`
	// Globs are the source globs this profile owns.
	Globs []string `json:"globs"`
	// Unless are marker files whose presence keeps this profile out of
	// automatic selection (§2.4.2): a TypeScript project that configures Jest
	// is not a Vitest project, whatever else it shares with one.
	Unless []string `json:"unless"`
}

// Sandbox prepares the probe worktree of §5.
type Sandbox struct {
	// Copy names paths copied from the main worktree into the sandbox.
	Copy []string `json:"copy"`
	// Setup holds the commands run once after sandbox creation.
	Setup []string `json:"setup"`
	// Require names the paths, relative to the repository root, that
	// §5.1.8 has `cr test` and `cr probe run` refuse to run without. It
	// is what turns a suite silently reading the wrong environment into
	// a refusal before anything executes.
	Require []string `json:"require"`
}

// Tests configures the runner. An absent Cmd disables the test axis, which is
// why the field keeps its zero value rather than carrying a default.
type Tests struct {
	// Cmd is the test runner argv; empty disables the test axis.
	Cmd []string `json:"cmd"`
	// Globs identify test files. The first entry seeds
	// ProbePathTemplate's default.
	Globs []string `json:"globs"`
	// FilterFlag narrows the run to a subset.
	FilterFlag string `json:"filter_flag"`
	// PathsArg is the argv §5.2.1 appends to Cmd once per `--path`, with
	// every `{path}` in it replaced by that path. An empty PathsArg is a
	// profile that cannot narrow a run by path at all, and §2.4 refuses a
	// `--path` against one rather than dropping it.
	PathsArg []string `json:"paths_arg"`
	// PathsDefault is the argv §5.2.1 appends to Cmd when no `--path` is
	// given. A runner whose bare invocation tests one directory — `go test`
	// tests the current package and nothing else — needs it to run the whole
	// suite, and a runner that tests everything by default leaves it empty.
	PathsDefault []string `json:"paths_default"`
	// TimeoutSeconds bounds one run, default DefaultTimeoutSeconds.
	TimeoutSeconds int `json:"timeout_seconds"`
	// OutputTailBytes is the retained runner output, default
	// DefaultOutputTailBytes.
	OutputTailBytes int `json:"output_tail_bytes"`
	// CountPattern yields one executed test count per match, which §5.2.1
	// sums over every match in the output.
	CountPattern string `json:"count_pattern"`
	// FailedPattern yields one failed test count per match, summed the same
	// way. Matching nothing means zero rather than undetermined.
	FailedPattern string `json:"failed_pattern"`
	// CountMode is how §5.2.1 reads the two patterns: CountModeSum adds
	// their one capture group over every match, CountModeOccurrences counts
	// their matches. It is never empty once resolved.
	CountMode string `json:"count_mode"`
	// ProbePathTemplate is where a gap probe's file is placed, resolved
	// per probepath.go: the default is filled in, `<ext>` is substituted,
	// and `<probe-id>` is the one placeholder left.
	ProbePathTemplate string `json:"probe_path_template"`
}

// Symbols carries the language hint for the reinvention search of §4.3.
type Symbols struct {
	// Lang is the language hint.
	Lang string `json:"lang"`
}

// Units is §2.4's `units` block: the unit kinds of §4.6.7.
type Units struct {
	// Kinds are the declared kinds, in file order, which is the order
	// KindOf tries them in.
	Kinds []UnitKind `json:"kinds"`
}

// UnitKind is one entry of `units.kinds`: a unit every path of which matches
// Globs is of Kind, and only the roles Roles names read it (§4.6.7).
type UnitKind struct {
	// Kind names the kind, and is what the `na` cells cr records for the
	// roles left out give as their reason.
	Kind string `json:"kind"`
	// Globs are the repository-relative globs a unit's paths must all match.
	Globs []string `json:"globs"`
	// Roles are the ids of the roles that read a unit of this kind.
	Roles []string `json:"roles"`
}

// KindOf is §4.6.7 for one unit: the first declared kind whose globs every one
// of paths matches, and false when none does or the unit has no path.
func (p *Profile) KindOf(paths ...string) (UnitKind, bool) {
	if len(paths) == 0 {
		return UnitKind{}, false
	}
	for _, kind := range p.Units.Kinds {
		if !slices.ContainsFunc(paths, func(path string) bool { return !glob.MatchAny(kind.Globs, path) }) {
			return kind, true
		}
	}
	return UnitKind{}, false
}

// MalformedError reports a profile file cr cannot use. It carries the file so
// the user can open it and the field so they know what to fix, which is what
// §2.5 item 3 requires of the abort. The cli layer maps it onto exit code 3.
type MalformedError struct {
	// File is the profile file.
	File string
	// Field is the offending field in dotted §2.4 spelling, empty when the
	// fault is the file as a whole.
	Field string
	// Problem completes the sentence naming the field.
	Problem string
}

func (e *MalformedError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%s: %s", e.File, e.Problem)
	}
	return fmt.Sprintf("%s: %s %s", e.File, e.Field, e.Problem)
}

// wire is the on-disk shape. It differs from Profile in one respect: a field
// whose absence means something distinct from its zero value is a pointer or a
// nilable list here, so "absent" and "empty" stay separable while validating.
type wire struct {
	ID      string            `json:"id"`
	Match   *wireMatch        `json:"match"`
	Axes    map[string]bool   `json:"axes"`
	Sandbox *wireSandbox      `json:"sandbox"`
	Tests   *wireTests        `json:"tests"`
	Rules   []json.RawMessage `json:"rules"`
	Symbols *wireSymbols      `json:"symbols"`
	Units   *wireUnits        `json:"units"`
}

type wireUnits struct {
	Kinds []UnitKind `json:"kinds"`
}

type wireMatch struct {
	Files  []string `json:"files"`
	Globs  []string `json:"globs"`
	Unless []string `json:"unless"`
}

type wireSandbox struct {
	Copy    []string `json:"copy"`
	Setup   []string `json:"setup"`
	Require []string `json:"require"`
}

type wireTests struct {
	Cmd               []string `json:"cmd"`
	Globs             []string `json:"globs"`
	FilterFlag        string   `json:"filter_flag"`
	PathsArg          []string `json:"paths_arg"`
	PathsDefault      []string `json:"paths_default"`
	CountMode         string   `json:"count_mode"`
	TimeoutSeconds    *int     `json:"timeout_seconds"`
	OutputTailBytes   *int     `json:"output_tail_bytes"`
	CountPattern      string   `json:"count_pattern"`
	FailedPattern     string   `json:"failed_pattern"`
	ProbePathTemplate string   `json:"probe_path_template"`
}

type wireSymbols struct {
	Lang string `json:"lang"`
}

// Load reads and validates one profile file. path also fixes the expected id,
// which §2.4 requires to equal the file stem.
func Load(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, &MalformedError{File: path, Problem: fmt.Sprintf("cannot be read: %v", err)}
	}
	return Parse(path, data)
}

// Parse validates data as the profile file at path and applies the §2.4
// defaults. The path is only read for the file stem and for the error message;
// nothing on disk is touched.
func Parse(path string, data []byte) (Profile, error) {
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return Profile{}, decodeError(path, err)
	}
	if err := refuseProtectedFields(path, data); err != nil {
		return Profile{}, err
	}
	if err := w.validate(path); err != nil {
		return Profile{}, err
	}
	template, err := w.Tests.probeTemplate(path)
	if err != nil {
		return Profile{}, err
	}
	resolved := w.resolve(template)
	resolved.stale = staleNotice(path, data)
	return resolved, nil
}

// refuseProtectedFields refuses a field, at any depth, whose name addresses the
// confirmation gate, the argued forcing or the question label. §6.3.3 names a
// profile field among the channels that may not override the forcing, and
// §2.4's table has no such field, so decoding it into nothing would leave its
// author believing it took effect. The error is §2.7's *config.ProtectedError,
// naming the field in dotted spelling, prefixed with the file.
func refuseProtectedFields(path string, data []byte) error {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return decodeError(path, err)
	}
	if err := protectedName("", document); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// CheckDir refuses the first profile file in dir, in file name order, holding a
// field whose name addresses a protected decision, with the error Parse gives
// it, and checks nothing else. Every profile file is one a command may resolve:
// automatic selection loads them all, and the `profile` setting and a round's
// profile_id each name one of them. A directory or file that cannot be read,
// and a file that is not JSON, is left to the command that loads it: this is
// the check every command makes before its work.
func CheckDir(dir string) error {
	entries, listErr := os.ReadDir(dir)
	if listErr != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), fileExt) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, readErr := os.ReadFile(path)
		var document any
		if readErr != nil || json.Unmarshal(data, &document) != nil {
			continue
		}
		if err := protectedName("", document); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

// protectedName walks one decoded JSON value and checks every object key it
// holds, in sorted order so a file carrying two such fields names the same one
// on every run. An element of a list is named by its index.
func protectedName(prefix string, value any) error {
	switch node := value.(type) {
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(node)) {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			if err := config.CheckName(name); err != nil {
				return err
			}
			if err := protectedName(name, node[key]); err != nil {
				return err
			}
		}
	case []any:
		for i, element := range node {
			if err := protectedName(fmt.Sprintf("%s[%d]", prefix, i), element); err != nil {
				return err
			}
		}
	}
	return nil
}

// decodeError turns a decoding failure into a MalformedError, keeping the field
// the decoder identified so a type error names the field like every other
// fault.
func decodeError(path string, err error) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return &MalformedError{
			File:    path,
			Field:   typeErr.Field,
			Problem: fmt.Sprintf("must be %s, but the file has %s", typeErr.Type, typeErr.Value),
		}
	}
	return &MalformedError{File: path, Problem: fmt.Sprintf("is not valid JSON: %v", err)}
}

// validate applies every requirement of the §2.4 table, in table order, so the
// first fault a user sees is the earliest one in the file's own layout.
func (w *wire) validate(path string) error {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	switch {
	case w.ID == "":
		return &MalformedError{File: path, Field: "id", Problem: "is required"}
	case w.ID != stem:
		return &MalformedError{
			File:    path,
			Field:   "id",
			Problem: fmt.Sprintf("is %q, but §2.4 requires it to equal the file stem %q", w.ID, stem),
		}
	case w.Match == nil:
		return &MalformedError{File: path, Field: "match", Problem: "is required"}
	case w.Match.Files == nil:
		return &MalformedError{
			File:    path,
			Field:   "match.files",
			Problem: "is required; write [] for a profile that is never selected automatically",
		}
	case w.Match.Globs == nil:
		return &MalformedError{File: path, Field: "match.globs", Problem: "is required"}
	case w.Axes == nil:
		return &MalformedError{File: path, Field: "axes", Problem: "is required"}
	}
	// §1.5 closes the axis id set, and axis.Validate is the one place that
	// judges it. Keys are visited in sorted order so a file carrying two bad
	// ids reports the same one on every run.
	keys := make([]string, 0, len(w.Axes))
	for id := range w.Axes {
		keys = append(keys, id)
	}
	slices.Sort(keys)
	for _, id := range keys {
		if err := axis.Validate(path, "axes."+id, id); err != nil {
			return err
		}
	}
	if err := w.Tests.validate(path); err != nil {
		return err
	}
	return w.Units.validate(path)
}

// validate checks §2.4's `units.kinds`: every entry names a kind no other
// entry names, at least one glob, and a `roles` list, each id non-empty. An
// empty `roles` is a kind no role reads, whose every cell cr records `na`.
func (u *wireUnits) validate(path string) error {
	if u == nil {
		return nil
	}
	seen := make(map[string]bool, len(u.Kinds))
	for i := range u.Kinds {
		entry := &u.Kinds[i]
		field := fmt.Sprintf("units.kinds[%d]", i)
		switch {
		case entry.Kind == "":
			return &MalformedError{File: path, Field: field + ".kind", Problem: "is required"}
		case seen[entry.Kind]:
			return &MalformedError{File: path, Field: field + ".kind",
				Problem: fmt.Sprintf("is %q, which an earlier entry already names", entry.Kind)}
		case len(entry.Globs) == 0 || slices.Contains(entry.Globs, ""):
			return &MalformedError{File: path, Field: field + ".globs",
				Problem: "must hold at least one glob, none of them empty"}
		case entry.Roles == nil:
			return &MalformedError{File: path, Field: field + ".roles",
				Problem: "is required; write [] for a kind no role reads"}
		case slices.Contains(entry.Roles, ""):
			return &MalformedError{File: path, Field: field + ".roles", Problem: "holds an empty role id"}
		}
		seen[entry.Kind] = true
	}
	return nil
}

// validate checks the test block. A nil block is the absent one, which §2.4
// allows: it disables the test axis.
func (t *wireTests) validate(path string) error {
	if t == nil {
		return nil
	}
	if t.Cmd != nil && len(t.Cmd) == 0 {
		return &MalformedError{
			File:    path,
			Field:   "tests.cmd",
			Problem: "is empty; omit it to disable the test axis",
		}
	}
	if len(t.Cmd) > 0 && len(t.Globs) == 0 {
		return &MalformedError{
			File:    path,
			Field:   "tests.globs",
			Problem: "is required when tests.cmd is present",
		}
	}
	if t.TimeoutSeconds != nil && *t.TimeoutSeconds <= 0 {
		return &MalformedError{
			File:    path,
			Field:   "tests.timeout_seconds",
			Problem: fmt.Sprintf("is %d; a run needs a positive timeout", *t.TimeoutSeconds),
		}
	}
	if t.OutputTailBytes != nil && *t.OutputTailBytes <= 0 {
		return &MalformedError{
			File:    path,
			Field:   "tests.output_tail_bytes",
			Problem: fmt.Sprintf("is %d; retaining no output leaves a run unevidenced", *t.OutputTailBytes),
		}
	}
	groups, err := t.patternGroups(path)
	if err != nil {
		return err
	}
	if err := validatePattern(path, countPatternField, "executed", t.CountPattern, groups); err != nil {
		return err
	}
	// §2.4 makes tests.failed_pattern required alongside tests.count_pattern,
	// and §5.2.1 says why: no match is zero failures, so a profile that
	// configured only the executed count would read every run as a clean one.
	if t.CountPattern != "" && t.FailedPattern == "" {
		return &MalformedError{
			File:    path,
			Field:   failedPatternField,
			Problem: "is required when " + countPatternField + " is present",
		}
	}
	return validatePattern(path, failedPatternField, "failed", t.FailedPattern, groups)
}

// patternGroups is the capture-group count `tests.count_mode` requires of both
// patterns: one to sum, none to count. An absent mode is §2.4's default, the
// sum, and a value naming neither aborts rather than guessing which a runner
// meant — the two read the same output as different numbers.
func (t *wireTests) patternGroups(path string) (int, error) {
	switch t.CountMode {
	case "", CountModeSum:
		return patternGroups, nil
	case CountModeOccurrences:
		return 0, nil
	}
	return 0, &MalformedError{
		File:  path,
		Field: countModeField,
		Problem: fmt.Sprintf("is %q, and §2.4 admits %q or %q",
			t.CountMode, CountModeSum, CountModeOccurrences),
	}
}

// validatePattern holds one of §2.4's two count patterns to the arity §2.4
// fixes: exactly one capture group, holding the number that match contributes
// to §5.2.1's sum. Arity is what makes the pattern's output addressable, so a
// pattern with any other count is not a weaker pattern but an unreadable one —
// with none §5.2.1 has no group to take a number from, and with two it has no
// rule for which one counts. field and what name the pattern and the number its
// group yields, so one function serves both fields and still says which of them
// the author must fix.
//
// One group is what lets a runner spread its counts over several matches
// instead of one: §5.2.1 matches the pattern repeatedly and adds the groups up,
// which is how a recap line naming a status per count is read.
//
// Capturing is the only kind of group that counts, so `(?:...)` is invisible
// here and `(?P<name>...)` is not: NumSubexp counts exactly the groups §5.2.1
// can index. An empty pattern is the absent one, since §2.4 makes the field
// optional and a JSON string cannot distinguish the two.
//
// A pattern that does not compile aborts the same way. §2.4 names only the
// arity fault, but its group count cannot be read at all, and §2.6.1.2 already
// settles the shape for cr's other configured regex: an uncompilable pattern
// aborts with exit code 3 naming the file it came from.
func validatePattern(path, field, what, pattern string, groups int) error {
	if pattern == "" {
		return nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return &MalformedError{
			File:    path,
			Field:   field,
			Problem: fmt.Sprintf("is not a valid Go regexp: %v", err),
		}
	}
	if n := compiled.NumSubexp(); n != groups {
		yielding := fmt.Sprintf("yielding a count of %s tests", what)
		if groups == 0 {
			yielding = fmt.Sprintf("because %s %q counts matching lines of %s tests",
				countModeField, CountModeOccurrences, what)
		}
		return &MalformedError{
			File:    path,
			Field:   field,
			Problem: fmt.Sprintf("has %d capture groups, but §2.4 requires exactly %d, %s. A (?:...) group does not capture.", n, groups, yielding),
		}
	}
	return nil
}

// resolve fills in the §2.4 defaults and normalises every absent list to an
// empty one. probeTemplate is the already-resolved `tests.probe_path_template`,
// computed before this point because deriving it can fail.
func (w *wire) resolve(probeTemplate string) Profile {
	p := Profile{
		ID:    w.ID,
		Match: Match{Files: list(w.Match.Files), Globs: list(w.Match.Globs), Unless: list(w.Match.Unless)},
		Axes:  make(map[string]bool, len(w.Axes)),
		Tests: Tests{
			Cmd:             []string{},
			Globs:           []string{},
			PathsArg:        []string{},
			PathsDefault:    []string{},
			CountMode:       CountModeSum,
			TimeoutSeconds:  DefaultTimeoutSeconds,
			OutputTailBytes: DefaultOutputTailBytes,
		},
		Sandbox: Sandbox{Copy: []string{}, Setup: []string{}, Require: []string{}},
		Rules:   make([]json.RawMessage, 0, len(w.Rules)),
	}
	maps.Copy(p.Axes, w.Axes)
	p.Rules = append(p.Rules, w.Rules...)
	if w.Sandbox != nil {
		p.Sandbox = Sandbox{
			Copy:    list(w.Sandbox.Copy),
			Setup:   list(w.Sandbox.Setup),
			Require: list(w.Sandbox.Require),
		}
	}
	if w.Symbols != nil {
		p.Symbols = Symbols{Lang: w.Symbols.Lang}
	}
	p.Units = Units{Kinds: make([]UnitKind, 0)}
	if w.Units != nil {
		for _, kind := range w.Units.Kinds {
			p.Units.Kinds = append(p.Units.Kinds, UnitKind{Kind: kind.Kind, Globs: list(kind.Globs), Roles: list(kind.Roles)})
		}
	}
	if t := w.Tests; t != nil {
		p.Tests.Cmd = list(t.Cmd)
		p.Tests.Globs = list(t.Globs)
		p.Tests.FilterFlag = t.FilterFlag
		p.Tests.PathsArg = list(t.PathsArg)
		p.Tests.PathsDefault = list(t.PathsDefault)
		if t.CountMode != "" {
			p.Tests.CountMode = t.CountMode
		}
		p.Tests.CountPattern = t.CountPattern
		p.Tests.FailedPattern = t.FailedPattern
		p.Tests.ProbePathTemplate = probeTemplate
		if t.TimeoutSeconds != nil {
			p.Tests.TimeoutSeconds = *t.TimeoutSeconds
		}
		if t.OutputTailBytes != nil {
			p.Tests.OutputTailBytes = *t.OutputTailBytes
		}
	}
	return p
}

// list copies a decoded list and turns an absent one into an empty one, so no
// profile field ever serialises as null.
func list(values []string) []string {
	return append(make([]string, 0, len(values)), values...)
}
