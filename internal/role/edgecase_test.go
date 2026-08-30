package role

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// edgeCaseSetting is the key an earlier draft of §4.4 bounded an enumeration
// with, and that the spec no longer has. It is written out as a literal because
// the point of the test is that nothing in the package defines it.
const edgeCaseSetting = "test.max_edge_cases"

// Edge-case reasoning changed owner rather than disappearing, and the two
// halves of that trade are only worth anything together.
//
// Counting edge cases would have made cr form the opinion invariant 1 of
// CLAUDE.md and §1.4 keep it out of: a cap answers "how many are enough", which
// is a judgement about a specific change that no number reached from a config
// file can hold. So the negative half is asserted against the resolver rather
// than against the spec text — grepping the document for the string would prove
// the sentence was deleted while a live setting kept working underneath it.
//
// It is asserted at the strongest form that is true. §2.7's table is the whole
// configuration surface and `apply` drops a name absent from it, so the claim is
// not merely that the key has no default: it is that all four layers above the
// defaults may each name it at once, in the nested spelling and the dotted one
// and the environment spelling, and the resolution still succeeds carrying no
// such key and no such value. A weaker test asserting only the missing default
// would pass on a build where a file could introduce arbitrary keys.
//
// The positive half is where the reasoning went. A role is a lens, and telling
// an agent how to think about boundaries is exactly what a lens is for, so the
// shipped test-adequacy role carries it — under the same register the rest of
// that prose holds, since an edge case nobody probed is a suspicion like any
// other.
func TestEdgeCaseReasoningIsRoleGuidanceAndNotASetting(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.json")
	repo := filepath.Join(dir, "repo.json")
	nested := []byte(`{"test": {"max_edge_cases": 7}}`)
	require.NoError(t, os.WriteFile(global, nested, 0o600))
	require.NoError(t, os.WriteFile(repo, nested, 0o600))

	resolved, err := config.Resolve(config.Sources{
		Flags:        map[string]any{edgeCaseSetting: 7},
		Environ:      []string{"CR_TEST_MAX_EDGE_CASES=7"},
		RepoConfig:   repo,
		GlobalConfig: global,
	})
	require.NoError(t, err, "a name that is not a setting configures nothing and aborts nothing")

	assert.NotContains(t, resolved.Map(), edgeCaseSetting,
		"%s resolves as a key; §4.4 bounds no edge-case enumeration and §2.7's table has no such setting",
		edgeCaseSetting)
	assert.Zero(t, resolved.Int(edgeCaseSetting),
		"%s carries a value, so some layer reached a setting cr does not have", edgeCaseSetting)

	r, err := Parse(testAdequacyID+".json", []byte(Builtins()[testAdequacyID]))
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(r.Instructions), "edge case",
		"the test lens must carry edge-case reasoning, which is the only place it lives now")
}
