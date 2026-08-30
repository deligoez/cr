package role

import (
	"encoding/json"
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
