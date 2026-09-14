package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every field of the §2.3 meta.json row survives the writer and the reader.
func TestMetaSurvivesARoundTrip(t *testing.T) {
	l := unlockedPR(t)
	want := Meta{
		Owner: "acme", Repo: "web", PR: 42,
		IssueKey:       "ACME-7",
		ProfileID:      "generic",
		ActiveRoles:    []string{"correctness", "convention"},
		Round:          2,
		Head:           "0f1e2d3",
		PostUnresolved: true,
		MappingRound:   2,
		MappingHead:    "0f1e2d3",
		ClaimsRound:    2,
		ClaimsHead:     "0f1e2d3",
	}

	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&want))
	require.NoError(t, held.Unlock())

	got, err := l.ReadMeta("acme", "web", 42)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// §12.3: an absent list serialises as [], never null.
func TestMetaNeverWritesANullRoleList(t *testing.T) {
	l := unlockedPR(t)

	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&Meta{Owner: "acme", Repo: "web", PR: 42}))
	require.NoError(t, held.Unlock())

	body, err := l.ReadPR("acme", "web", 42, FileMeta)
	require.NoError(t, err)
	assert.Contains(t, string(body), "\"active_roles\": []")
	assert.NotContains(t, string(body), "null")
}

// A mapping stamp counts only for the round and head meta.json records: a stamp
// a closed round left behind names a round this one is not, and the zero stamp
// of a round nobody mapped names none.
func TestAMappingStampCountsOnlyForTheRoundAndHeadItNames(t *testing.T) {
	for name, c := range map[string]struct {
		meta Meta
		want bool
	}{
		"stamped for this round and head": {Meta{Round: 2, Head: "b", MappingRound: 2, MappingHead: "b"}, true},
		"never stamped":                   {Meta{Round: 2, Head: "b"}, false},
		"stamped by the closed round":     {Meta{Round: 2, Head: "b", MappingRound: 1, MappingHead: "a"}, false},
		"same round, another head":        {Meta{Round: 2, Head: "b", MappingRound: 2, MappingHead: "a"}, false},
		"no round opened":                 {Meta{}, false},
	} {
		assert.Equal(t, c.want, c.meta.MappingRecorded(), name)
	}
}

// A claims stamp counts only for the round and head meta.json records, as the
// mapping stamp does.
func TestAClaimsStampCountsOnlyForTheRoundAndHeadItNames(t *testing.T) {
	for name, c := range map[string]struct {
		meta Meta
		want bool
	}{
		"stamped for this round and head": {Meta{Round: 2, Head: "b", ClaimsRound: 2, ClaimsHead: "b"}, true},
		"never stamped":                   {Meta{Round: 2, Head: "b"}, false},
		"stamped by the closed round":     {Meta{Round: 2, Head: "b", ClaimsRound: 1, ClaimsHead: "a"}, false},
		"same round, another head":        {Meta{Round: 2, Head: "b", ClaimsRound: 2, ClaimsHead: "a"}, false},
		"no round opened":                 {Meta{}, false},
	} {
		assert.Equal(t, c.want, c.meta.ClaimsRecorded(), name)
	}
}
