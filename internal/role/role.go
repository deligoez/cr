// Package role models the role file of spec/0.1.0.md §2.5.
//
// A role customises the prompt for one axis. §2.5 states the division plainly:
// cr owns the output contract, and a role only supplies persona and focus.
// §4.6.1 takes a role's `instructions` and `focus` into the prompt it emits,
// and §4.6.2 has cr — never the role — name the NDJSON path that role writes to
// and state the §6.1 record schema together with the fields §6.1.4 forbids.
//
// A role file is project-owned and hand-edited, so it is untrusted input, and
// the fault worth defending against is a role that tries to redefine what cr
// emits. The defence is structural, in two layers. The struct below carries
// exactly the seven rows of §2.5's table and is held to it by a guard test, so
// there is nowhere to put an output path, a record schema, or a field cr writes
// itself. Every top-level key outside that table is then refused by name rather
// than ignored, because a silently dropped `output_path` leaves its author
// believing it took effect — which is the whole failure this boundary exists to
// prevent.
//
// A malformed role file aborts the command with exit code 3, naming the file
// and the offending field, per §2.5.3. The abort is a *MalformedError, or an
// *axis.InvalidError when `axis` is not one of the four ids of §1.5; the cli
// layer maps both onto the code.
//
// This package defends the file format and nothing else. §2.5.1's four default
// roles, §2.5.2's eject, §2.5.4's resolution order, and §2.5.5's corpus order
// are separate obligations, written elsewhere against this schema.
package role

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/axis"
)

// idPattern is the kebab-case §2.5 requires of a role id: lowercase
// alphanumerics in hyphen-separated groups. Because §2.5 also fixes the id as
// the file stem, this constrains the filename as much as the field — which is
// the point of the pair. The id is what §4.6.2 attributes output to and what
// §2.5.5 orders the corpus by, so it must be one shape and readable in a
// directory listing.
var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// fields is the §2.5 table's top-level key set, in the table's own order. It is
// both the allowlist and the message a rejected key is answered with, so the
// two cannot drift apart.
var fields = []string{"id", "title", "axis", "instructions", "focus", "profiles", "classes"}

// Role is one §2.5 role: every row of the table, with each absent list
// normalised to an empty one so a role never serialises a slice as null.
//
// The set of fields is the contract. §2.5 gives a role a persona (`title`,
// `instructions`), a lens (`axis`), a set of prompts to append (`focus`), and
// the profiles it applies to; it gives a role no say in what cr emits.
type Role struct {
	// ID is the role id, equal to the file stem, kebab-case.
	ID string `json:"id"`
	// Title is the human label shown in prompts and progress output.
	Title string `json:"title"`
	// Axis is the axis this role serves, one of the four ids of §1.5.
	Axis string `json:"axis"`
	// Instructions is the role's framing text, carried into the §4.6.1
	// prompt verbatim.
	Instructions string `json:"instructions"`
	// Focus holds the questions appended to that prompt.
	Focus []string `json:"focus"`
	// Profiles names the profiles this role applies to. Empty means all,
	// which §4.5.1 reads when it decides whether the role is active.
	Profiles []string `json:"profiles"`
	// Classes is the role's class vocabulary (§2.5), printed in its prompts
	// per §4.6.1. `cr record` reports a record of the role whose class is not
	// in it and never rejects one for that (§2.5.6), so it is a vocabulary
	// and not a fence. Empty means the role declares none.
	Classes []string `json:"classes"`
}

// MalformedError reports a role file cr cannot use. It carries the file so the
// user can open it and the field so they know what to fix, which is what §2.5.3
// requires of the abort. The cli layer maps it onto exit code 3.
type MalformedError struct {
	// File is the role file.
	File string
	// Field is the offending field, empty when the fault is the file as a
	// whole and no single field can be blamed for it.
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

// Load reads and validates one role file, and decides §2.5.2's standing of the
// bytes it read. path also fixes the expected id, which §2.5 requires to equal
// the file stem.
//
// The standing is settled here and nowhere else, because it is a fact about the
// file rather than about what the file says: whether these exact bytes are a
// role an earlier release shipped can only be known where the bytes are read,
// and a second read to answer it later could read a different file. The Layer
// is left unset; §2.5.4 decides that, and only resolution knows it.
func Load(path string) (Resolved, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Resolved{}, &MalformedError{File: path, Problem: fmt.Sprintf("cannot be read: %v", err)}
	}
	r, err := Parse(path, data)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{Role: r, stale: staleNotice(path, data)}, nil
}

// Parse validates data as the role file at path. The path is only read for the
// file stem and for the error message; nothing on disk is touched.
//
// The key check runs before the field checks on purpose. A file carrying both
// an unknown key and a missing required field is most often one file: the
// author wrote the field under a name cr does not know, and naming the key they
// used is the answer that leads them to it.
func Parse(path string, data []byte) (Role, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return Role{}, decodeError(path, err)
	}
	if err := checkKeys(path, keys); err != nil {
		return Role{}, err
	}
	var r Role
	if err := json.Unmarshal(data, &r); err != nil {
		return Role{}, decodeError(path, err)
	}
	if err := r.validate(path); err != nil {
		return Role{}, err
	}
	r.Focus = list(r.Focus)
	r.Profiles = list(r.Profiles)
	r.Classes = list(r.Classes)
	return r, nil
}

// checkKeys refuses any top-level key outside §2.5's table. This is where "cr
// owns the output contract" stops being a sentence and becomes a fence: a role
// declaring `output_path`, `schema`, or a §6.1.4 stamp field is rejected by
// name rather than decoded into nothing.
//
// Keys are visited in sorted order so a file carrying two unknown keys reports
// the same one on every run.
func checkKeys(path string, keys map[string]json.RawMessage) error {
	unknown := make([]string, 0, len(keys))
	for key := range keys {
		if !slices.Contains(fields, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	return &MalformedError{
		File:  path,
		Field: unknown[0],
		Problem: fmt.Sprintf(
			"is not a role field; §2.5 has exactly %s, because cr owns the output contract and a role only supplies persona and focus",
			strings.Join(fields, ", "),
		),
	}
}

// decodeError turns a decoding failure into a MalformedError, keeping the field
// the decoder identified so a type error names the field like every other
// fault. A `focus` that is a string rather than an array arrives here.
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

// validate applies every requirement of the §2.5 table, in table order, so the
// first fault a user sees is the earliest one in the file's own layout.
//
// A required text field is checked for blankness rather than absence. A title
// of " " is not a human label and instructions of " " frame nothing, so the two
// cases have the same fix and get the same message.
func (r *Role) validate(path string) error {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	switch {
	case r.ID == "":
		return &MalformedError{File: path, Field: "id", Problem: "is required"}
	case r.ID != stem:
		return &MalformedError{
			File:    path,
			Field:   "id",
			Problem: fmt.Sprintf("is %q, but §2.5 requires it to equal the file stem %q", r.ID, stem),
		}
	case !idPattern.MatchString(r.ID):
		return &MalformedError{
			File:  path,
			Field: "id",
			Problem: fmt.Sprintf(
				"is %q, which is not kebab-case; §2.5 requires %s, and the file must be renamed with it",
				r.ID, idPattern,
			),
		}
	case strings.TrimSpace(r.Title) == "":
		return &MalformedError{File: path, Field: "title", Problem: "is required"}
	case strings.TrimSpace(r.Axis) == "":
		return &MalformedError{File: path, Field: "axis", Problem: "is required"}
	}
	// §1.5 closes the axis id set, and axis.Validate is the one place that
	// judges it.
	if err := axis.Validate(path, "axis", r.Axis); err != nil {
		return err
	}
	if strings.TrimSpace(r.Instructions) == "" {
		return &MalformedError{File: path, Field: "instructions", Problem: "is required"}
	}
	return nil
}

// list copies a decoded list and turns an absent one into an empty one, so no
// role field ever serialises as null.
func list(values []string) []string {
	return append(make([]string, 0, len(values)), values...)
}
