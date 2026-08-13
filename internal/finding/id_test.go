package finding

import (
	"testing"

	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
)

// §6.1 makes a record id stable for the life of the pull request, which is the
// opposite of §3.4.6's unit ids: those are round-scoped and are reassigned every
// round. findings.ndjson keeps every round's records, so allocation reads all of
// them — an id spent by a record that has since gone stale, been discarded, or
// been posted stays spent, or one id would name two records in one file and two
// comments in the pull request's history.
func TestARecordIDIsNeverReusedAcrossRounds(t *testing.T) {
	assert.Equal(t, "f1", NextID(nil))
	assert.Equal(t, "f1", NextID([]Finding{}))
	assert.Equal(t, "f2", NextID([]Finding{{ID: "f1"}}), "the first id is spent like any other")
	assert.Equal(t, "f3", NextID([]Finding{{ID: "f1"}, {ID: "f2"}}))

	acrossRounds := []Finding{
		{ID: "f1", Stamp: state.Stamp{Round: 1}},
		{ID: "f7", Stamp: state.Stamp{Round: 1}},
		{ID: "f3", Stamp: state.Stamp{Round: 2}},
	}
	assert.Equal(t, "f8", NextID(acrossRounds), "round 1 spent f7, so round 2 starts after it")
}

