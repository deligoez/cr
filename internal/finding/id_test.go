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

// An id cr did not write says nothing about what is taken, so it contributes
// nothing rather than being read as some number near it. f0 is the boundary:
// ids are numbered from one, so counting it would hand the next record f1 while
// something already claimed to be a record id below it.
func TestAnIDCrDidNotWriteIsNotCounted(t *testing.T) {
	for _, id := range []string{"", "9", "u9", "F9", "f", "f0", "f-9", "f+9", "f09", "f9x", "fnine"} {
		assert.Equal(t, "f1", NextID([]Finding{{ID: id}}), "%q", id)
	}
	assert.Equal(t, "f10", NextID([]Finding{{ID: "f9"}, {ID: "u99"}}))
}

// The same reading, asked of an id that came from a person rather than from
// findings.ndjson. §3.6.2's `cr answer` names a record inside a note without
// resolving one, so the spelling is the whole of what it can check, and it
// reads §6.1's spelling here rather than restating it.
//
// The two directions matter equally. Accepting f09 or f-9 would let a note
// reference an id cr could never have allocated, and rejecting f10 would refuse
// an answer to the tenth record of a pull request.
func TestOnlyTheSpellingSixOneGivesARecordIsValid(t *testing.T) {
	for _, id := range []string{"f1", "f2", "f9", "f10", "f4096"} {
		assert.True(t, ValidID(id), "%q", id)
	}
	for _, id := range []string{
		"", "9", "u9", "F9", "f", "f0", "f-9", "f+9", "f09", "f9x", "fnine", " f9", "f9 ", "CR-1#n1",
	} {
		assert.False(t, ValidID(id), "%q", id)
	}
}
