package role

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// specFields is the §2.5 field table, in its own order. It is written out here
// rather than taken from the package, because a guard that read the value it
// judges would pass whatever the package happened to say.
var specFields = []string{"id", "title", "axis", "instructions", "focus", "profiles"}

// write puts content at a role path with the given stem and returns it.
func write(t *testing.T, stem, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), stem+".json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// roleFile writes a role file at the given stem satisfying every required field
// of §2.5, with overrides applied first: a value replaces that field, and nil
// removes it. Building the document rather than splicing strings is what lets a
// case say "title is absent" and "title is blank" as two different files.
func roleFile(t *testing.T, stem string, overrides map[string]any) string {
	t.Helper()
	doc := map[string]any{
		"id":           stem,
		"title":        "Test adequacy",
		"axis":         axis.Test,
		"instructions": "Judge whether the changed behaviour is exercised.",
	}
	for key, value := range overrides {
		if value == nil {
			delete(doc, key)
			continue
		}
		doc[key] = value
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return write(t, stem, string(data))
}

// §2.5's table is the whole role surface, and the sentence above it — cr owns
// the output contract, a role only supplies persona and focus — holds only if
// the struct carries exactly the table's rows. A missing row drops something
// the spec promised; an extra one is the crack an output path or a §6.1.4 stamp
// field would come through. Walking the struct reflectively catches either,
// including a field added years from now.
//
// The allowlist is held to the same literal, because it is the runtime half of
// the same claim. The struct alone only makes an unknown key inert; the
// allowlist is what makes it audible, and a key it forgot would be decoded into
// nothing and reported as nothing.
func TestRoleCarriesExactlyTheSpecFields(t *testing.T) {
	names := make([]string, 0, reflect.TypeFor[Role]().NumField())
	for field := range reflect.TypeFor[Role]().Fields() {
		names = append(names, field.Tag.Get("json"))
	}

	assert.Equal(t, specFields, names)
	assert.Equal(t, specFields, fields,
		"§2.5's table is the allowlist; a key missing from it is a key cr would silently ignore")
}

// §2.5 fixes the id twice over: kebab-case, and equal to the file stem. The
// second is what makes it unspoofable, the way §4.6.2's output paths are — the
// stem already names the role, so a file stating a different id is a
// contradiction with no correct resolution, and cr refuses it rather than
// preferring one half over the other. Kebab-case then constrains the filename
// as much as the field, because the two are the same string.
func TestTheIDMustBeTheKebabCaseFileStem(t *testing.T) {
	t.Run("a well-formed file takes its id from the stem", func(t *testing.T) {
		r, err := Load(roleFile(t, "test-adequacy", nil))
		require.NoError(t, err)
		assert.Equal(t, "test-adequacy", r.ID)
	})

	for _, c := range []struct {
		name, stem string
		id         any
		says       string
	}{
		{"absent", "correctness", nil, "is required"},
		{"naming another role", "correctness", "convention", `"convention"`},
		{"capitalised", "Correctness", "Correctness", "kebab-case"},
		{"hyphen-terminated", "correctness-", "correctness-", "kebab-case"},
		{"underscored", "test_adequacy", "test_adequacy", "kebab-case"},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := roleFile(t, c.stem, map[string]any{"id": c.id})

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "id", malformed.Field)
			assert.Contains(t, err.Error(), path)
			assert.Contains(t, err.Error(), c.says)
		})
	}
}

// §2.5 marks title, axis, and instructions required, and §2.5.3 requires the
// abort to name the offending field: a message saying only "malformed role"
// leaves the user opening every field by hand. Blank counts as absent, because
// a title of " " is not a human label and instructions of " " frame nothing,
// and the fix is the same one either way.
func TestEveryRequiredTextFieldIsNamedWhenItIsBlank(t *testing.T) {
	for _, field := range []string{"title", "axis", "instructions"} {
		for _, value := range []any{nil, "", "   "} {
			t.Run(fmt.Sprintf("%s is %q", field, value), func(t *testing.T) {
				path := roleFile(t, "test-adequacy", map[string]any{field: value})

				_, err := Load(path)

				var malformed *MalformedError
				require.ErrorAs(t, err, &malformed)
				assert.Equal(t, field, malformed.Field)
				assert.Equal(t, "is required", malformed.Problem)
				assert.Contains(t, err.Error(), path)
			})
		}
	}
}

// §1.5 closes the axis id set and axis.Validate is the one place that judges
// it, so a role naming an axis of its own is rejected there rather than by a
// second copy of the rule here. §1.5 and §2.5.3 fix the same abort, and the
// error carries the file and the field either way — a role package that wrapped
// it in a MalformedError of its own would be the second judge §1.5 exists to
// prevent.
func TestAnAxisOutsideTheClosedSetIsRejected(t *testing.T) {
	path := roleFile(t, "security", map[string]any{"axis": "security"})

	_, err := Load(path)

	var invalid *axis.InvalidError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, path, invalid.File)
	assert.Equal(t, "axis", invalid.Field)
	assert.Equal(t, "security", invalid.Value)
}

// A role file cr cannot read or decode has no field to blame, and its message
// must not leave a gap where one would go. A type error is the case in between:
// the decoder knows which field it choked on, so the abort names it like every
// other fault instead of falling back to the whole file.
//
// Both spellings of the message are pinned, because nothing else distinguishes
// them and nothing else notices if they merge.
func TestAFileCrCannotReadOrDecodeSaysWhatItCan(t *testing.T) {
	t.Run("unreadable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "correctness.json")

		_, err := Load(path)

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Equal(t, path, malformed.File)
		assert.Empty(t, malformed.Field)
		assert.Contains(t, err.Error(), "cannot be read")
		assert.Equal(t, path+": "+malformed.Problem, malformed.Error())
	})

	t.Run("not JSON at all", func(t *testing.T) {
		path := write(t, "correctness", "instructions: judge the diff")

		_, err := Load(path)

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Empty(t, malformed.Field)
		assert.Contains(t, err.Error(), "is not valid JSON")
		assert.Equal(t, path+": "+malformed.Problem, malformed.Error())
	})

	t.Run("a focus question written as one string", func(t *testing.T) {
		path := roleFile(t, "correctness", map[string]any{"focus": "Which branch is unexercised?"})

		_, err := Load(path)

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Equal(t, "focus", malformed.Field)
		assert.Contains(t, err.Error(), path)
		assert.Equal(t, path+": focus "+malformed.Problem, malformed.Error())
	})
}
