package profile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// wellFormed is the smallest profile satisfying every required field of §2.4,
// written for the stem "laravel-pest".
const wellFormed = `{
	"id": "laravel-pest",
	"match": {"files": ["artisan"], "globs": ["app/**/*.php"]},
	"axes": {"intent": true, "correctness": true, "convention": true, "test": false}
}`

// write puts content at a profile path with the given stem and returns it.
func write(t *testing.T, stem, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), stem+".json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// §2.4 gives tests.timeout_seconds and tests.output_tail_bytes defaults, and
// gives every other optional field no value at all. A profile that omits them
// must come back carrying the two defaults and empty lists rather than nil, so
// no consumer has to know which fields the file happened to set.
func TestParseAppliesTheDocumentedDefaults(t *testing.T) {
	path := write(t, "laravel-pest", wellFormed)

	p, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, "laravel-pest", p.ID)
	assert.Equal(t, []string{"artisan"}, p.Match.Files)
	assert.Equal(t, []string{"app/**/*.php"}, p.Match.Globs)
	assert.Equal(t, map[string]bool{
		"intent": true, "correctness": true, "convention": true, "test": false,
	}, p.Axes)
	assert.Equal(t, DefaultTimeoutSeconds, p.Tests.TimeoutSeconds)
	assert.Equal(t, DefaultOutputTailBytes, p.Tests.OutputTailBytes)
	// Every list is empty rather than nil, so a profile never serialises a
	// slice as null.
	assert.Equal(t, []string{}, p.Sandbox.Copy)
	assert.Equal(t, []string{}, p.Sandbox.Setup)
	assert.Equal(t, []string{}, p.Tests.Cmd)
	assert.Equal(t, []string{}, p.Tests.Globs)
	assert.NotNil(t, p.Rules)
	assert.Empty(t, p.Rules)
	// The remaining optional fields stay unset: §2.4 gives them no default.
	assert.Empty(t, p.Tests.FilterFlag)
	assert.Empty(t, p.Tests.CountPattern)
	assert.Empty(t, p.Tests.ProbePathTemplate)
	assert.Empty(t, p.Symbols.Lang)
}

// An explicit value must survive, or the default would be a ceiling instead of
// a fallback.
func TestParseKeepsExplicitOptionalValues(t *testing.T) {
	path := write(t, "laravel-pest", `{
		"id": "laravel-pest",
		"match": {"files": ["artisan"], "globs": ["app/**/*.php"]},
		"axes": {"test": true},
		"sandbox": {"copy": [".env"], "setup": ["composer install"]},
		"tests": {
			"cmd": ["./vendor/bin/pest"],
			"globs": ["tests/**/*Test.php"],
			"filter_flag": "--filter",
			"timeout_seconds": 120,
			"output_tail_bytes": 8192,
			"count_pattern": "Tests:\\s+(\\d+).*?(\\d+) failed",
			"probe_path_template": "tests/Feature/cr_probe_<probe-id>.php"
		},
		"rules": [{"id": "no-facades"}],
		"symbols": {"lang": "php"}
	}`)

	p, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, []string{".env"}, p.Sandbox.Copy)
	assert.Equal(t, []string{"composer install"}, p.Sandbox.Setup)
	assert.Equal(t, []string{"./vendor/bin/pest"}, p.Tests.Cmd)
	assert.Equal(t, []string{"tests/**/*Test.php"}, p.Tests.Globs)
	assert.Equal(t, "--filter", p.Tests.FilterFlag)
	assert.Equal(t, 120, p.Tests.TimeoutSeconds)
	assert.Equal(t, 8192, p.Tests.OutputTailBytes)
	assert.Equal(t, `Tests:\s+(\d+).*?(\d+) failed`, p.Tests.CountPattern)
	assert.Equal(t, "tests/Feature/cr_probe_<probe-id>.php", p.Tests.ProbePathTemplate)
	assert.Equal(t, "php", p.Symbols.Lang)
	// §2.6 owns the rule schema, so the profile carries its rules verbatim.
	require.Len(t, p.Rules, 1)
	assert.JSONEq(t, `{"id": "no-facades"}`, string(p.Rules[0]))
}
