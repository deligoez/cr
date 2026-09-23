package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// settledRecord is one line of findings.ndjson in the shape §9.5 and §9.6 act
// on: a record that reached GitHub, so it carries a thread.
//
// The head is the one briefedHome records, because §9.3.2 refuses every write
// under a head the round did not open at — and every command here but
// `cr recheck` writes.
func settledRecord(id, kind, recordState, thread string) string {
	return `{"id":"` + id + `","kind":"` + kind + `","role":"correctness","unit":"u1",` +
		`"class":"unbounded-retry","anchor":` + stampedAnchor + `,` +
		`"summary":"is the retry unbounded?","state":"` + recordState + `",` +
		`"thread_id":"` + thread + `","head":"0f1e2d3","round":1}`
}

// stampedAnchor is an anchor as `cr record` stores it since §9.2.4, carrying
// the waiver key's hash, so a withdrawal forms its waiver without reading a
// tree — which is the case a withdrawal after a force-push is in.
const stampedAnchor = `{"path":"lib.go","side":"RIGHT","start_line":4,"line":4,` +
	`"content_hash":"0123456789abcdef","context_hash":"fedcba9876543210"}`

// settledHead is the head the round below is opened at, and the one every
// seeded record carries. crHome's default currentPullRequest echoes the
// recorded head, so §9.3.1 finds the round current and no GitHub answer has to
// be stood up for commands that are not about a moved head.
const settledHead = "0f1e2d3"

// settlingHome is a briefed pull request at settledHead, holding the records a
// case needs.
//
// It opens the round itself rather than calling briefedHome, which writes no
// round: every command here but `cr recheck` writes per-PR state, and §9.3.2
// refuses a write under a round that was never opened.
func settlingHome(t *testing.T, lines ...string) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(answeredOwner, answeredRepo, answeredPRNum))

	held, err := layout.LockPR(answeredOwner, answeredRepo, answeredPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: answeredOwner, Repo: answeredRepo, PR: answeredPRNum,
		IssueKey: "CR-7", Round: 1, Head: settledHead,
	}))
	require.NoError(t, held.Unlock())

	holdRecords(t, layout, answeredOwner, answeredRepo, answeredPRNum, lines...)
	return layout
}

// settledAs reads one record back out of findings.ndjson.
func settledAs(t *testing.T, l state.Layout, id string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(l.PRFile(answeredOwner, answeredRepo, answeredPRNum, state.FileFindings))
	require.NoError(t, err)
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		var held map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &held))
		if held["id"] == id {
			return held
		}
	}
	require.FailNow(t, "no record "+id+" was stored")
	return nil
}

// storedVerdicts reads verdicts.ndjson.
func storedVerdicts(t *testing.T, l state.Layout) []map[string]any {
	t.Helper()
	body, err := os.ReadFile(l.PRFile(answeredOwner, answeredRepo, answeredPRNum, state.FileVerdicts))
	require.NoError(t, err)
	lines := make([]map[string]any, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		if line == "" {
			continue
		}
		var held map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &held))
		lines = append(lines, held)
	}
	return lines
}

// §9.5.5: each of the three verbs does what it says, and only two of them move
// the record.
//
// `standing` is the case worth its own assertion. It changes no state and is
// recorded all the same, because §9.1.3 counts a `posted` record as open and a
// reviewer needs to tell one nobody read from one somebody read and left open —
// a distinction that did not exist before v0.5, when both sat in `posted`.
func TestEachVerdictSettlesWhatItSays(t *testing.T) {
	for verdict, want := range map[string]string{
		"answered":  "answered",
		"addressed": "addressed",
		"standing":  "posted",
	} {
		t.Run(verdict, func(t *testing.T) {
			layout := settlingHome(t, settledRecord("f3", "question", "posted", "PRRT_a"))

			out, err := runIn(t, "verify", answeredPR, "f3", verdict,
				"--evidence", "the author's reply names the commit", "--repo", answeredSlug)

			require.NoError(t, err)
			assert.Contains(t, out, "f3")
			assert.Equal(t, want, settledAs(t, layout, "f3")["state"])

			recorded := storedVerdicts(t, layout)
			require.Len(t, recorded, 1, "§9.5.5 appends a line whatever the verdict decides")
			assert.Equal(t, verdict, recorded[0]["verdict"])
			assert.Equal(t, "the author's reply names the commit", recorded[0]["evidence"])
			assert.Equal(t, "f3", recorded[0]["record"])
		})
	}
}

// §9.5.5: `answered` is a question's word, and a finding given it is refused
// with exit 1.
//
// The refusal matters because §9.1.3 counts an `answered` record as settled: a
// finding nobody dealt with, filed under a word no reply could have earned,
// would be counted as dealt with by every convergence figure that reads the
// states.
func TestAnsweredIsRefusedForARecordThatAsksNothing(t *testing.T) {
	layout := settlingHome(t, settledRecord("f4", "finding", "posted", "PRRT_a"))

	_, err := runIn(t, "verify", answeredPR, "f4", "answered",
		"--evidence", "the author says it is fixed", "--repo", answeredSlug)

	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Contains(t, err.Error(), "§9.5.5 admits answered only on a question")
	assert.Equal(t, "posted", settledAs(t, layout, "f4")["state"], "the record did not move")
}

// §9.5.5 requires a judgement to say what it rests on, and the flag being
// required is only half of that: `--evidence ""` satisfies cobra and says
// nothing.
func TestAVerdictWithEmptyEvidenceIsRefused(t *testing.T) {
	layout := settlingHome(t, settledRecord("f3", "question", "posted", "PRRT_a"))

	_, err := runIn(t, "verify", answeredPR, "f3", "standing", "--evidence", "", "--repo", answeredSlug)

	require.Error(t, err)
	assert.Equal(t, ExitUsage, exitCodeFor(err))
	assert.Equal(t, "posted", settledAs(t, layout, "f3")["state"])
}

// §9.5.5's vocabulary is closed: a fourth verb is refused naming the three.
func TestAVerdictOutsideTheThreeVerbsIsRefused(t *testing.T) {
	settlingHome(t, settledRecord("f3", "question", "posted", "PRRT_a"))

	_, err := runIn(t, "verify", answeredPR, "f3", "fixed",
		"--evidence", "the author says so", "--repo", answeredSlug)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "answered, addressed, standing")
}

// §9.6.1: resolving a thread is how a concern stops being visible, so a record
// nobody settled is refused with exit 4.
//
// This is the refusal the section exists for. A thread resolved over an open
// concern is worse than one left open, because nobody will look at it again.
func TestResolveRefusesARecordNobodySettled(t *testing.T) {
	settlingHome(t, settledRecord("f3", "question", "posted", "PRRT_a"))

	_, err := runIn(t, "resolve", answeredPR, "f3", "--repo", answeredSlug)

	require.Error(t, err)
	assert.Equal(t, ExitState, exitCodeFor(err))
	assert.Contains(t, err.Error(), "resolves a thread only for a record in answered or addressed")
}

// settlingArgv is one of §9.6's two commands over one record, a withdrawal
// carrying the disposition §9.6.2 requires.
func settlingArgv(command, id string) []string {
	if command == "withdraw" {
		return []string{command, answeredPR, id, "wrong"}
	}
	return []string{command, answeredPR, id}
}

// §9.6: a record that never reached GitHub has no thread to act on, and both
// commands say so rather than reaching for one.
func TestSettlingARecordWithNoThreadIsRefused(t *testing.T) {
	for _, command := range []string{"resolve", "withdraw"} {
		t.Run(command, func(t *testing.T) {
			settlingHome(t, `{"id":"f5","kind":"question","summary":"unposted",`+
				`"state":"queued","head":"0f1e2d3","round":1}`)

			_, err := runIn(t, append(settlingArgv(command, "f5"), "--repo", answeredSlug)...)

			require.Error(t, err)
			assert.Equal(t, ExitState, exitCodeFor(err))
			assert.Contains(t, err.Error(), "carries no posted thread")
		})
	}
}

// §8.5 over §9.6's two writes: without `--confirm` each prints what it would do
// and neither moves the record nor reaches GitHub.
//
// Nothing stubs gh here, and that is the assertion: a run that tried to write
// would fail looking for the binary, so a clean exit is itself the evidence
// that no call was attempted.
func TestNeitherSettlingCommandWritesWithoutConfirm(t *testing.T) {
	for command, id := range map[string]string{"resolve": "f6", "withdraw": "f3"} {
		t.Run(command, func(t *testing.T) {
			layout := settlingHome(t,
				settledRecord("f3", "question", "posted", "PRRT_a"),
				settledRecord("f6", "question", "addressed", "PRRT_b"))

			out, err := runIn(t, append(settlingArgv(command, id), "--repo", answeredSlug)...)

			require.NoError(t, err)
			assert.Contains(t, out, `"posted": false`, "§12.6: nothing was sent")
			assert.Contains(t, out, `"confirm_given": false`,
				"§8.5.4: the one fact about the gate cr can establish")
			before := map[string]string{"f6": "addressed", "f3": "posted"}[id]
			assert.Equal(t, before, settledAs(t, layout, id)["state"],
				"§8.5.1: an unconfirmed run moves nothing")
		})
	}
}

// §9.5: `cr recheck` reports the round's posted records and writes no state.
//
// The report carries the record's kind, which is what tells a reader which of
// §9.5.5's verbs is even available — `answered` is a question's — and it names
// no verdict of its own, which is §9.5.6.
func TestRecheckReportsPostedRecordsAndJudgesNothing(t *testing.T) {
	layout := settlingHome(t,
		settledRecord("f3", "question", "posted", "PRRT_a"),
		settledRecord("f6", "finding", "addressed", "PRRT_b"))
	threadsAnswering(t, liveThread)

	out, err := runIn(t, "recheck", answeredPR, "--repo", answeredSlug)

	require.NoError(t, err)
	assert.Contains(t, out, "f3", "the posted record is reported")
	assert.NotContains(t, out, "f6", "a settled record is not a concern to read back")
	for _, verdict := range []string{"answered", "addressed", "standing"} {
		assert.NotContains(t, out, verdict, "§9.5.6: cr reports and does not conclude")
	}
	assert.Equal(t, "posted", settledAs(t, layout, "f3")["state"], "the read moved nothing")
	assert.NoFileExists(t,
		layout.PRFile(answeredOwner, answeredRepo, answeredPRNum, state.FileVerdicts)+".tmp")
}

// §9.1: the states v0.5 adds are terminal, so a settled record cannot be
// verified a second time — the refusal names the record and where it stands.
func TestASettledRecordTakesNoSecondVerdict(t *testing.T) {
	settlingHome(t, settledRecord("f6", "question", "answered", "PRRT_b"))

	_, err := runIn(t, "verify", answeredPR, "f6", "addressed",
		"--evidence", "the code is gone too", "--repo", answeredSlug)

	require.Error(t, err)
	assert.Equal(t, ExitState, exitCodeFor(err))
	var illegal *finding.IllegalTransitionError
	require.ErrorAs(t, err, &illegal)
	assert.Equal(t, "f6", illegal.Record)
}

// liveThread is PRRT_a as GitHub holds it after the review was posted: on
// lines 17–20 now, first written at 16–19, and answered once.
const liveThread = `{"id":"PRRT_a","isResolved":false,"isOutdated":false,"path":"lib.go",` +
	`"line":20,"startLine":17,"originalLine":19,"originalStartLine":16,"diffSide":"RIGHT",` +
	`"comments":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[` +
	`{"id":"PRRC_open","url":"https://example.invalid/1","body":"is the retry unbounded?",` +
	`"createdAt":"2026-09-01T10:00:00Z","author":{"__typename":"User","login":"reviewer"}},` +
	`{"id":"PRRC_reply","url":"https://example.invalid/2","body":"bounded at three",` +
	`"createdAt":"2026-09-01T11:00:00Z","author":{"__typename":"User","login":"author"}}]}}`

// threadsAnswering installs a `gh` whose reviewThreads page is the nodes given.
func threadsAnswering(t *testing.T, nodes ...string) {
	t.Helper()
	page := `{"data":{"repository":{"pullRequest":{"reviewThreads":{` +
		`"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[` + strings.Join(nodes, ",") + `]}}}}}`
	restore := ghClient
	ghClient = func() gh.Client {
		return gh.WithRunner(func(_ ...string) (string, error) { return page, nil })
	}
	t.Cleanup(func() { ghClient = restore })
}

// §9.5.2 and §9.5.3: `cr recheck` reports what GitHub says now, every line of
// it, and not what `cr brief` stored when the round opened. Measured before
// the fix on deligoez/cr-qa#25: straight after `cr post --confirm`, recheck
// printed line 0, no replies and not resolved, while GitHub held line 20, and
// start lines were never printed at all.
func TestRecheckReadsWhatGitHubSaysNow(t *testing.T) {
	settlingHome(t, settledRecord("f3", "question", "posted", "PRRT_a"))
	threadsAnswering(t, liveThread)

	out, err := runIn(t, "recheck", answeredPR, "--repo", answeredSlug)

	require.NoError(t, err)
	var report struct {
		Concerns []struct {
			ID                string `json:"id"`
			StartLine         int    `json:"start_line"`
			Line              int    `json:"line"`
			OriginalStartLine int    `json:"original_start_line"`
			OriginalLine      int    `json:"original_line"`
			Replies           []struct {
				Body string `json:"body"`
			} `json:"replies"`
		} `json:"concerns"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	require.Len(t, report.Concerns, 1)
	concern := report.Concerns[0]
	assert.Equal(t, [4]int{17, 20, 16, 19},
		[4]int{concern.StartLine, concern.Line, concern.OriginalStartLine, concern.OriginalLine})
	require.Len(t, concern.Replies, 1)
	assert.Equal(t, "bounded at three", concern.Replies[0].Body)
}

// §9.5.4's re-run is not implemented, and a record naming a probe says so
// rather than reading as though the probe had been looked at.
func TestRecheckSaysAProbeWasNotReRun(t *testing.T) {
	probed := strings.Replace(settledRecord("f3", "question", "posted", "PRRT_a"),
		`"summary"`, `"probe":"p1","summary"`, 1)
	settlingHome(t, probed)
	threadsAnswering(t, liveThread)

	out, err := runIn(t, "recheck", answeredPR, "--repo", answeredSlug)

	require.NoError(t, err)
	assert.Contains(t, out, "record f3 names probe p1, and §9.5.4's re-run")
}
