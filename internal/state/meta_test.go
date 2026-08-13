package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every field of the §2.3 meta.json row survives the writer and the reader.
func TestMetaSurvivesARoundTrip(t *testing.T) {
	l := lockedPR(t)
	want := Meta{
		Owner: "acme", Repo: "web", PR: 42,
		IssueKey:       "ACME-7",
		ProfileID:      "generic",
		ActiveRoles:    []string{"correctness", "convention"},
		Round:          2,
		Head:           "0f1e2d3",
		PostUnresolved: true,
	}

	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&want))
	require.NoError(t, held.Unlock())

	got, err := l.ReadMeta("acme", "web", 42)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

