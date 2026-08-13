package finding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/deligoez/cr/internal/state"
)

// RejectedRecordError reports a record §6.1.3 refuses.
//
// §6.1.3 gives all of its rejections one shape — exit code 1, naming the line
// and the field — so they share one type. The cli layer maps it onto exit code
// 1: the file was found, read, and parsed, and what is wrong is the data inside
// it.
type RejectedRecordError struct {
	// File is the NDJSON file the agent handed the command.
	File string
	// Line is the one-based line the record sits on, counting blank lines.
	Line int
	// Field is the field at fault, named by its JSON key so the user can
	// find it in the line the error points at.
	Field string
	// Problem is what is wrong with that field.
	Problem string
}

func (e *RejectedRecordError) Error() string {
	return fmt.Sprintf("%s line %d: %s %s", e.File, e.Line, e.Field, e.Problem)
}

// UnattributableFileError reports an input `cr merge` cannot bind to a role.
//
// §6.5.1 has `cr merge` read per-role files, and §6.1.3 has it reject a record
// whose role is not the role whose file it arrived in. A name that carries no
// role leaves nothing to compare against, so the whole file's `role` fields
// would revert to being taken on the agent's word — and with them the axis
// §6.2 grades on. Refusing the file is the only answer that keeps the binding
// worth anything.
type UnattributableFileError struct {
	// File is the input as it was named on the command line.
	File string
}

func (e *UnattributableFileError) Error() string {
	return fmt.Sprintf(
		"%s: cr merge reads the §4.6.2 fan-out output files, named %s; this name binds its records to no role",
		e.File, FanOutFile("<role-id>"),
	)
}

// Decode reads the records an agent hands `cr record`, holding every line to
// §6.1.3.
//
// units is the id of every unit of the current round: §9.3.5 scopes a command
// to that round, and §6.1.3 rejects a record naming any other unit. That check
// proves the unit **exists**, never that it is the record's own. Nothing here
// stops a record anchored in u1 from declaring `unit: u2`, and §6.2.1's
// containment predicate is what has to catch it — a citation into the record's
// own hunk would otherwise count as outside the unit it named and buy the
// `cited` grade. The two checks are separate and only one of them is here.
//
// The role binding applies whenever the file's name carries a role, so feeding
// a single role's fan-out file straight to `cr record` is checked exactly as
// `cr merge` would have checked it. §6.5.1's merged output carries records from
// several roles and its name carries none; those `role` fields were bound by
// `cr merge` at the file it read each record from, and no name can re-derive
// them here.
func Decode(file string, body []byte, units []string) ([]*Finding, error) {
	role, _ := RoleForFile(file)
	against := checker{file: file, units: units, role: role}
	return state.DecodeStamped[Finding](file, body, against.check)
}

// DecodePerRole reads one role's §4.6.2 output file for `cr merge`, under the
// same rules, and refuses an input whose name binds its records to no role.
func DecodePerRole(file string, body []byte, units []string) ([]*Finding, error) {
	if _, bound := RoleForFile(file); !bound {
		return nil, &UnattributableFileError{File: file}
	}
	return Decode(file, body, units)
}

// checker holds what one file's records are checked against: the file they
// arrived in, the units of the current round, and the role the file's name
// binds them to, empty when it binds none.
type checker struct {
	file  string
	units []string
	role  string
}

// check holds one line to §6.1.3.
//
// Presence is read from the wire rather than from the decoded record, for the
// reason state.DecodeStamped already gives about head and round: a struct that
// decoded cannot say which keys were there, and `""` is a value the agent chose
// exactly as much as a sentence is. The remaining checks read the decoded
// record, because by then the field is known to be present.
//
// Required fields are walked in §6.1's table order, so a record missing several
// is always reported by the same one.
func (c checker) check(line int, supplied map[string]json.RawMessage, record *Finding) error {
	for _, field := range fields {
		if field.Requirement == Required && !written(supplied[field.Name]) {
			return &RejectedRecordError{
				File: c.file, Line: line, Field: field.Name,
				Problem: "is required by §6.1 and this record does not supply it",
			}
		}
	}
	if record.Anchor.Path == "" {
		return &RejectedRecordError{
			File: c.file, Line: line, Field: "anchor",
			Problem: "names no path, so this is an item with no code location and never becomes a record (§6.1.2, §4.1.3)",
		}
	}
	if !slices.Contains(c.units, record.Unit) {
		return &RejectedRecordError{
			File: c.file, Line: line, Field: "unit",
			Problem: fmt.Sprintf("%q is not a unit of the current round", record.Unit),
		}
	}
	if c.role != "" && record.Role != c.role {
		return &RejectedRecordError{
			File: c.file, Line: line, Field: "role",
			Problem: fmt.Sprintf(
				"%q is not %q, the role whose §4.6.2 output file this is",
				record.Role, c.role,
			),
		}
	}
	return nil
}

// The two values a JSON object can hold under a key and still supply nothing.
var (
	nullLiteral = []byte("null")
	emptyString = []byte(`""`)
)

// written reports whether a wire line supplied a field.
//
// A key that is not there arrives as a nil value, which is the plain case.
// null and "" are the two ways a line can hold the key and supply nothing
// under it, and §6.1.3 reads both as missing: a record whose evidence is the
// empty string has no evidence, and a validator that passed it would let a
// record reach the assertion register with nothing behind it.
func written(value json.RawMessage) bool {
	value = bytes.TrimSpace(value)
	return len(value) > 0 &&
		!bytes.Equal(value, nullLiteral) &&
		!bytes.Equal(value, emptyString)
}
