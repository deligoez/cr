package brief

import (
	"os"
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

// §9.1.1 when the journal append fails on the increment: the sweep's moves are
// not published without their lines, so the brief that follows journals every
// record it stales exactly once.
//
// transitions.ndjson is made a directory for one run. Its append reads the file
// before publishing and that read fails, while findings.ndjson is published by a
// rename in a directory that stays writable, so the fixture fails the journal
// alone. Measured before stale-sweep-journal-before-publish, which swept and
// published findings.ndjson first: the failed brief left f1 and f2 already
// stale, the re-run found nothing open to move, and the journal held no line.
func TestAFailedJournalAppendLeavesNoStaleMoveUnjournaled(t *testing.T) {
	dir, first, base := repository(t)
	src := sources(t, dir, answering(first, base, oneThread))
	_, err := Run(src)
	require.NoError(t, err)
	seed(t, src, first)

	second := advanceAdding(t, dir)
	src.GH = gh.WithRunner(answering(second, base, oneThread))
	journalPath := src.Layout.PRFile(testOwner, testRepo, testPR, state.FileTransitions)
	require.NoError(t, os.Remove(journalPath))
	require.NoError(t, os.Mkdir(journalPath, 0o700))
	_, err = Run(src)
	require.Error(t, err, "the journal cannot be appended to, so the brief fails")

	states := func() map[string]finding.State {
		t.Helper()
		stored, err := state.ReadRecords[*finding.Finding](
			src.Layout, testOwner, testRepo, testPR, state.FileFindings)
		require.NoError(t, err)
		out := map[string]finding.State{}
		for _, record := range stored {
			out[record.ID] = record.State
		}
		return out
	}
	assert.Equal(t, map[string]finding.State{
		"f1": finding.StateDraft, "f2": finding.StateQueued, "f3": finding.StatePosted,
	}, states(), "a move whose journal line was not written is not published")

	require.NoError(t, os.Remove(journalPath))
	_, err = Run(src)
	require.NoError(t, err)

	assert.Equal(t, map[string]finding.State{
		"f1": finding.StateStale, "f2": finding.StateStale, "f3": finding.StatePosted,
	}, states())
	journal, err := state.ReadRecords[finding.Transition](
		src.Layout, testOwner, testRepo, testPR, state.FileTransitions)
	require.NoError(t, err)
	require.Len(t, journal, 2, "one line per record moved to stale, across the failure and the re-run")
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
		assert.Equal(t, second, line.Head)
	}
}
