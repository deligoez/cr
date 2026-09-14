package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// staledRound is aRoundTheHeadOutran holding three open records and a posted
// one, and the issue file a brief of it reads.
func staledRound(t *testing.T) (layout state.Layout, issue string) {
	t.Helper()
	layout, recorded, _ := aRoundTheHeadOutran(t)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	var stored strings.Builder
	for _, record := range []struct{ id, state string }{
		{"f1", "draft"}, {"f2", "queued"}, {"f3", "posted"}, {"f4", "draft"},
	} {
		stored.WriteString(`{"id":"` + record.id + `","state":"` + record.state +
			`","head":"` + recorded + `","round":1}` + "\n")
	}
	require.NoError(t, held.Write(state.FileFindings, []byte(stored.String())))
	require.NoError(t, held.Unlock())
	issue = filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte("Retry on 5xx.\n"), 0o600))
	return layout, issue
}

// The brief that opens a new round on a moved head reports the records §9.3.4
// moved to stale, by id, and the ids are the records transitions.ndjson
// journals the move for; a same-head brief after it reports none.
//
// Measured on release QA before the fix (D-S11-3): the brief that opened round
// 2 and moved f701, f1 and f1701 to stale printed round 2 and nothing about the
// move, honesty [] included; only transitions.ndjson held it.
func TestABriefOpeningARoundReportsTheRecordsItStaled(t *testing.T) {
	layout, issue := staledRound(t)
	brief := func() []string {
		t.Helper()
		printed, err := runCLIPrinting(t, "brief", fixturePR, "--issue", fixtureIssue,
			"--intent-file", issue, "--repo", fixtureSlug)
		require.NoError(t, err)
		var report struct {
			Round  int      `json:"round"`
			Staled []string `json:"staled_records"`
		}
		require.NoError(t, json.Unmarshal([]byte(printed), &report))
		require.Equal(t, 2, report.Round)
		return report.Staled
	}

	assert.Equal(t, []string{"f1", "f2", "f4"}, brief())

	journal, err := state.ReadRecords[finding.Transition](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileTransitions)
	require.NoError(t, err)
	journaled := make([]string, 0, len(journal))
	for i := range journal {
		journaled = append(journaled, journal[i].Record)
	}
	assert.Equal(t, []string{"f1", "f2", "f4"}, journaled, "the report and the journal name the same moves")

	assert.Equal(t, []string{}, brief(), "a same-head brief opens no round and moves nothing")
}

// The terminal says the same thing, as one line beside the round's head.
func TestATerminalBriefOpeningARoundNamesTheRecordsItStaled(t *testing.T) {
	_, issue := staledRound(t)

	shown := throughATerminal(t, "brief", fixturePR, "--issue", fixtureIssue,
		"--intent-file", issue, "--repo", fixtureSlug, "--no-color")

	assert.Contains(t, strings.Split(strings.ReplaceAll(shown, "\r", ""), "\n"),
		"  staled     3 open record(s) moved to stale, per §9.3.4: f1, f2, f4")
}
