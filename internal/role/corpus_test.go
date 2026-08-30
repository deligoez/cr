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

// The second half of §2.5.4, on its own: with no per-repository copy the global
// file must win over the built-in one, which is the case a user's edit to
// ~/.cr/roles/<id>.json depends on. Asserting it apart from the three-layer
// case is what separates a resolver that prefers the highest present layer from
// one that only ever prefers the repository.
func TestTheGlobalRoleWinsOverTheBuiltinOne(t *testing.T) {
	const id = "convention"
	require.Contains(t, Builtins(), id)

	globalFile := roleJSON(t, id, map[string]any{"title": "The global convention lens"})

	got, err := Resolve(absentDir(t), layerDir(t, map[string]string{id: globalFile}))
	require.NoError(t, err)

	resolved := entriesFor(got, id)
	require.Len(t, resolved, 1)
	assert.Equal(t, GlobalLayer, resolved[0].Layer)
	assert.Equal(t, "The global convention lens", resolved[0].Role.Title)
}

// §2.5.5 fixes corpus order, and §6.4.2 reads it to pick the representative of
// a duplicate group — so the order is a normative contract and not a rendering
// detail. Three things are asserted at once because they can only go wrong
// together:
//
//   - layer before id, so every per-repository role precedes every global one;
//   - ascending id *within* a layer, spelled with `a` and `a-b`, whose file
//     names sort the other way round — `a-b.json` is below `a.json` because `-`
//     is below `.` — so a resolver trusting os.ReadDir's order fails here and
//     passes on every other pair of ids;
//   - a shadowed id sitting at the winning layer's position: `correctness` is
//     global here, so it comes before `zz` and not among the built-ins it
//     displaced. §2.5.4 resolved it from the global layer, and that is the
//     layer §2.5.5 then orders it by.
func TestCorpusOrderIsLayerThenAscendingRoleID(t *testing.T) {
	require.Less(t, int(RepoLayer), int(GlobalLayer), "§2.5.4's order is the constants' order")
	require.Less(t, int(GlobalLayer), int(BuiltinLayer))

	repo := layerDir(t, map[string]string{
		"a-b": roleJSON(t, "a-b", nil),
		"a":   roleJSON(t, "a", nil),
	})
	global := layerDir(t, map[string]string{
		"zz":          roleJSON(t, "zz", nil),
		"correctness": roleJSON(t, "correctness", nil),
	})

	got, err := Resolve(repo, global)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"a", "a-b",
		"correctness", "zz",
		"convention", "intent-coverage", "test-adequacy",
	}, corpusIDs(got))

	assert.Equal(t, []Layer{
		RepoLayer, RepoLayer,
		GlobalLayer, GlobalLayer,
		BuiltinLayer, BuiltinLayer, BuiltinLayer,
	}, layersOf(got))
}

// layersOf reports the layer each corpus entry was resolved from, in corpus
// order.
func layersOf(roles []Resolved) []Layer {
	out := make([]Layer, 0, len(roles))
	for _, r := range roles {
		out = append(out, r.Layer)
	}
	return out
}

// §2.5.5 says more than one role MAY serve the same axis, so resolution
// produces a corpus rather than a lookup: keying by axis anywhere in it would
// silently drop one of these two, and the review would read as though both
// lenses had looked. They are placed at different layers on purpose — the
// second is the one a keyed resolver loses first.
func TestMoreThanOneRoleMayServeTheSameAxis(t *testing.T) {
	repo := layerDir(t, map[string]string{
		"security": roleJSON(t, "security", map[string]any{"axis": axis.Correctness}),
	})
	global := layerDir(t, map[string]string{
		"concurrency": roleJSON(t, "concurrency", map[string]any{"axis": axis.Correctness}),
	})

	got, err := Resolve(repo, global)
	require.NoError(t, err)

	serving := make([]string, 0, len(got))
	for _, r := range got {
		if r.Role.Axis == axis.Correctness {
			serving = append(serving, r.Role.ID)
		}
	}
	assert.Equal(t, []string{"security", "concurrency", "correctness"}, serving)
}

// §2.5.3 aborts on a malformed role file and names it, without qualifying which
// layer holds it. The shadowed case is the one worth writing down: a broken
// per-repository file skipped over would resolve to the global role while its
// author believes their override is in force, and a broken global file skipped
// over is a role the user edited and cr silently does not use. Both are the
// silent-wrong §1.6 forbids, so resolution reads every layer before it resolves
// anything.
func TestAMalformedRoleAbortsResolutionWhicheverLayerHoldsIt(t *testing.T) {
	const broken = "{"
	valid := roleJSON(t, "correctness", nil)

	for _, tc := range []struct {
		name         string
		repo, global map[string]string
		wantID       string
	}{
		{
			name:   "per-repository",
			repo:   map[string]string{"correctness": broken},
			global: map[string]string{},
			wantID: "correctness",
		},
		{
			name:   "global",
			repo:   map[string]string{},
			global: map[string]string{"convention": broken},
			wantID: "convention",
		},
		{
			name:   "global, shadowed by a valid per-repository copy",
			repo:   map[string]string{"correctness": valid},
			global: map[string]string{"correctness": broken},
			wantID: "correctness",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, global := layerDir(t, tc.repo), layerDir(t, tc.global)

			_, err := Resolve(repo, global)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed, "§2.5.3 aborts, and the cli layer codes that 3")
			assert.Equal(t, tc.wantID+fileExt, filepath.Base(malformed.File), "the abort names the file")
		})
	}
}

// The built-in layer goes through the same loader, so a shipped file cr could
// not read stops the run rather than being trusted unread. It cannot be reached
// through Resolve — a guard test already holds the four shipped files to §2.5 —
// so the parse is exercised where it lives, which is also what keeps the branch
// from being an unexecuted claim.
func TestAMalformedShippedRoleAbortsToo(t *testing.T) {
	_, err := parseAll(map[string]string{"correctness": "{"})

	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "correctness"+fileExt, malformed.File, "a shipped role is named as the file it ejects to")
}

