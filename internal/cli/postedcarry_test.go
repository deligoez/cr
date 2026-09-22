package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// findingLines reads findings.ndjson back as one decoded object per record id.
func findingLines(t *testing.T, l state.Layout) map[string]map[string]any {
	t.Helper()
	body, err := os.ReadFile(l.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings))
	require.NoError(t, err)
	lines := make(map[string]map[string]any)
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		var held map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &held))
		id, _ := held["id"].(string)
		require.NotContains(t, lines, id, "§6.1 gives a record one id, so findings.ndjson holds it on one line")
		lines[id] = held
	}
	return lines
}

// §9.1: a `posted` record no one settles is carried into the next round, and
// §9.5 and §9.6 are how it is settled — so after the push that moves the head,
// each of them still has to reach it.
//
// v0.5.0 read only the current round's records, and `cr brief` copies no posted
// record into the round a moved head opens, so this is what that release did
// after a real push: `cr recheck` reported no concern, and `cr verify`,
// `cr resolve` and `cr withdraw` answered "not a record of round 2". Every test
// before this one seeded the posted record into the round the command read, so
// none of them could see it.
//
// The head is moved by a real second commit and the round is opened by a real
// `cr brief`, so what is measured is the state the release actually leaves, not
// a hand-written picture of it.
func TestAPostedConcernOutlivesThePushThatMovesTheHead(t *testing.T) {
	layout, recorded, moved := aRoundTheHeadOutran(t)
	holdRecords(t, layout, fixtureOwner, fixtureProject, fixturePRNumber,
		`{"id":"f3","kind":"question","summary":"why is the retry unbounded?","anchor":`+stampedAnchor+`,`+
			`"state":"posted","thread_id":"PRRT_q","head":"`+recorded+`","round":1}`,
		`{"id":"f4","kind":"finding","summary":"the retry never stops","anchor":`+stampedAnchor+`,`+
			`"state":"posted","thread_id":"PRRT_f","head":"`+recorded+`","round":1}`)
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte("Retry on 5xx.\n"), 0o600))

	_, err := runCLIPrinting(t, "brief", fixturePR, "--issue", fixtureIssue, "--intent-file", issue,
		"--repo", fixtureSlug)
	require.NoError(t, err)
	opened, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.Equal(t, 2, opened.Round, "the push opened a round, which is the case being measured")
	require.Equal(t, moved, opened.Head)

	printed, err := runCLIPrinting(t, "recheck", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report struct {
		Concerns []struct {
			ID string `json:"id"`
		} `json:"concerns"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	reported := make([]string, 0, len(report.Concerns))
	for _, concern := range report.Concerns {
		reported = append(reported, concern.ID)
	}
	assert.ElementsMatch(t, []string{"f3", "f4"}, reported,
		"§9.5.2 reports every posted record, and the push did not settle either")

	_, err = runCLIPrinting(t, "withdraw", fixturePR, "f4", "not-here", "--repo", fixtureSlug)
	require.NoError(t, err, "§9.6.2 reaches the posted record from the round the push opened")
	_, err = runCLIPrinting(t, "resolve", fixturePR, "f3", "--repo", fixtureSlug)
	require.Error(t, err, "f3 is not settled yet, so §9.6.1 refuses — on its state, not on its round")
	assert.Contains(t, err.Error(), "f3")
	assert.NotContains(t, err.Error(), "round")

	_, err = runCLIPrinting(t, "verify", fixturePR, "f3", "answered",
		"--evidence", "the author's reply names the bound", "--repo", fixtureSlug)
	require.NoError(t, err, "§9.5.5 reaches the posted record from the round the push opened")

	stored := findingLines(t, layout)
	require.Len(t, stored, 2, "the verdict moved the record where it is and wrote no second copy")
	assert.Equal(t, "answered", stored["f3"]["state"])
	assert.EqualValues(t, 1, stored["f3"]["round"],
		"§2.3.3's pair says which run wrote the record, and a verdict changes where it stands")
	assert.Equal(t, recorded, stored["f3"]["head"])
	assert.Equal(t, "posted", stored["f4"]["state"], "a dry-run withdrawal writes nothing")

	_, err = runCLIPrinting(t, "resolve", fixturePR, "f3", "--repo", fixtureSlug)
	require.NoError(t, err, "settled now, so §9.6.1's dry run prints what it would send")
}
