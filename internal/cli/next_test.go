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

// roleWrote writes one role's file into the fan-out of the fixture round's
// unit u2, the unit statusHome leaves without cells.
func roleWrote(t *testing.T, name, body string) string {
	t.Helper()
	layout, err := state.Default()
	require.NoError(t, err)
	dir := layout.FanOutDir(fixtureOwner, fixtureProject, fixturePRNumber, 1, "u2")
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
	written := roleWrote(t, finding.FanOutFile("correctness"), `{"id":"f1","kind":"question"}`+"\n")

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

// §10.4.5's exception: a file whose records a later merge or record already
// read — and dropped, as waived or already posted — is not owed again.
func TestAFileTheLastRecordReadIsNotOwedAgain(t *testing.T) {
	statusHome(t)
	written := roleWrote(t, finding.FanOutFile("correctness"), `{"id":"f1","kind":"question"}`+"\n")
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

// §10.4.8: records in draft or queued are the human's step, and the report's
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
			assert.NotContains(t, command, "--confirm", "§10.4.8")
		}
	}
}

// The draft step asks for the English bodies to be rewritten in render.lang
// before the human reads them, and asks nothing when that language is English.
func TestTheDraftStepAsksForTheBodiesInTheRenderLanguage(t *testing.T) {
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

	assert.NotContains(t, stepNamed(t, &report, "draft").Why, "rewrite", "§8.1.1: en is the default")

	config := layout.RepoConfig(fixtureOwner, fixtureProject)
	require.NoError(t, os.MkdirAll(filepath.Dir(config), 0o700))
	require.NoError(t, os.WriteFile(config, []byte(`{"render": {"lang": "tr"}}`), 0o600))

	report = nextOfFixture(t)

	assert.Contains(t, stepNamed(t, &report, "draft").Why, "rewrite each block's English body in render.lang `tr`")
}

// §10.4.2: a pull request no brief has opened owes the brief, and nothing else
// can be read about it.
func TestAnUnbriefedPullRequestOwesTheBrief(t *testing.T) {
	require.NoError(t, state.New(crHome(t)).Init())

	report := nextOfFixture(t)

	assert.Equal(t, []string{"brief"}, stepNames(&report))
	assert.Equal(t, []string{"cr brief " + fixturePR + " --repo " + fixtureSlug}, report.Steps[0].Commands)
}

// §10.4.2: a round whose head moved owes the brief alone, carrying the issue
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

// §10.4.5 looks at every file of a unit: an empty file, a fully recorded one,
// or a proposals file sorting first says nothing about the file beside it.
// Measured before the fix: any of the three hid the unrecorded file after it.
func TestEveryFileOfAUnitIsLookedAt(t *testing.T) {
	for name, first := range map[string]struct{ file, body string }{
		"an empty file":           {finding.FanOutFile("convention"), ""},
		"an empty proposals file": {"proposals-correctness.ndjson", ""},
	} {
		t.Run(name, func(t *testing.T) {
			statusHome(t)
			roleWrote(t, first.file, first.body)
			written := roleWrote(t, finding.FanOutFile("correctness"), `{"id":"f1","kind":"question"}`+"\n")

			report := nextOfFixture(t)

			require.Equal(t, "record", report.Steps[0].Step)
			assert.Equal(t, []string{written}, report.Steps[0].Items)
		})
	}
}

// A file only opening with `proposals-` is not a §5.7 file, and a directory
// named like a role's file is nobody's output.
func TestOnlyARolesFilesAreOwed(t *testing.T) {
	statusHome(t)
	roleWrote(t, "proposals-notes.txt", `{"id":"x1"}`+"\n")
	roleWrote(t, "proposals-x.ndjson.bak", `{"id":"x1"}`+"\n")
	layout, err := state.Default()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(
		layout.FanOutDir(fixtureOwner, fixtureProject, fixturePRNumber, 1, "u2"), finding.FanOutFile("x")), 0o700))

	report := nextOfFixture(t)

	assert.Equal(t, []string{"review", "settle"}, stepNames(&report))
}

// A file `cr next` cannot read is §11.2's file failure, exit 3, naming the
// file — not a malformed invocation.
func TestAnUnreadableRoleFileIsAFileFailure(t *testing.T) {
	statusHome(t)
	written := roleWrote(t, finding.FanOutFile("correctness"), `{"id":"f1"}`+"\n")
	require.NoError(t, os.Chmod(written, 0o000))
	t.Cleanup(func() { _ = os.Chmod(written, 0o600) })

	_, err := runCLIPrinting(t, "next", fixturePR, "--repo", fixtureSlug)

	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Contains(t, err.Error(), written)
}

// §10.4 prints commands to be pasted, so a path holding a space or a quote is
// one shell word in them.
func TestAPrintedPathIsOneShellWord(t *testing.T) {
	assert.Equal(t, "/tmp/plain.ndjson", shellWord("/tmp/plain.ndjson"))
	assert.Equal(t, "'/tmp/home dir/it'\\''s.ndjson'", shellWord("/tmp/home dir/it's.ndjson"))
	assert.Equal(t, "''", shellWord(""))

	statusHome(t)
	written := roleWrote(t, finding.FanOutFile("correctness"), `{"id":"f1"}`+"\n")
	spaced := filepath.Join(filepath.Dir(written), "..", "u 3")
	require.NoError(t, os.MkdirAll(spaced, 0o700))
	moved := filepath.Join(spaced, finding.FanOutFile("correctness"))
	require.NoError(t, os.Rename(written, moved))

	report := nextOfFixture(t)

	require.Equal(t, "record", report.Steps[0].Step)
	assert.Contains(t, report.Steps[0].Commands[0], "'"+filepath.Clean(moved)+"'")
}

// A role that appended new records to a file the round already recorded
// leaves a file `cr merge` refuses whole, naming an id already held; the
// record step says so rather than printing a command that fails (QA,
// 2026-09-23).
func TestAFileMixingHeldAndNewIDsIsNamed(t *testing.T) {
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
	written := roleWrote(t, finding.FanOutFile("correctness"),
		`{"id":"f1","kind":"question"}`+"\n"+`{"id":"f2","kind":"question"}`+"\n")

	report := nextOfFixture(t)

	require.Equal(t, "record", report.Steps[0].Step)
	assert.Contains(t, report.Steps[0].Why, "each of "+written+" also holds ids the round already holds")
}

// §2.4.6: `cr next` loads the round's profile, so a profile file an earlier
// release shipped is reported as every other command reports it.
func TestNextReportsAProfileAnEarlierReleaseShipped(t *testing.T) {
	statusHome(t)
	layout, err := state.Default()
	require.NoError(t, err)
	old, err := os.ReadFile(filepath.Join("..", "profile", "builtin", "shipped", "v0.7.1", "go.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(layout.Profile("go"), old, 0o600))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.ProfileID = "go"
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())

	report := nextOfFixture(t)

	assert.Contains(t, strings.Join(report.Honesty, "\n"), "is the go profile cr v0.7.1 shipped")
}

// §10.4.1: a send whose outcome cr never learned owes its reconciliation
// alone, because every other step refuses until it has run (QA, 2026-09-23:
// `cr next` offered `settle` and `draft`, and `cr draft` exited 4).
func TestAnUnresolvedSendOwesOnlyItsReconciliation(t *testing.T) {
	statusHome(t)
	layout, err := state.Default()
	require.NoError(t, err)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.PostUnresolved = true
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())

	report := nextOfFixture(t)

	assert.Equal(t, []string{"reconcile"}, stepNames(&report))
	assert.Equal(t, []string{"cr post " + fixturePR + " --repo " + fixtureSlug + " --reconcile"},
		report.Steps[0].Commands)
}

// §10.4.5 over proposals alone: every role's unrecorded proposals file is one
// `cr proposals record` in the record step, with no merge of review files
// nobody wrote and nothing said about ids the round already holds.
func TestUnrecordedProposalsAloneOweTheirRecording(t *testing.T) {
	statusHome(t)
	written := make([]string, 0, 3)
	for _, role := range []string{"convention", "correctness", "intent-coverage"} {
		written = append(written, roleWrote(t, "proposals-"+role+".ndjson", `{"id":"p1"}`+"\n"))
	}

	report := nextOfFixture(t)

	require.Equal(t, "record", report.Steps[0].Step)
	record := report.Steps[0]
	assert.Equal(t, written, record.Items)
	want := make([]string, 0, len(written))
	for _, file := range written {
		want = append(want, "cr proposals record "+fixturePR+" --repo "+fixtureSlug+" "+file)
	}
	assert.Equal(t, want, record.Commands)
	assert.Equal(t, "the roles wrote records or proposals this round does not hold yet", record.Why)
}

// §10.4.6: a missing intent-coverage cell is the intent pass's to fill, so the
// review step opens with that pass's own emission, and without one it does not.
func TestAMissingIntentCellSendsTheReviewStepThroughTheIntentPass(t *testing.T) {
	statusHome(t)
	intentFirst := "cr review " + fixturePR + " --repo " + fixtureSlug + " --axis intent"

	report := nextOfFixture(t)

	review := stepNamed(t, &report, "review")
	require.Contains(t, review.Items, "u2/intent-coverage")
	assert.Equal(t, []string{
		intentFirst,
		"cr review " + fixturePR + " --repo " + fixtureSlug,
		"cr cells record " + fixturePR + " --repo " + fixtureSlug + " <cells.ndjson>",
	}, review.Commands)

	layout, err := state.Default()
	require.NoError(t, err)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	var cells strings.Builder
	for _, seat := range []struct{ unit, hash, role string }{
		{"u1", "h1", "convention"}, {"u1", "h1", "correctness"}, {"u1", "h1", "intent-coverage"},
		{"u2", "h2", "intent-coverage"},
	} {
		cells.WriteString(`{"unit":"` + seat.unit + `","role":"` + seat.role + `","result":"pass","unit_hash":"` +
			seat.hash + `","head":"` + meta.Head + `","round":1}` + "\n")
	}
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileCoverage, []byte(cells.String())))
	require.NoError(t, held.Unlock())

	report = nextOfFixture(t)

	review = stepNamed(t, &report, "review")
	assert.Equal(t, []string{"u2/convention", "u2/correctness"}, review.Items)
	assert.NotContains(t, review.Commands, intentFirst)
}

// stepNamed is the report's step called name, failing the test without one.
func stepNamed(t *testing.T, report *nextResult, name string) nextStep {
	t.Helper()
	for _, step := range report.Steps {
		if step.Step == name {
			return step
		}
	}
	require.Failf(t, "no such step", "the report owes no %s step: %v", name, stepNames(report))
	return nextStep{}
}

// A round that owes nothing says so with no steps and no next one, rather than
// a review of no cells or a settling of no claims.
func TestACompleteRoundOwesNothing(t *testing.T) {
	statusHome(t)
	layout, err := state.Default()
	require.NoError(t, err)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	var cells strings.Builder
	for _, cell := range []struct{ unit, hash string }{{"u1", "h1"}, {"u2", "h2"}} {
		for _, role := range []string{"convention", "correctness", "intent-coverage"} {
			cells.WriteString(`{"unit":"` + cell.unit + `","role":"` + role + `","result":"pass","unit_hash":"` +
				cell.hash + `","head":"` + meta.Head + `","round":1}` + "\n")
		}
	}
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileCoverage, []byte(cells.String())))
	require.NoError(t, held.Write(state.FileIntentGaps, []byte(
		`{"claim":"`+fixtureIssue+`#c2","set_aside_note":"`+fixtureIssue+`#n1","head":"`+meta.Head+`","round":1}`+"\n"+
			`{"claim":"`+fixtureIssue+`#c3","set_aside_note":"`+fixtureIssue+`#n1","head":"`+meta.Head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())

	report := nextOfFixture(t)

	assert.Empty(t, report.Steps)
	assert.Nil(t, report.Next)
}

// §10.4.4: a round whose intent axis is active and whose mapping is not
// recorded owes the intent pass before anything else.
func TestAnUnmappedRoundOwesTheIntentPassFirst(t *testing.T) {
	statusHome(t)
	layout, err := state.Default()
	require.NoError(t, err)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.MappingRound, meta.MappingHead = 0, ""
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())

	report := nextOfFixture(t)

	require.NotEmpty(t, report.Steps)
	assert.Equal(t, "intent", report.Steps[0].Step)
}

// §10.4.3 and §10.4.4: a round briefed from a file with neither claims nor a
// mapping recorded owes the claims first, naming the file the brief read —
// `cr claims record` does not inherit it — and then the intent pass, whose
// three commands emit, store the mapping, and store the cells.
func TestAnUnclaimedRoundOwesTheClaimsAndThenTheIntentPass(t *testing.T) {
	_, issue, _ := rerecordHome(t)
	target := fixturePR + " --repo " + fixtureSlug

	report := nextOfFixture(t)

	require.GreaterOrEqual(t, len(report.Steps), 2)
	assert.Equal(t, []string{"claims", "intent"}, stepNames(&report)[:2])
	claims := report.Steps[0]
	assert.Equal(t, actorAgent, claims.Actor)
	assert.Equal(t, []string{"cr claims record " + target + " <claims.ndjson> --intent-file " + issue},
		claims.Commands)
	intent := report.Steps[1]
	assert.Equal(t, actorAgent, intent.Actor)
	assert.Equal(t, []string{
		"cr review " + target + " --axis intent",
		"cr map record " + target + " <mapping.ndjson>",
		"cr cells record " + target + " <cells.ndjson>",
	}, intent.Commands)

	recordClaimsFile(t, issue, rerecordClaims...)

	report = nextOfFixture(t)

	assert.NotContains(t, stepNames(&report), "claims", "recorded claims are owed no more")
	require.NotEmpty(t, report.Steps)
	assert.Equal(t, "intent", report.Steps[0].Step)
}

// §10.4.9: a posted record still awaiting a verdict is the agent's recheck,
// naming the record, and is not the human's draft.
func TestAPostedRecordOwesTheRecheck(t *testing.T) {
	statusHome(t)
	layout, err := state.Default()
	require.NoError(t, err)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	holdRecords(t, layout, fixtureOwner, fixtureProject, fixturePRNumber,
		`{"id":"f3","kind":"question","summary":"why is the retry unbounded?","state":"posted",`+
			`"thread_id":"PRRT_q","head":"`+meta.Head+`","round":1}`)

	report := nextOfFixture(t)

	recheck := stepNamed(t, &report, "recheck")
	assert.Equal(t, actorAgent, recheck.Actor)
	assert.Equal(t, []string{"f3"}, recheck.Items)
	assert.Equal(t, []string{"cr recheck " + fixturePR + " --repo " + fixtureSlug}, recheck.Commands)
	assert.NotContains(t, stepNames(&report), "draft", "a posted record is not unsent")
}
