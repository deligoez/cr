package proposal

import (
	"slices"

	"github.com/deligoez/cr/internal/finding"
)

// fanOutPrefix and fanOutSuffix are the name §4.6.2 gives the file one role
// writes its proposals for one unit to.
//
// It is a second file beside the role's records rather than a second kind of
// line in one: §6.1's decoder and §5.7's refuse different shapes, and a file
// holding both would have to guess which refusal a malformed line earned.
const (
	fanOutPrefix = "proposals-"
	fanOutSuffix = ".ndjson"
)

// FanOutFile is the file §4.6.2 has one role write its proposals to.
func FanOutFile(role string) string {
	return fanOutPrefix + role + fanOutSuffix
}

// Field is one row of §5.7's table: the name it goes by on the wire, and what
// the Required column answers for it.
//
// The requirement vocabulary is internal/finding's rather than one of this
// package's. §5.7's column answers the same four things §6.1's does, and a
// second set of words would let a prompt describe a proposal's `state` and a
// record's `grade` differently while both mean "cr writes it".
type Field struct {
	// Name is the field's JSON name.
	Name string
	// Requirement is what §5.7's Required column answers.
	Requirement finding.Requirement
}

// fields is §5.7's table in the order the section prints it.
var fields = []Field{
	{"id", finding.Required},
	{"kind", finding.Required},
	{"role", finding.Required},
	{"unit", finding.Required},
	{"finding", finding.Optional},
	{"target", finding.Required},
	{"hypothesis", finding.Required},
	{"settles", finding.Required},
	{"input", finding.Required},
	{"filter", finding.Optional},
	{"paths", finding.Optional},
	{"state", finding.Computed},
	{"probe", finding.Computed},
	{"reason", finding.Computed},
	{"round", finding.Stamped},
	{"head", finding.Stamped},
}

// Fields returns §5.7's rows in table order. The result is a copy, so a caller
// can neither widen the table nor reorder it.
func Fields() []Field {
	return append(make([]Field, 0, len(fields)), fields...)
}

// Reserved returns every field a proposal may not arrive carrying: §5.7's
// computed rows and §2.3.3's two stamps, in table order.
//
// It is the list the decoder refuses by, handed out rather than restated, so a
// prompt telling a role which fields it may not write names exactly the fields
// `cr proposals record` will reject.
func Reserved() []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.Requirement == finding.Computed || field.Requirement == finding.Stamped {
			names = append(names, field.Name)
		}
	}
	return names
}

// Kinds returns §5.7's two kinds in the order the section names them.
func Kinds() []string {
	return slices.Clone(kinds)
}
