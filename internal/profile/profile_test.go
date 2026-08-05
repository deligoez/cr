package profile

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// specFields is the §2.4 field table, in its own order, in the dotted spelling
// the spec uses.
var specFields = []string{
	"id",
	"match.files",
	"match.globs",
	"axes",
	"sandbox.copy",
	"sandbox.setup",
	"tests.cmd",
	"tests.globs",
	"tests.filter_flag",
	"tests.timeout_seconds",
	"tests.output_tail_bytes",
	"tests.count_pattern",
	"tests.probe_path_template",
	"rules",
	"symbols.lang",
}

// §2.4's table is the whole profile surface, and a profile is mechanical,
// language-specific data rather than prompt text. Both hold only if the struct
// carries exactly the table's rows: a missing row drops configuration the spec
// promised, and an extra one is the crack a free-form instruction field would
// come through. Walking the struct reflectively catches either, including a
// field added years from now.
func TestProfileCarriesExactlyTheSpecFields(t *testing.T) {
	assert.ElementsMatch(t, specFields, fieldPaths(reflect.TypeOf(Profile{}), ""))
}

// fieldPaths returns the dotted json names of every leaf field, descending into
// nested structs only. A map or a slice is a leaf: it is one row of the table,
// whatever it holds.
func fieldPaths(t reflect.Type, prefix string) []string {
	paths := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		name := field.Tag.Get("json")
		if prefix != "" {
			name = prefix + "." + name
		}
		if field.Type.Kind() == reflect.Struct {
			paths = append(paths, fieldPaths(field.Type, name)...)
			continue
		}
		paths = append(paths, name)
	}
	return paths
}
