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

// resolvingGh installs a `gh` that resolves a thread the way GitHub does only
// when it is handed the mutation's variable as gh fills one — the field
// `thread=<id>` — and fails the way GitHub failed v0.5.0 otherwise.
//
// A shim that answered every call would have passed v0.5.0's resolve, which
// sent its variable in a shape gh does not read. So this one is only as
// generous as the real door: the same `Variable $thread … invalid value`
// GitHub returned, on the same argv.
func resolvingGh(t *testing.T, thread string) (transcript string) {
	t.Helper()
	dir := t.TempDir()
	transcript = filepath.Join(dir, "transcript")
	shim := filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(shim, []byte(
		"#!/bin/sh\n"+
			"printf '%s\\n' \"$*\" >> "+transcript+"\n"+
			"for argument in \"$@\"; do\n"+
			"  if [ \"$argument\" = 'thread="+thread+"' ]; then\n"+
			"    echo '{\"data\":{\"resolveReviewThread\":{\"thread\":{\"id\":\""+thread+"\",\"isResolved\":true}}}}'\n"+
			"    exit 0\n"+
			"  fi\n"+
			"done\n"+
			"echo 'gh: Variable $thread of type ID! was provided invalid value' >&2\n"+
			"exit 1\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return transcript
}

// withdrawnWaivers reads one of §7.4.4's two files back, one object per line.
func withdrawnWaivers(t *testing.T, path string) []map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	lines := make([]map[string]any, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		var held map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &held))
		lines = append(lines, held)
	}
	return lines
}

// §9.6.2: a confirmed withdrawal resolves the thread, writes the waiver its
// disposition scopes, records the outcome in the triage ledger under the
// posting's key, and moves the record — and its waiver is keyed from the hash
// the record stamped, so no tree is read.
//
// The `kept` the posting wrote is seeded first, because replacing it is the
// point: one raise, one outcome, and §7.3.2's counts still partition the
// events. A ledger holding both would count the record as kept and withdrawn
// at once, and the demotion rate would divide a class's retractions by a raise
// that had been counted twice.
func TestAConfirmedWithdrawalWaivesTheConcernAndReplacesItsKeptOutcome(t *testing.T) {
	for verb, want := range map[string]struct {
		outcome finding.Outcome
		file    func(state.Layout) string
	}{
		"wrong": {finding.OutcomeWithdrawnWrong, func(l state.Layout) string {
			return l.WaiversFile(answeredOwner, answeredRepo)
		}},
		"not-here": {finding.OutcomeWithdrawnNotHere, func(l state.Layout) string {
			return l.PRFile(answeredOwner, answeredRepo, answeredPRNum, state.FileWaivers)
		}},
	} {
		t.Run(verb, func(t *testing.T) {
			layout := settlingHome(t, settledRecord("f3", "question", "posted", "PRRT_a"))
			posted := &finding.Finding{ID: "f3", Class: "unbounded-retry", Role: "correctness"}
			require.NoError(t, finding.RecordOutcomes(layout, answeredOwner, answeredRepo,
				[]finding.Settled{{Record: posted, Outcome: finding.OutcomeKept}},
				&finding.TriageOccasion{PR: answeredPRNum, Round: 1, Head: settledHead, At: time.Now()}))
			transcript := resolvingGh(t, "PRRT_a")

			out, err := runIn(t, "withdraw", answeredPR, "f3", verb, "--confirm", "--repo", answeredSlug)

			require.NoError(t, err)
			assert.Contains(t, out, `"posted": true`)
			calls, err := os.ReadFile(transcript)
			require.NoError(t, err)
			assert.Contains(t, string(calls), "resolveReviewThread", "the thread was resolved on GitHub")

			stored := settledAs(t, layout, "f3")
			assert.Equal(t, "withdrawn", stored["state"])
			assert.EqualValues(t, 1, stored["round"], "the record stays in the round that posted it")

			waivers := withdrawnWaivers(t, want.file(layout))
			require.Len(t, waivers, 1, "§7.4.1's scope follows the disposition")
			assert.Equal(t, verb, waivers[0]["disposition"])
			assert.Equal(t, "unbounded-retry", waivers[0]["class"])
			assert.Equal(t, "fedcba9876543210", waivers[0]["content_hash"],
				"§9.2.4: the key is the hash the record stamped, read without a tree")
			assert.Equal(t, finding.WithdrawalReason, waivers[0]["reason"])

			events, err := finding.TriageEvents(layout, answeredOwner, answeredRepo)
			require.NoError(t, err)
			require.Len(t, events, 1, "the withdrawal replaced the kept rather than joining it")
			assert.Equal(t, finding.TriageAction(want.outcome), events[0].Action)
			assert.Equal(t, 1, events[0].Round)
		})
	}
}

// The record's own move is §9.6.2's last write, and a caller has to be told
// when it fails: a withdrawal whose record is still `posted` was not made,
// whatever the thread and the waiver say.
//
// gremlins found this. Negating the guard on MoveSent's error left `cr
// withdraw --confirm` reporting the record withdrawn over a findings.ndjson
// that still held it posted. The pull request's directory is made read-only
// after the fixture is written, so the thread is resolved and the `wrong`
// waiver and outcome, which live beside the repository and not under the pull
// request, still land; only the move refuses.
func TestAFailedWithdrawalMoveIsReportedRatherThanSwallowed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes through a read-only directory")
	}
	layout := settlingHome(t, settledRecord("f3", "question", "posted", "PRRT_a"))
	resolvingGh(t, "PRRT_a")
	dir := filepath.Dir(layout.PRFile(answeredOwner, answeredRepo, answeredPRNum, state.FileFindings))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	require.NoError(t, os.Chmod(dir, 0o500))

	_, err := runIn(t, "withdraw", answeredPR, "f3", "wrong", "--confirm", "--repo", answeredSlug)

	require.Error(t, err, "a record still posted was not withdrawn")
	assert.Contains(t, err.Error(), dir, "the refusal names where the write failed")
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, "posted", settledAs(t, layout, "f3")["state"])
}

// §9.6.2 takes the disposition from the reviewer and from nowhere else: a
// withdrawal that does not say which it is, or names neither, is refused as
// the malformed invocation it is, before anything is read or sent.
func TestAWithdrawalWithoutItsDispositionIsRefused(t *testing.T) {
	layout := settlingHome(t, settledRecord("f3", "question", "posted", "PRRT_a"))

	_, missing := runIn(t, "withdraw", answeredPR, "f3", "--repo", answeredSlug)
	require.Error(t, missing)
	assert.Equal(t, ExitUsage, exitCodeFor(missing))

	_, unknown := runIn(t, "withdraw", answeredPR, "f3", "maybe", "--confirm", "--repo", answeredSlug)
	require.Error(t, unknown)
	assert.Equal(t, ExitUsage, exitCodeFor(unknown))
	assert.Contains(t, unknown.Error(), "wrong or not-here")
	assert.Equal(t, "posted", settledAs(t, layout, "f3")["state"])
}
