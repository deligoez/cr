package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// nextOfFixture runs `cr next` over the fixture pull request and decodes it.
func nextOfFixture(t *testing.T) nextResult {
	t.Helper()
	printed, err := runCLIPrinting(t, "next", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report nextResult
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report
}

// stepNames are the report's steps, by name, in order.
func stepNames(report *nextResult) []string {
	names := make([]string, 0, len(report.Steps))
	for i := range report.Steps {
		names = append(names, report.Steps[i].Step)
	}
	return names
}

// roleWrote writes one role's file into the fixture round's fan-out.
func roleWrote(t *testing.T, unit, name, body string) string {
	t.Helper()
	layout, err := state.Default()
	require.NoError(t, err)
	dir := layout.FanOutDir(fixtureOwner, fixtureProject, fixturePRNumber, 1, unit)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// §10.4 over statusHome's round: a role's file nobody recorded, a unit no role
// reported on, and a claim mapped to nothing are three steps, in §10.4's order,
// each naming what it is owed for.
func TestNextNamesEveryStepTheRoundOwesInOrder(t *testing.T) {
	statusHome(t)
	written := roleWrote(t, "u2", finding.FanOutFile("correctness"), `{"id":"f1","kind":"question"}`+"\n")

	report := nextOfFixture(t)

	assert.Equal(t, 1, report.Round)
	assert.Equal(t, []string{"record", "review", "settle"}, stepNames(&report))
	require.NotNil(t, report.Next)
	assert.Equal(t, "record", report.Next.Step, "the first owed step is the next one")

	record := report.Steps[0]
	assert.Equal(t, actorCr, record.Actor)
	assert.Equal(t, []string{written}, record.Items)
	require.Len(t, record.Commands, 2)
	assert.Contains(t, record.Commands[0], "cr merge "+written+" -o ")
	assert.True(t, strings.HasPrefix(record.Commands[1], "cr record "+fixturePR+" --repo "+fixtureSlug+" "))

	review := report.Steps[1]
	assert.Equal(t, actorAgent, review.Actor)
	assert.Equal(t, []string{"u2/convention", "u2/correctness", "u2/intent-coverage"}, review.Items)

	settle := report.Steps[2]
	assert.Equal(t, []string{fixtureIssue + "#c3"}, settle.Items,
		"#c2 is set aside on a standing note, so only #c3 blocks §10.2.3")
}

// §10.4.4's exception: a file whose records a later merge or record already
// read — and dropped, as waived or already posted — is not owed again.
func TestAFileTheLastRecordReadIsNotOwedAgain(t *testing.T) {
	statusHome(t)
	written := roleWrote(t, "u2", finding.FanOutFile("correctness"), `{"id":"f1","kind":"question"}`+"\n")
	layout, err := state.Default()
	require.NoError(t, err)
	intake := layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileIntake)
	require.NoError(t, os.MkdirAll(filepath.Dir(intake), 0o700))
	require.NoError(t, os.WriteFile(intake, []byte("{}\n"), 0o600))
	earlier := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(written, earlier, earlier))

	report := nextOfFixture(t)

	assert.Equal(t, []string{"review", "settle"}, stepNames(&report))
}

// §10.4.7: records in draft or queued are the human's step, and the report's
// commands stop at the dry run — the one command that sends is never printed.
func TestTheDraftStepNeverCarriesTheConfirmation(t *testing.T) {
	statusHome(t)
	layout, err := state.Default()
	require.NoError(t, err)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(
		`{"id":"f1","state":"queued","head":"`+meta.Head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())

	report := nextOfFixture(t)

	require.Contains(t, stepNames(&report), "draft")
	draft := report.Steps[len(report.Steps)-1]
	assert.Equal(t, actorHuman, draft.Actor)
	assert.Equal(t, []string{"f1"}, draft.Items)
	for i := range report.Steps {
		for _, command := range report.Steps[i].Commands {
			assert.NotContains(t, command, "--confirm", "§10.4.7")
		}
	}
}

// §10.4.1: a pull request no brief has opened owes the brief, and nothing else
// can be read about it.
func TestAnUnbriefedPullRequestOwesTheBrief(t *testing.T) {
	require.NoError(t, state.New(crHome(t)).Init())

	report := nextOfFixture(t)

	assert.Equal(t, []string{"brief"}, stepNames(&report))
	assert.Equal(t, []string{"cr brief " + fixturePR + " --repo " + fixtureSlug}, report.Steps[0].Commands)
}

// §10.4.1: a round whose head moved owes the brief alone, carrying the issue
// key it was briefed with, because §9.3.2 refuses every write the other steps
// would make.
func TestAMovedHeadOwesOnlyTheBrief(t *testing.T) {
	layout, _, _ := aRoundTheHeadOutran(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)

	report := nextOfFixture(t)

	assert.Equal(t, []string{"brief"}, stepNames(&report))
	want := "cr brief " + fixturePR + " --repo " + fixtureSlug
	if meta.IssueKey != "" {
		want += " --issue " + meta.IssueKey
	}
	assert.Equal(t, []string{want}, report.Steps[0].Commands)
}
