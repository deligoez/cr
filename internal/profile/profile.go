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
// This package defends the file format and nothing else. Selecting a profile by
// its marker files, checking `tests.count_pattern`'s group arity, and deriving
// `tests.probe_path_template`'s default are separate obligations of §2.4 and are
// implemented elsewhere.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/axis"
)

// The defaults §2.4 gives the two numeric test fields.
const (
	// DefaultTimeoutSeconds bounds one test run.
	DefaultTimeoutSeconds = 900
	// DefaultOutputTailBytes bounds the retained runner output.
	DefaultOutputTailBytes = 4096
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
}

// Match is the selection block. Both fields are required, so a profile always
// states what it matches, even when the answer is nothing.
type Match struct {
	// Files are the marker files selecting this profile. Empty means the
	// profile is never selected automatically.
	Files []string `json:"files"`
	// Globs are the source globs this profile owns.
	Globs []string `json:"globs"`
}

// Sandbox prepares the probe worktree of §5.
type Sandbox struct {
	// Copy names paths copied from the main worktree into the sandbox.
	Copy []string `json:"copy"`
	// Setup holds the commands run once after sandbox creation.
	Setup []string `json:"setup"`
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
	// TimeoutSeconds bounds one run, default DefaultTimeoutSeconds.
	TimeoutSeconds int `json:"timeout_seconds"`
	// OutputTailBytes is the retained runner output, default
	// DefaultOutputTailBytes.
	OutputTailBytes int `json:"output_tail_bytes"`
	// CountPattern yields the executed and failed test counts, in that
	// order.
	CountPattern string `json:"count_pattern"`
	// ProbePathTemplate is where a gap probe's file is placed.
	ProbePathTemplate string `json:"probe_path_template"`
}

// Symbols carries the language hint for the reinvention search of §4.3.
type Symbols struct {
	// Lang is the language hint.
	Lang string `json:"lang"`
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
}

type wireMatch struct {
	Files []string `json:"files"`
	Globs []string `json:"globs"`
}

type wireSandbox struct {
	Copy  []string `json:"copy"`
	Setup []string `json:"setup"`
}

type wireTests struct {
	Cmd               []string `json:"cmd"`
	Globs             []string `json:"globs"`
	FilterFlag        string   `json:"filter_flag"`
	TimeoutSeconds    *int     `json:"timeout_seconds"`
	OutputTailBytes   *int     `json:"output_tail_bytes"`
	CountPattern      string   `json:"count_pattern"`
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
	if err := w.validate(path); err != nil {
		return Profile{}, err
	}
	return w.resolve(), nil
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
	return w.Tests.validate(path)
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
	return nil
}

// resolve fills in the §2.4 defaults and normalises every absent list to an
// empty one.
func (w *wire) resolve() Profile {
	p := Profile{
		ID:    w.ID,
		Match: Match{Files: list(w.Match.Files), Globs: list(w.Match.Globs)},
		Axes:  make(map[string]bool, len(w.Axes)),
		Tests: Tests{
			Cmd:             []string{},
			Globs:           []string{},
			TimeoutSeconds:  DefaultTimeoutSeconds,
			OutputTailBytes: DefaultOutputTailBytes,
		},
		Sandbox: Sandbox{Copy: []string{}, Setup: []string{}},
		Rules:   make([]json.RawMessage, 0, len(w.Rules)),
	}
	for id, enabled := range w.Axes {
		p.Axes[id] = enabled
	}
	p.Rules = append(p.Rules, w.Rules...)
	if w.Sandbox != nil {
		p.Sandbox = Sandbox{Copy: list(w.Sandbox.Copy), Setup: list(w.Sandbox.Setup)}
	}
	if w.Symbols != nil {
		p.Symbols = Symbols{Lang: w.Symbols.Lang}
	}
	if t := w.Tests; t != nil {
		p.Tests.Cmd = list(t.Cmd)
		p.Tests.Globs = list(t.Globs)
		p.Tests.FilterFlag = t.FilterFlag
		p.Tests.CountPattern = t.CountPattern
		p.Tests.ProbePathTemplate = t.ProbePathTemplate
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
