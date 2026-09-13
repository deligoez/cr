package cli

import (
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// journaled names the pull request and head a fixture's journal is read for.
type journaled struct {
	repo string
	pr   int
	head string
}

// The two fixtures' pull requests, both under the owner the fixtures share.
var (
	recordJournal = journaled{repo: recordRepo, pr: recordPRNum, head: recordHead}
	draftJournal  = journaled{repo: draftRepo, pr: draftPRNum, head: draftHead}
)

// journalOf is one pull request's transitions.ndjson as "<record>:<from>><to> by
// <actor>", in the order the file holds the lines, after checking that every
// line carries §9.1.1's timestamp, head SHA and actor.
func journalOf(t *testing.T, l state.Layout, of journaled) []string {
	t.Helper()
	require.Equal(t, recordOwner, draftOwner, "both fixtures sit under one owner")
	lines, err := state.ReadRecords[finding.Transition](l, recordOwner, of.repo, of.pr, state.FileTransitions)
	require.NoError(t, err)
	named := make([]string, 0, len(lines))
	for _, line := range lines {
		assert.False(t, line.At.IsZero(), "§9.1.1: every line carries a timestamp")
		assert.Equal(t, of.head, line.Head, "§9.1.1: every line carries the head SHA")
		assert.NotEmpty(t, line.Actor, "§9.1.1: every line carries the actor")
		from := "new"
		if line.From != nil {
			from = line.From.String()
		}
		named = append(named, line.Record+":"+from+">"+line.To.String()+" by "+line.Actor)
	}
	return named
}

// §9.1.1 through `cr record` and `cr draft`: the table's first three rows and
// `cr draft`'s discard each leave exactly one line, and a draft run that moves
// nothing leaves none.
//
// f2 retires as a duplicate and f3 as thread-suppressed, so both of the second
// row's states are reached; f1 is queued by the first draft and discarded by
// the second, once its block was deleted.
func TestRecordAndDraftLeaveOneJournalLinePerTransition(t *testing.T) {
	layout := recordedHome(t)
	ingestThreads(t, layout, ingestedThread)
	duplicate := aRecord("f2", "u2")
	duplicate["duplicate_of"] = "f1"
	covered := aRecord("f3", "u2")
	covered["suppressed_by"] = ingestedThread
	_, err := runRecord(t, recordPR, asMergeOutput(t, layout, writeRecordFile(t, "merged.ndjson",
		aRecord("f1", "u1"), duplicate, covered)), "--repo", recordSlug)
	require.NoError(t, err)
	recorded := []string{
		"f1:new>draft by cr record",
		"f2:new>draft by cr record", "f2:draft>duplicate by cr record",
		"f3:new>draft by cr record", "f3:draft>suppressed by cr record",
	}
	assert.Equal(t, recorded, journalOf(t, layout, recordJournal))

	_, err = runDraft(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)
	queued := append(slices.Clone(recorded), "f1:draft>queued by cr draft")
	assert.Equal(t, queued, journalOf(t, layout, recordJournal))

	_, err = runDraft(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)
	assert.Equal(t, queued, journalOf(t, layout, recordJournal),
		"§7.1.6's regeneration moves nothing, so it leaves no line")

	drafted := layout.RoundFile(recordOwner, recordRepo, recordPRNum, recordRound, state.FileDraft)
	body, err := os.ReadFile(drafted)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(drafted, []byte(deleteBlock(t, string(body), "f1")), 0o600))
	_, err = runDraft(t, recordPR, "--repo", recordSlug)
	require.NoError(t, err)
	assert.Equal(t, append(queued, "f1:queued>discarded by cr draft"),
		journalOf(t, layout, recordJournal))
}

// §9.1.1 through `cr post --confirm`: a queued record sent leaves a posted line
// and one whose block the reviewer deleted leaves a discarded line, both under
// the actor that sent — §9.1's table names `cr post --confirm` on both rows.
func TestAConfirmedPostLeavesOneJournalLinePerTransition(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	writeDraft(t, layout, deleteBlock(t, readDraft(t, layout), "f2"))
	ghShimming(t, builtPayload(t))

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	assert.Equal(t, []string{
		"f1:draft>queued by cr draft", "f2:draft>queued by cr draft",
		"f2:queued>discarded by cr post --confirm", "f1:queued>posted by cr post --confirm",
	}, journalOf(t, layout, draftJournal))
}

// §9.1.1 through `cr post --reconcile` on adopt: each record the adopted review
// carried leaves one posted line under the reconcile actor.
func TestAReconcileAdoptLeavesOneJournalLinePerTransition(t *testing.T) {
	layout, hash := anUnresolvedPosting(t)
	reviewsAnswering(t, gh.Review{ID: "PRR_ours", URL: "https://example.invalid/r", Body: reviewBodyCarrying(hash)})

	_, err := runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
	require.NoError(t, err)

	assert.Equal(t, []string{
		"f1:queued>posted by cr post --reconcile", "f2:queued>posted by cr post --reconcile",
	}, journalOf(t, layout, draftJournal))
}
