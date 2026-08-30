package role

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roleJSON builds the bytes of a role file for id, satisfying every required
// field of §2.5, with overrides applied first: a value replaces that field, and
// nil removes it. Building the document rather than splicing strings is what
// lets a case say "this layer's copy has no focus" as a file rather than as a
// hope about the decoder.
func roleJSON(t *testing.T, id string, overrides map[string]any) string {
	t.Helper()
	doc := map[string]any{
		"id":           id,
		"title":        "Title of " + id,
		"axis":         axis.Correctness,
		"instructions": "Framing text for " + id + ".",
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
	return string(data)
}

// layerDir builds one resolution layer: a fresh directory holding one file per
// entry, named <id>.json per §2.2, with the entry's string as its bytes.
func layerDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for id, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+fileExt), []byte(content), 0o600))
	}
	return dir
}

// absentDir is a path where no directory exists.
func absentDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "never-created")
}

// corpusIDs reports the corpus as its role ids, in corpus order.
func corpusIDs(roles []Resolved) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, r.Role.ID)
	}
	return out
}

// entriesFor returns every corpus entry carrying the given role id. §2.5.4
// leaves the shadowed copies out of the corpus entirely, so a resolved id has
// exactly one entry and a caller can assert that rather than assume it.
func entriesFor(roles []Resolved, id string) []Resolved {
	out := make([]Resolved, 0, 1)
	for _, r := range roles {
		if r.Role.ID == id {
			out = append(out, r)
		}
	}
	return out
}

// §2.5.4 puts the per-repository layer above the global one and the global one
// above the built-in, and it resolves by *file*: the winning copy is taken
// whole. That word is what this case is about. The global copy here carries a
// focus list and a profiles list the per-repository copy omits, and a resolver
// that built its answer field by field — the shape §2.7 uses for configuration,
// where the layers merge key by key — would hand back a role wearing the
// repository's instructions and the global copy's questions. Nobody wrote that
// role, and a focus question an author deleted still being asked of their
// colleague is exactly the trust §1.6 says is spent once.
//
// The id is a shipped one, so all three layers really are in play rather than
// two and an empty shelf.
func TestThePerRepositoryRoleWinsWholeAcrossAllThreeLayers(t *testing.T) {
	const id = "correctness"
	require.Contains(t, Builtins(), id, "the case only spans three layers if cr ships this id")

	repoFile := roleJSON(t, id, map[string]any{
		"title":        "The repository's correctness lens",
		"axis":         axis.Convention,
		"instructions": "The repository's own framing.",
	})
	globalFile := roleJSON(t, id, map[string]any{
		"title":    "The global correctness lens",
		"focus":    []string{"A question the global copy asks?"},
		"profiles": []string{"go"},
	})

	got, err := Resolve(
		layerDir(t, map[string]string{id: repoFile}),
		layerDir(t, map[string]string{id: globalFile}),
	)
	require.NoError(t, err)

	resolved := entriesFor(got, id)
	require.Len(t, resolved, 1, "a shadowed copy is out of the corpus, not later in it")
	assert.Equal(t, RepoLayer, resolved[0].Layer)

	want, err := Parse(id+fileExt, []byte(repoFile))
	require.NoError(t, err)
	assert.Equal(t, want, resolved[0].Role, "the winning file is the whole answer")

	assert.Empty(t, resolved[0].Role.Focus, "an omitted focus stays omitted rather than inherited")
	assert.Empty(t, resolved[0].Role.Profiles, "and so does an omitted profiles list")
	assert.Equal(t, axis.Convention, resolved[0].Role.Axis, "even the axis comes from the winning file")
}

