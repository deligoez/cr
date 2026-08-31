package activation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/role"
)

// shippedCorpus is §2.5.5's corpus with neither on-disk layer present, so it is
// the four built-in roles of §2.5.1 in the order §2.5.5 fixes.
func shippedCorpus(t *testing.T) []role.Resolved {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "absent")
	corpus, err := role.Resolve(filepath.Join(empty, "repo"), filepath.Join(empty, "global"))
	require.NoError(t, err)
	require.Len(t, corpus, 4, "§2.5.1 ships four roles, one per axis")
	return corpus
}

// scopedCorpus is the shipped corpus plus one global role whose `profiles` list
// names profiles. §2.5.1 ships no such role, and the list is the half of §4.5.1
// the shipped four cannot exercise at all.
func scopedCorpus(t *testing.T, id string, profiles string) []role.Resolved {
	t.Helper()
	global := filepath.Join(t.TempDir(), "roles")
	require.NoError(t, os.MkdirAll(global, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(global, id+".json"), []byte(`{
	  "id": "`+id+`",
	  "title": "Scoped",
	  "axis": "correctness",
	  "instructions": "Look at the change.",
	  "profiles": `+profiles+`
	}`), 0o600))

	corpus, err := role.Resolve(filepath.Join(t.TempDir(), "absent"), global)
	require.NoError(t, err)
	return corpus
}

// §4.5.1's role half, computed with `cr review` never invoked — which it cannot
// be, because the active set is what `cr review` fans out over.
//
// That is the round-8 `circular-definition` finding stated as a test. §4.5.1
// reads "its axis is active, its `profiles` list is empty or names the resolved
// profile, and §4.6.4 did not report it skipped", and the third clause is a
// report `cr review` emits over this very set. Taken as an input it would make
// activeness undefined until the command that consumes it has run, while §4.5.6
// rejects a cell naming an inactive role and §10.2.2 counts a complete row of
// cells per active role — neither of which may wait on a fan-out. So the answer
// below rests on two inputs and no third, and every case reads it off an
// Activation and a corpus alone.
func TestRoleActivenessRestsOnTheAxisAndTheProfileAlone(t *testing.T) {
	for _, tc := range []struct {
		name    string
		active  []string
		corpus  []role.Resolved
		profile string
		roles   []string
	}{
		{
			// Every axis active: the corpus is the active set,
			// order included.
			name:    "every axis active admits every shipped role",
			active:  []string{"intent", "correctness", "convention", "test"},
			corpus:  shippedCorpus(t),
			profile: "laravel-pest",
			roles:   []string{"convention", "correctness", "intent-coverage", "test-adequacy"},
		},
		{
			// §4.5.2's disabled test axis, read through the role
			// half: the role on that axis is not active, and the
			// other three are untouched.
			name:    "a role on a disabled axis is not active",
			active:  []string{"intent", "correctness", "convention"},
			corpus:  shippedCorpus(t),
			profile: "generic",
			roles:   []string{"convention", "correctness", "intent-coverage"},
		},
		{
			// §4.5.3's unavailable intent axis, likewise. An absent
			// tracker takes one role out and leaves three.
			name:    "a role on an unavailable axis is not active",
			active:  []string{"correctness", "convention", "test"},
			corpus:  shippedCorpus(t),
			profile: "laravel-pest",
			roles:   []string{"convention", "correctness", "test-adequacy"},
		},
		{
			// No axis ran at all, which is §2.4.4's repository. The
			// active set is empty rather than the corpus.
			name:    "no active axis admits no role",
			active:  []string{},
			corpus:  shippedCorpus(t),
			profile: "",
			roles:   []string{},
		},
		{
			// §4.5.1's `profiles` clause, positive half.
			name:    "a profiles list naming the resolved profile admits the role",
			active:  []string{"correctness"},
			corpus:  scopedCorpus(t, "scoped", `["laravel-pest", "generic"]`),
			profile: "laravel-pest",
			roles:   []string{"scoped", "correctness"},
		},
		{
			// Negative half: the axis is active and the list names
			// some other profile, so the role is out while the role
			// with an empty list on the same axis stays in.
			name:    "a profiles list naming another profile excludes the role",
			active:  []string{"correctness"},
			corpus:  scopedCorpus(t, "scoped", `["generic"]`),
			profile: "laravel-pest",
			roles:   []string{"correctness"},
		},
		{
			// The empty list is "all profiles" and not "no
			// profile", which is the reading §2.5's table fixes and
			// the one an emptiness check gets backwards.
			name:    "an empty profiles list admits the role under any profile",
			active:  []string{"correctness"},
			corpus:  scopedCorpus(t, "scoped", `[]`),
			profile: "laravel-pest",
			roles:   []string{"scoped", "correctness"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := Activation{Active: tc.active}

			assert.Equal(t, tc.roles, a.ActiveRoles(tc.corpus, tc.profile))
		})
	}
}

// ids reduces a corpus to the role ids it holds, in corpus order.
func ids(corpus []role.Resolved) []string {
	out := make([]string, 0, len(corpus))
	for i := range corpus {
		out = append(out, corpus[i].Role.ID)
	}
	return out
}

// The active set is a subset of the corpus and keeps its order.
//
// §2.5.5 fixes the corpus order and §6.4.2 reads it to pick a duplicate group's
// representative, so a filter that sorted its own result would hand cr a second
// order to disagree with. The case that shows it is a per-repository override:
// it resolves from the highest layer and therefore precedes the built-ins,
// which alphabetical order of the ids would not produce.
func TestTheActiveSetKeepsCorpusOrder(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "roles")
	require.NoError(t, os.MkdirAll(repo, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "zeta.json"), []byte(`{
	  "id": "zeta",
	  "title": "Zeta",
	  "axis": "correctness",
	  "instructions": "Look at the change."
	}`), 0o600))

	corpus, err := role.Resolve(repo, filepath.Join(t.TempDir(), "absent"))
	require.NoError(t, err)
	require.Equal(t, "zeta", ids(corpus)[0], "a per-repository role precedes the built-ins per §2.5.5")

	a := Activation{Active: []string{"intent", "correctness", "convention", "test"}}

	assert.Equal(t, ids(corpus), a.ActiveRoles(corpus, "laravel-pest"),
		"every role is active here, so the active set is the corpus in corpus order")
}
