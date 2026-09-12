package brief

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// §9.1.1 through `cr brief` on a new head: §9.1's last row moves f1 from
// `draft` and f2 from `queued` to `stale`, and each move leaves exactly one
// journal line naming `cr brief` and the head the brief moved to. The posted
// f3 is not moved and leaves none, and a same-head brief after it adds nothing.
func TestAnIncrementLeavesOneJournalLinePerRecordItStales(t *testing.T) {
	dir, first, base := repository(t)
	src := sources(t, dir, answering(first, base, oneThread))
	_, err := Run(src)
	require.NoError(t, err)
	seed(t, src, first)

	second := advanceAdding(t, dir)
	src.GH = gh.WithRunner(answering(second, base, oneThread))
	_, err = Run(src)
	require.NoError(t, err)
	_, err = Run(src)
	require.NoError(t, err)

	journal, err := state.ReadRecords[finding.Transition](
		src.Layout, testOwner, testRepo, testPR, state.FileTransitions)
	require.NoError(t, err)
	require.Len(t, journal, 2, "one line per record §9.3.4 staled, and none for the same-head re-run")
	for i, want := range []struct {
		record string
		from   finding.State
	}{{"f1", finding.StateDraft}, {"f2", finding.StateQueued}} {
		line := journal[i]
		assert.Equal(t, want.record, line.Record)
		require.NotNil(t, line.From)
		assert.Equal(t, want.from, *line.From)
		assert.Equal(t, finding.StateStale, line.To)
		assert.Equal(t, "cr brief", line.Actor)
		assert.Equal(t, second, line.Head, "the head the brief moved the round to")
		assert.False(t, line.At.IsZero())
	}
}
