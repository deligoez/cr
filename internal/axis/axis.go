// Package axis holds the closed set of review axes.
//
// spec/0.1.0.md §1.5 fixes the set at exactly intent, correctness, convention,
// and test, defined in §4.1 through §4.4. The set is closed: it is a literal
// here, there is no registration entry point, and no configuration key names an
// axis id, so no layer can extend it. Every axis field in a profile, role, or
// rule passes through Validate, and a value outside the set aborts the command
// with exit code 3.
//
// The axis on a finding is a different thing: §6.1 computes it from the
// record's role and never reads it from input. This validator guards
// configuration files, not records.
package axis

import (
	"fmt"
	"slices"
	"strings"
)

// The four axis ids of §1.5.
const (
	Intent      = "intent"
	Correctness = "correctness"
	Convention  = "convention"
	Test        = "test"
)

// ids is the closed set, in the order §1.5 lists it.
var ids = []string{Intent, Correctness, Convention, Test}

// IDs returns the axis ids in spec order. The result is a copy, so a caller can
// neither widen the set nor reorder it.
func IDs() []string {
	return append(make([]string, 0, len(ids)), ids...)
}

// Valid reports whether id names one of the four axes.
func Valid(id string) bool {
	return slices.Contains(ids, id)
}

// InvalidError reports an axis field naming something outside the closed set.
// It carries the file so the user can open it and the value so they can see
// what was rejected, which is what §1.5 and §2.5 item 3 require of the abort.
type InvalidError struct {
	// File is the profile, role, or rule file that carried the value.
	File string
	// Field is the field inside that file, such as "axis" or "axes.foo".
	Field string
	// Value is the offending value, exactly as it was written.
	Value string
}

func (e *InvalidError) Error() string {
	return fmt.Sprintf(
		"%s: %s is %q, which is not an axis id; v0.2 has exactly %s",
		e.File, e.Field, e.Value, strings.Join(IDs(), ", "),
	)
}

// Validate checks one axis field of a profile, role, or rule. file and field
// locate the value for the user. The error is an *InvalidError, which the cli
// layer maps onto exit code 3.
func Validate(file, field, id string) error {
	if Valid(id) {
		return nil
	}
	return &InvalidError{File: file, Field: field, Value: id}
}
