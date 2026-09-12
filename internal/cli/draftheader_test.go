package cli

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// §7.1.4 through the command: the draft opens with a header whose comment count
// is the one §1.6.2's cap check uses, measured against post.max_comments as
// §2.7 resolves it for this repository — and whose coverage state is the round's.
//
// The cap is set below the queue in the repository layer, so the header has an
// excess to name and the cap check has a block to make, and the number the
// header prints is compared with the number finding.CommentCapFor gives the
// same queued records under the same resolution. The round is two units under
// two active roles with three of four cells filled, so the coverage line has a
// gap to report rather than a zero that any fixture would produce.
func TestTheDraftHeaderReportsTheCountTheCapCheckUses(t *testing.T) {
	layout := draftedHome(t,
		aStoredRecord("f1", finding.StateDraft),
		aStoredRecord("f2", finding.StateDraft),
		aStoredRecord("f3", finding.StateDraft),
		aStoredRecord("f4", finding.StateDuplicate),
	)
	require.NoError(t, layout.EnsureRepo(draftOwner, draftRepo))
	require.NoError(t, os.WriteFile(layout.RepoConfig(draftOwner, draftRepo),
		[]byte(`{"post": {"max_comments": 2}}`), 0o600))
	coveredRound(t, layout)

	_, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err, "§1.6.2 blocks posting, not drafting: the draft is where the user triages")

	rendered := readDraft(t, layout)
	require.True(t, strings.HasPrefix(rendered, "<!-- cr:summary\n"), "§7.1.4: the file opens with the header")
	header, _, closed := strings.Cut(rendered, "-->")
	require.True(t, closed, "the header closes")

	resolved, err := config.Resolve(config.Sources{
		GlobalConfig: layout.Config(),
		RepoConfig:   layout.RepoConfig(draftOwner, draftRepo),
	})
	require.NoError(t, err)
	stored, err := state.ReadRecords[*finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err)
	queued, err := queueRecords(stored, finding.NewJournal(finding.ActorDraft, draftHead, time.Now()))
	require.NoError(t, err)
	capped := finding.CommentCapFor(queued, resolved.Int(settingMaxComments))

	assert.Contains(t, header, "\ncomments: "+capped.Disclosure()+"\n",
		"the header's count is the cap check's count")
	assert.Contains(t, header, "3 comments queued against post.max_comments 2, 1 over the cap")
	var exceeded *finding.CommentCapExceededError
	assert.ErrorAs(t, capped.Err(), &exceeded, "and it is the count the cap check blocks on")

	assert.Contains(t, header,
		"\ncoverage: 2 unit(s) against 2 active role(s): 1 with a complete row of cells, 1 with gaps, 0 oversized\n")
	assert.Contains(t, header, "\nrecords: 3 queued\n", "the duplicate is counted by nothing here")
}

// coveredRound gives the drafted round two active roles, two units, and three
// of the four cells between them, written as `cr brief` and `cr cells record`
// leave them.
func coveredRound(t *testing.T, layout state.Layout) {
	t.Helper()
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum,
		Round: draftRound, Head: draftHead,
		ActiveRoles: []string{"convention", "correctness"},
	}))
	stamp := `"head":"` + draftHead + `","round":2}`
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"internal/api/handler.go","hash":"38372bc96eb4010e",`+stamp+"\n"+
			`{"id":"u2","path":"internal/api/money.go","hash":"0a1b2c3d4e5f6071",`+stamp+"\n")))
	require.NoError(t, held.Write(state.FileCoverage, []byte(
		`{"unit":"u1","role":"convention","result":"pass","unit_hash":"38372bc96eb4010e",`+stamp+"\n"+
			`{"unit":"u1","role":"correctness","result":"finding","unit_hash":"38372bc96eb4010e",`+stamp+"\n"+
			`{"unit":"u2","role":"correctness","result":"pass","unit_hash":"0a1b2c3d4e5f6071",`+stamp+"\n")))
	require.NoError(t, held.Unlock())
}
