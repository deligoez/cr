package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/deligoez/cr/internal/axis"
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
	"tests.failed_pattern",
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
	assert.ElementsMatch(t, specFields, fieldPaths(reflect.TypeFor[Profile](), ""))
}

// fieldPaths returns the dotted json names of every leaf field, descending into
// nested structs only. A map or a slice is a leaf: it is one row of the table,
// whatever it holds.
func fieldPaths(t reflect.Type, prefix string) []string {
	paths := make([]string, 0, t.NumField())
	for field := range t.Fields() {
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
	assert.Empty(t, p.Tests.FailedPattern)
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
			"count_pattern": "(\\d+) (?:passed|failed)",
			"failed_pattern": "(\\d+) failed",
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
	assert.Equal(t, `(\d+) (?:passed|failed)`, p.Tests.CountPattern)
	assert.Equal(t, `(\d+) failed`, p.Tests.FailedPattern)
	assert.Equal(t, "tests/Feature/cr_probe_<probe-id>.php", p.Tests.ProbePathTemplate)
	assert.Equal(t, "php", p.Symbols.Lang)
	// §2.6 owns the rule schema, so the profile carries its rules verbatim.
	require.Len(t, p.Rules, 1)
	assert.JSONEq(t, `{"id": "no-facades"}`, string(p.Rules[0]))
}

// §2.5 item 3 makes a malformed profile abort with exit code 3 naming the file
// and the offending field. Every required row of the §2.4 table is a way to be
// malformed, so each one is checked to name itself rather than to fail somewhere
// downstream where the user cannot act on it.
func TestParseNamesTheMissingRequiredField(t *testing.T) {
	cases := map[string]struct {
		content string
		field   string
	}{
		"id": {`{
			"match": {"files": [], "globs": []},
			"axes": {}
		}`, "id"},
		"match": {`{"id": "generic", "axes": {}}`, "match"},
		"match.files": {`{
			"id": "generic",
			"match": {"globs": []},
			"axes": {}
		}`, "match.files"},
		"match.globs": {`{
			"id": "generic",
			"match": {"files": []},
			"axes": {}
		}`, "match.globs"},
		"axes": {`{"id": "generic", "match": {"files": [], "globs": []}}`, "axes"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := write(t, "generic", tc.content)

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, tc.field, malformed.Field)
			assert.Contains(t, err.Error(), path)
			assert.Contains(t, err.Error(), tc.field)
		})
	}
}

// §2.4 makes tests.globs required exactly when tests.cmd is present, because a
// runner with no way to recognise a test file cannot seed the gap probe's path.
// An absent tests.cmd disables the test axis and asks nothing.
func TestTestsGlobsIsRequiredWhenTestsCmdIsPresent(t *testing.T) {
	base := `{
		"id": "generic",
		"match": {"files": [], "globs": ["**/*"]},
		"axes": {"convention": true},
		"tests": %s
	}`

	_, err := Load(write(t, "generic", fmt.Sprintf(base, `{"cmd": ["make", "test"]}`)))
	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "tests.globs", malformed.Field)

	// An empty list is no better than an absent one.
	_, err = Load(write(t, "generic", fmt.Sprintf(base, `{"cmd": ["make", "test"], "globs": []}`)))
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "tests.globs", malformed.Field)

	// Without a command the test axis is off, so globs are not asked for.
	p, err := Load(write(t, "generic", fmt.Sprintf(base, `{"filter_flag": "-run"}`)))
	require.NoError(t, err)
	assert.Empty(t, p.Tests.Cmd)

	// A present but empty command is neither absent nor runnable.
	_, err = Load(write(t, "generic", fmt.Sprintf(base, `{"cmd": [], "globs": ["*_test.go"]}`)))
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "tests.cmd", malformed.Field)
}

// §2.4 makes id equal to the file stem. A profile whose id disagrees with its
// filename is addressable by two different names, so the disagreement is a
// malformed file rather than a preference.
func TestParseRequiresTheIDToEqualTheFileStem(t *testing.T) {
	path := write(t, "generic", `{
		"id": "laravel-pest",
		"match": {"files": [], "globs": ["**/*"]},
		"axes": {}
	}`)

	_, err := Load(path)

	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "id", malformed.Field)
	assert.Contains(t, err.Error(), `"generic"`)
	assert.Contains(t, err.Error(), `"laravel-pest"`)
}

// §1.5 closes the axis id set, so an axes key outside it is a bad configuration
// file. The judgement belongs to axis.Validate, which the cli layer already maps
// onto exit code 3; the profile only has to route every key through it.
func TestParseRejectsAnAxisIDOutsideTheClosedSet(t *testing.T) {
	path := write(t, "generic", `{
		"id": "generic",
		"match": {"files": [], "globs": ["**/*"]},
		"axes": {"correctness": true, "security": true}
	}`)

	_, err := Load(path)

	var invalid *axis.InvalidError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "axes.security", invalid.Field)
	assert.Equal(t, path, invalid.File)
}

// §2.4 fixes tests.count_pattern at exactly one capture group, a count of
// executed tests, and makes any other group count abort. One is a boundary with
// a side on each of it, so the count is walked from zero to two: zero leaves
// §5.2.1 with no group to read a number from, and two gives it no rule for
// which one it must sum.
//
// Go counts capturing groups only, which is the reading §5.2.1 needs — it
// indexes submatches — so a pattern is checked to be judged by what it captures
// rather than by how many parentheses it contains.
//
// Every case carries a well-formed failed_pattern, so what fails is the arity
// of the count pattern and never its missing companion.
func TestACountPatternMustCaptureExactlyOneGroup(t *testing.T) {
	cases := map[string]struct {
		pattern string
		groups  int
	}{
		"zero groups":          {`\d+ passed`, 0},
		"one group":            {`(\d+) passed`, 1},
		"two groups":           {`(\d+) passed, (\d+) failed`, 2},
		"only non-capturing":   {`(?:\d+) passed`, 0},
		"named groups capture": {`(?P<run>\d+) passed`, 1},
		"non-capturing groups do not count": {
			`(?:Tests:)\s+(\d+) passed`, 1,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := write(t, "generic", fmt.Sprintf(`{
				"id": "generic",
				"match": {"files": [], "globs": ["**/*"]},
				"axes": {"test": true},
				"tests": {"count_pattern": %q, "failed_pattern": "(\\d+) failed"}
			}`, tc.pattern))

			p, err := Load(path)

			if tc.groups == 1 {
				require.NoError(t, err)
				assert.Equal(t, tc.pattern, p.Tests.CountPattern)
				return
			}
			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "tests.count_pattern", malformed.Field)
			assert.Equal(t, path, malformed.File)
			// The message names the count it found, so the fix is
			// visible without counting parentheses by hand.
			assert.Contains(t, err.Error(), fmt.Sprintf("has %d capture groups", tc.groups))
		})
	}
}

// §2.4 makes tests.failed_pattern required exactly when tests.count_pattern is
// present, and §5.2.1 says what rests on it: a failed pattern that never
// matches yields zero rather than nothing, so a profile configuring only the
// executed count would read every run as having no failures at all. That is the
// one misconfiguration cr cannot afford to accept quietly — it manufactures the
// `no-test-failed` result §5.3.5 lets a probe assert on.
//
// The two fields are therefore checked together: neither is a fault, both is
// the working shape, and the failed pattern is held to the same single group as
// its companion.
func TestAFailedPatternIsRequiredBesideTheCountPattern(t *testing.T) {
	t.Run("neither pattern is configured", func(t *testing.T) {
		path := write(t, "generic", `{
			"id": "generic",
			"match": {"files": [], "globs": ["**/*"]},
			"axes": {"test": true},
			"tests": {"cmd": ["make", "test"], "globs": ["*_test.go"]}
		}`)

		p, err := Load(path)

		require.NoError(t, err)
		assert.Empty(t, p.Tests.CountPattern)
		assert.Empty(t, p.Tests.FailedPattern)
	})

	t.Run("a count pattern without a failed pattern", func(t *testing.T) {
		path := write(t, "generic", `{
			"id": "generic",
			"match": {"files": [], "globs": ["**/*"]},
			"axes": {"test": true},
			"tests": {"count_pattern": "(\\d+) passed"}
		}`)

		_, err := Load(path)

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Equal(t, "tests.failed_pattern", malformed.Field)
		assert.Equal(t, path, malformed.File)
		assert.Contains(t, err.Error(), "is required when tests.count_pattern is present")
	})

	t.Run("a failed pattern with two groups", func(t *testing.T) {
		path := write(t, "generic", `{
			"id": "generic",
			"match": {"files": [], "globs": ["**/*"]},
			"axes": {"test": true},
			"tests": {
				"count_pattern": "(\\d+) passed",
				"failed_pattern": "(\\d+) failed of (\\d+)"
			}
		}`)

		_, err := Load(path)

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		// The abort names the companion, not the count pattern it sits
		// beside, so the author edits the field that is actually wrong.
		assert.Equal(t, "tests.failed_pattern", malformed.Field)
		assert.Contains(t, err.Error(), "has 2 capture groups")
		assert.Contains(t, err.Error(), "a count of failed tests")
	})

	t.Run("both patterns with one group", func(t *testing.T) {
		path := write(t, "generic", `{
			"id": "generic",
			"match": {"files": [], "globs": ["**/*"]},
			"axes": {"test": true},
			"tests": {
				"count_pattern": "(\\d+) (?:passed|failed)",
				"failed_pattern": "(\\d+) failed"
			}
		}`)

		p, err := Load(path)

		require.NoError(t, err)
		assert.Equal(t, `(\d+) (?:passed|failed)`, p.Tests.CountPattern)
		assert.Equal(t, `(\d+) failed`, p.Tests.FailedPattern)
	})
}

// A field of the wrong type, an unparseable file, and an unreadable one are all
// profile files cr cannot use. Each must name the file, and a type error must
// name the field too, so the user is told what to open and what to fix.
func TestParseRejectsUnusableFiles(t *testing.T) {
	typed := write(t, "generic", `{
		"id": "generic",
		"match": {"files": [], "globs": ["**/*"]},
		"axes": {},
		"tests": {"timeout_seconds": "fast"}
	}`)
	_, err := Load(typed)
	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "tests.timeout_seconds", malformed.Field)

	notJSON := write(t, "generic", "id = generic\n")
	_, err = Load(notJSON)
	require.ErrorAs(t, err, &malformed)
	assert.Contains(t, err.Error(), notJSON)

	missing := filepath.Join(t.TempDir(), "generic.json")
	_, err = Load(missing)
	require.ErrorAs(t, err, &malformed)
	assert.Contains(t, err.Error(), missing)
}
