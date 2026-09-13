package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The §2.3 table is the whole of a pull request's state directory, so a
// directory that exists at all holds every file the table names.
func TestEnsurePRCreatesEveryStateFile(t *testing.T) {
	l := New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, l.Init())
	require.NoError(t, l.EnsurePR("acme", "web", 42))

	assert.Equal(t, []string{
		"meta.json", "claims.ndjson", "units.ndjson", "mapping.ndjson",
		"posted-index.ndjson", "intent-gaps.ndjson", "runs.ndjson",
		"threads.ndjson", "findings.ndjson", "probes.ndjson",
		"coverage.ndjson", "transitions.ndjson", "waivers.ndjson",
	}, PRFiles(), "the file set is the §2.3 table, in table order")

	for _, name := range PRFiles() {
		info, err := os.Stat(l.PRFile("acme", "web", 42, name))
		require.NoError(t, err, name)
		assert.False(t, info.IsDir(), name)
	}

	meta, err := l.ReadMeta("acme", "web", 42)
	require.NoError(t, err)
	assert.Equal(t, Meta{Owner: "acme", Repo: "web", PR: 42, ActiveRoles: []string{}}, meta,
		"a directory no round has been opened in carries the identity and nothing else")
}

// Creating the state directory a second time must never discard a recorded
// round: the files are created when missing, not written when present.
func TestEnsurePRKeepsRecordedState(t *testing.T) {
	l := unlockedPR(t)

	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&Meta{Owner: "acme", Repo: "web", PR: 42, Round: 3, Head: "0f1e2d3"}))
	require.NoError(t, held.Write(FileThreads, []byte("{\"id\":\"t1\"}\n")))
	require.NoError(t, held.Unlock())

	require.NoError(t, l.EnsurePR("acme", "web", 42))

	meta, err := l.ReadMeta("acme", "web", 42)
	require.NoError(t, err)
	assert.Equal(t, 3, meta.Round)
	assert.Equal(t, "0f1e2d3", meta.Head)

	body, err := l.ReadPR("acme", "web", 42, FileThreads)
	require.NoError(t, err)
	assert.Equal(t, "{\"id\":\"t1\"}\n", string(body))
}
