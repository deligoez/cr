package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// runTriage runs `cr triage` with args and stdin against whatever CR_HOME
// points at, and returns what it printed and what it refused.
func runTriage(t *testing.T, stdin string, args ...string) (printed string, err error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append([]string{"triage"}, args...))
	err = cmd.Execute()
	return out.String(), err
}

// triageRecords are the round the triage tests draft: two cited findings and a
// cited question, each anchored in a file of its own so every waiver is
// attributable to one verb. The question carries both a §8.1.4 label region
// above its body and a §8.1.7 evidence region beneath it, and the findings the
// evidence region alone, so a replaced body is measured between owned regions.
func triageRecords() []*finding.Finding {
	records := []*finding.Finding{aCitedRecord("f1"), aCitedRecord("f2"), aCitedRecord("f3")}
	for _, record := range records {
		record.Anchor.Path = "internal/api/" + record.ID + ".go"
	}
	records[2].Kind = finding.KindQuestion
	records[2].Summary = "Is the error Decode returns dropped, f3?"
	return records
}

// generatedBody is the agent region `cr draft` renders for a record.
func generatedBody(record *finding.Finding) string {
	return record.Summary + "\n\n" + record.Evidence
}

// draftedTriageHome is a state root holding triageRecords, drafted once.
func draftedTriageHome(t *testing.T) state.Layout {
	t.Helper()
	layout := draftedHome(t, triageRecords()...)
	redraft(t)
	return layout
}

// afterTriage is everything the next commands make of a triaged draft: the
// draft.md the triage left, what `cr draft` reported and wrote, the round's
// records and both waiver files, and what the `cr post` dry run printed or
// refused.
type afterTriage struct {
	triaged   string
	redrafted map[string]any
	draftErr  string
	rendered  string
	findings  []finding.Finding
	here      []finding.WaiverRecord
	wide      []finding.WaiverRecord
	posted    string
	postErr   string
	postCode  int
}

// readAfterTriage runs `cr draft` and a `cr post` dry run over the current
// draft and gathers afterTriage. The draft's path is dropped from the report,
// because it names the state root and that is the one thing two homes differ
// in.
func readAfterTriage(t *testing.T, layout state.Layout) afterTriage {
	t.Helper()
	var after afterTriage
	after.triaged = readDraft(t, layout)
	printed, err := runDraft(t, draftPR, "--repo", draftSlug)
	if err != nil {
		after.draftErr = err.Error()
	} else {
		require.NoError(t, json.Unmarshal([]byte(printed), &after.redrafted))
		delete(after.redrafted, "path")
	}
	after.rendered = readDraft(t, layout)
	after.findings, err = state.ReadRecords[finding.Finding](layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err)
	after.here, err = finding.PullRequestWaivers(layout, draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	after.wide, err = finding.RepositoryWaivers(layout, draftOwner, draftRepo)
	require.NoError(t, err)
	after.posted, err = runPost(t, draftPR, "--repo", draftSlug)
	if err != nil {
		after.postErr, after.postCode = err.Error(), exitCodeFor(err)
	}
	return after
}

// §7.2 and §7.2.4: triage happens by editing the file or through `cr triage`,
// with the same effect. Each verb is run in one home and its hand edit made in
// another from the same round, and the two are held to one another three
// commands deep: the draft.md each leaves byte for byte, then what `cr draft`
// reports, renders, stores and waives when it reads that draft back per §7.1.6,
// and then what the `cr post` dry run prints or refuses.
//
// Every case is also held to having changed something where its verb changes
// something, so two homes that agreed because neither edit landed cannot pass.
func TestEveryTriageVerbIsItsHandEdit(t *testing.T) {
	records := triageRecords()
	softened := "Does Decode's error reach the caller, f2?"
	rewritten := "Is the dropped error on line 44 deliberate, f3?"
	for _, tc := range []struct {
		name  string
		argv  []string
		stdin string
		hand  func(t *testing.T, file string) string
		keeps bool
	}{
		{
			name: "not-here deletes a block between two others",
			argv: []string{"f2", "not-here"},
			hand: func(t *testing.T, file string) string { return deleteBlock(t, file, "f2") },
		},
		{
			name: "not-here deletes the last block",
			argv: []string{"f3", "not-here"},
			hand: func(t *testing.T, file string) string { return deleteBlock(t, file, "f3") },
		},
		{
			name: "wrong sets the disposition in the marker",
			argv: []string{"f1", "wrong"},
			hand: func(t *testing.T, file string) string {
				return markerEdit(t, file, "f1", `disposition=""`, `disposition="wrong"`)
			},
		},
		{
			name: "soften changes the kind in the marker",
			argv: []string{"f1", "soften"},
			hand: func(t *testing.T, file string) string {
				return markerEdit(t, file, "f1", `kind="finding"`, `kind="question"`)
			},
		},
		{
			name:  "soften with a body from standard input rewrites the region above the evidence",
			argv:  []string{"f2", "soften", "--body-file", "-"},
			stdin: softened + "\n",
			hand: func(t *testing.T, file string) string {
				file = markerEdit(t, file, "f2", `kind="finding"`, `kind="question"`)
				return strings.Replace(file, generatedBody(records[1]), softened, 1)
			},
		},
		{
			name: "keep with a body file rewrites the region between the label and the evidence",
			argv: []string{"f3", "keep", "--body-file", "BODY"},
			hand: func(t *testing.T, file string) string {
				return strings.Replace(file, generatedBody(records[2]), rewritten, 1)
			},
		},
		{
			name:  "keep alone changes nothing",
			argv:  []string{"f1", "keep"},
			hand:  func(_ *testing.T, file string) string { return file },
			keeps: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			byHand := draftedTriageHome(t)
			untouched := readDraft(t, byHand)
			edited := tc.hand(t, untouched)
			assert.Equal(t, tc.keeps, edited == untouched, "the hand edit is the edit its verb names")
			writeDraft(t, byHand, edited)
			want := readAfterTriage(t, byHand)

			throughCR := draftedTriageHome(t)
			require.Equal(t, untouched, readDraft(t, throughCR), "both homes start from one draft")
			bodyFile := filepath.Join(t.TempDir(), "body.md")
			require.NoError(t, os.WriteFile(bodyFile, []byte(rewritten+"\n"), 0o600))
			argv := append([]string{draftPR}, tc.argv...)
			for i := range argv {
				if argv[i] == "BODY" {
					argv[i] = bodyFile
				}
			}
			_, err := runTriage(t, tc.stdin, append(argv, "--repo", draftSlug)...)
			require.NoError(t, err)
			got := readAfterTriage(t, throughCR)

			assert.Equal(t, want.triaged, got.triaged, "§7.2.4: draft.md is the hand edit's, byte for byte")
			assert.Equal(t, want.redrafted, got.redrafted, "`cr draft` reports the same triage")
			assert.Equal(t, want.draftErr, got.draftErr)
			assert.Equal(t, want.rendered, got.rendered, "`cr draft` renders the same draft")
			assert.Equal(t, want.findings, got.findings, "and stores the same records")
			assert.Equal(t, want.here, got.here, "§7.4: the same pull-request waivers")
			assert.Equal(t, want.wide, got.wide, "§7.4: the same repository waivers")
			assert.Equal(t, want.posted, got.posted, "`cr post` builds the same payload")
			assert.Equal(t, want.postErr, got.postErr)
			assert.Equal(t, want.postCode, got.postCode)
		})
	}
}

// The regenerated draft measures the verbs, not only their bytes: the deletion
// is a `not-here` discard with a pull-request waiver, `wrong` a discard with a
// repository waiver, the softened record a question, and a replaced body the
// one the reviewer is posted with.
func TestTriageVerbsReachTheRoundThroughTheDraft(t *testing.T) {
	layout := draftedTriageHome(t)
	body := filepath.Join(t.TempDir(), "body.md")
	require.NoError(t, os.WriteFile(body, []byte("\n\nWhy is the error dropped, f3?\n\n"), 0o600))
	for _, argv := range [][]string{
		{"f1", "wrong"}, {"f2", "not-here"}, {"f3", "keep", "--body-file", body},
	} {
		_, err := runTriage(t, "", append(append([]string{draftPR}, argv...), "--repo", draftSlug)...)
		require.NoError(t, err, "cr triage %v", argv)
	}

	report := redraft(t)

	outcomes := map[string]finding.Outcome{}
	for _, triaged := range report.Triaged {
		outcomes[triaged.ID] = triaged.Outcome
	}
	assert.Equal(t, map[string]finding.Outcome{
		"f1": finding.OutcomeDiscardedWrong, "f2": finding.OutcomeDiscardedNotHere,
	}, outcomes)
	assert.Equal(t, []string{"f3"}, report.Preserved, "the replaced body is kept as the reviewer's")
	assert.Equal(t, "repository", waiverScopeOf(t, layout, "internal/api/f1.go"))
	assert.Equal(t, "pull request", waiverScopeOf(t, layout, "internal/api/f2.go"))
	bodies, err := draft.Bodies(readDraft(t, layout))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"f3": "Why is the error dropped, f3?"}, bodies,
		"the region holds the file's content without its surrounding line feeds")
}

// §7.2.4's refusals, each driven through the command and each leaving draft.md
// exactly as it was: a record id with no block — never rendered, or deleted
// already — and `soften` on a question are exit code 1, and `--body-file`
// beside a discarding verb is exit code 2.
func TestTriageRefusesWithoutTouchingTheDraft(t *testing.T) {
	for _, tc := range []struct {
		name   string
		argv   []string
		before func(t *testing.T, file string) string
		want   error
		code   int
	}{
		{name: "an id the round never rendered", argv: []string{"f9", "keep"},
			want: &draft.NoBlockError{ID: "f9"}, code: ExitValidation},
		{name: "an id whose block is deleted", argv: []string{"f2", "wrong"},
			before: func(t *testing.T, file string) string { return deleteBlock(t, file, "f2") },
			want:   &draft.NoBlockError{ID: "f2"}, code: ExitValidation},
		{name: "soften on a question", argv: []string{"f3", "soften"},
			want: &draft.SoftenQuestionError{ID: "f3", At: 0}, code: ExitValidation},
		{name: "soften on a finding the reviewer already softened", argv: []string{"f1", "soften"},
			before: func(t *testing.T, file string) string {
				return markerEdit(t, file, "f1", `kind="finding"`, `kind="question"`)
			},
			want: &draft.SoftenQuestionError{ID: "f1", At: 0}, code: ExitValidation},
		{name: "a body beside not-here", argv: []string{"f1", "not-here", "--body-file", "-"},
			want: errBodyWithDiscard, code: ExitUsage},
		{name: "a body beside wrong", argv: []string{"f1", "wrong", "--body-file", "-"},
			want: errBodyWithDiscard, code: ExitUsage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout := draftedTriageHome(t)
			file := readDraft(t, layout)
			if tc.before != nil {
				file = tc.before(t, file)
				writeDraft(t, layout, file)
			}

			_, err := runTriage(t, "a body\n", append(append([]string{draftPR}, tc.argv...), "--repo", draftSlug)...)

			require.Error(t, err)
			assert.Equal(t, tc.code, exitCodeFor(err))
			var question *draft.SoftenQuestionError
			if errWant, isQuestion := tc.want.(*draft.SoftenQuestionError); isQuestion {
				require.ErrorAs(t, err, &question)
				errWant.At = lineOf(t, file, `<!-- cr:record id="`+errWant.ID+`"`)
			}
			assert.Equal(t, tc.want, err)
			assert.Equal(t, file, readDraft(t, layout), "a refused triage writes nothing")
		})
	}
}

// lineOf is the one-based line of the first line of file opening with prefix.
func lineOf(t *testing.T, file, prefix string) int {
	t.Helper()
	for i, line := range strings.Split(file, "\n") {
		if strings.HasPrefix(line, prefix) {
			return i + 1
		}
	}
	require.Failf(t, "no line", "no line opens with %s", prefix)
	return 0
}

// §7.2.4 prints the record id and the verb, as §12.1's document when piped and
// as a line in a terminal.
func TestTriagePrintsTheRecordAndTheVerb(t *testing.T) {
	layout := draftedTriageHome(t)
	path := layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft)

	printed, err := runTriage(t, "Is it dropped, f1?", draftPR, "f1", "soften", "--body-file", "-", "--repo", draftSlug)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"f1","verb":"soften","body_replaced":true,"path":`+quoteJSON(t, path)+`}`, printed)

	printed, err = runTriage(t, "", draftPR, "f2", "keep", "--repo", draftSlug)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"f2","verb":"keep","body_replaced":false,"path":`+quoteJSON(t, path)+`}`, printed)

	shown := throughATerminal(t, "triage", draftPR, "f2", "wrong", "--repo", draftSlug, "--no-color")
	assert.Equal(t, "f2: wrong in "+path+"\n", shown)
}

// quoteJSON is s as a JSON string literal.
func quoteJSON(t *testing.T, s string) string {
	t.Helper()
	encoded, err := json.Marshal(s)
	require.NoError(t, err)
	return string(encoded)
}

// A verb §7.2.4 does not name, and a body file cr cannot read, are refused
// before draft.md is written: the first as §11.2's usage error, the second as
// its file failure.
func TestTriageRefusesAnUnknownVerbAndAnUnreadableBody(t *testing.T) {
	layout := draftedTriageHome(t)
	file := readDraft(t, layout)

	_, err := runTriage(t, "", draftPR, "f1", "delete", "--repo", draftSlug)
	require.Error(t, err)
	assert.Equal(t, ExitUsage, exitCodeFor(err))
	assert.Equal(t, `"delete" is not a triage verb: §7.2.4 names not-here, wrong, soften, keep`, err.Error())

	missing := filepath.Join(t.TempDir(), "absent.md")
	_, err = runTriage(t, "", draftPR, "f1", "keep", "--body-file", missing, "--repo", draftSlug)
	var unreadable *state.FileError
	require.ErrorAs(t, err, &unreadable)
	assert.Equal(t, ExitFile, exitCodeFor(err))

	assert.Equal(t, file, readDraft(t, layout))
}

// §7.2.4 edits draft.md under the per-PR lock. The test holds the lock, lets
// the triage start, and writes a draft of its own while holding it — the write
// a concurrent `cr draft` makes — and only then lets go. A triage that took no
// lock has already written over the round's draft by then, and one that read
// before locking writes the draft it read back over the new one; only a
// triage that read and wrote under the lock leaves both edits in the file.
func TestTriageEditsTheDraftUnderThePerPRLock(t *testing.T) {
	layout := draftedTriageHome(t)
	body := filepath.Join(t.TempDir(), "body.md")
	require.NoError(t, os.WriteFile(body, []byte("Why is it dropped, f1?"), 0o600))

	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, err := runTriage(t, "", draftPR, "f1", "keep", "--body-file", body, "--repo", draftSlug)
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	concurrent := deleteBlock(t, readDraft(t, layout), "f2")
	require.NoError(t, held.WriteRound(draftRound, state.FileDraft, []byte(concurrent)))
	select {
	case err := <-done:
		require.Failf(t, "the triage did not wait", "it returned %v while the lock was held", err)
	default:
	}
	require.NoError(t, held.Unlock())
	require.NoError(t, <-done)

	bodies, err := draft.Bodies(readDraft(t, layout))
	require.NoError(t, err)
	assert.Equal(t, []string{"f1", "f3"}, markersIn(readDraft(t, layout)), "the concurrent deletion stands")
	assert.Equal(t, "Why is it dropped, f1?", bodies["f1"], "and so does the triage's body")
}
