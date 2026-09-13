package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/state"
)

// ghShimTranscript is what a run asked the `gh` on its PATH for, one
// invocation per line.
//
// The shim is a real program on PATH rather than a seam inside cr, and that is
// the whole reason it exists. §8.5's gate is worth what the write door is
// worth, and internal/gh reaches that door with os/exec — so a test that
// replaced a Go function would be measuring the code above the door and would
// say nothing about whether a run started `gh`. Replacing the binary measures
// the last thing that happens before the network.
type ghShimTranscript struct {
	// path is the file the shim appends each invocation to.
	path string
	// body is the file the shim copies the request body of a POST into, as
	// gh would have sent it: standard input for `--input -`, the named file
	// otherwise. It is empty for a shim that records no body.
	body string
}

// sentBody is the request body the shim's last POST carried.
func (g *ghShimTranscript) sentBody(t *testing.T) []byte {
	t.Helper()
	require.NotEmpty(t, g.body, "this shim records no request body")
	body, err := os.ReadFile(g.body)
	require.NoError(t, err, "no POST reached the shim")
	return body
}

// calls are the invocations the shim received, in order.
func (g *ghShimTranscript) calls(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(g.path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
}

// writes are the invocations that carried a request body, which §8.3.3 makes
// the review-creation call and nothing else.
func (g *ghShimTranscript) writes(t *testing.T) []string {
	t.Helper()
	sent := make([]string, 0)
	for _, call := range g.calls(t) {
		if strings.Contains(call, "--method POST") || strings.Contains(call, "--input") {
			sent = append(sent, call)
		}
	}
	return sent
}

// ghShimming installs a `gh` on PATH that records every invocation, answers
// §3.5.1's review-thread query with one thread per comment of review, and
// answers everything else with an empty document.
//
// The threads are built from the payload the dry run printed, so their bodies
// are the bodies the confirmed run will send — which is what §8.3.3's read-back
// matches on. Nothing about the shim is steerable from the environment:
// internal/gh's allowlist passes PATH, HOME, TMPDIR and SystemRoot and nothing
// else, so every answer is written into the script itself.
func ghShimming(t *testing.T, review *post.Review) *ghShimTranscript {
	t.Helper()
	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript")
	threads := filepath.Join(dir, "threads.json")
	created := filepath.Join(dir, "review.json")
	body := filepath.Join(dir, "body.json")

	require.NoError(t, os.WriteFile(threads, threadsPage(t, review), 0o600))
	require.NoError(t, os.WriteFile(created, []byte(
		`{"id":1,"node_id":"PRR_shim","state":"COMMENTED"}`+"\n"), 0o600))

	shim := filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(shim, []byte(
		"#!/bin/sh\n"+
			"printf '%s\\n' \"$*\" >> "+transcript+"\n"+
			"case \"$*\" in\n"+
			"  *reviewThreads*) exec cat "+threads+" ;;\n"+
			"  *'--method POST'*)\n"+
			"    input=''\n"+
			"    previous=''\n"+
			"    for argument in \"$@\"; do\n"+
			"      if [ \"$previous\" = --input ]; then input=$argument; fi\n"+
			"      previous=$argument\n"+
			"    done\n"+
			"    if [ \"$input\" = - ]; then cat > "+body+"; else cat \"$input\" > "+body+"; fi\n"+
			"    exec cat "+created+" ;;\n"+
			"  *) echo '{}' ;;\n"+
			"esac\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return &ghShimTranscript{path: transcript, body: body}
}

// threadsPage is §3.5.1's review-thread answer holding one thread per comment of
// review, each carrying the comment's body, which is what §8.3.3's read-back
// matches on, and hanging on the comment's range, as GitHub reports a thread a
// review comment opened: a one-line comment's start lines are null.
func threadsPage(t *testing.T, review *post.Review) []byte {
	t.Helper()
	nodes := make([]any, 0, len(review.Comments))
	for i := range review.Comments {
		comment := &review.Comments[i]
		var start any
		if comment.StartLine > 0 {
			start = comment.StartLine
		}
		nodes = append(nodes, map[string]any{
			"id": threadIDFor(i), "isResolved": false, "isOutdated": false,
			"path": comment.Path, "line": comment.Line, "startLine": start,
			"originalLine": comment.Line, "originalStartLine": start,
			"diffSide": comment.Side,
			"comments": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
				"nodes": []any{map[string]any{
					"id": "PRRC_" + threadIDFor(i), "url": "https://example.invalid/c",
					"body": comment.Body, "createdAt": "2026-09-12T00:00:00Z",
					"author": map[string]any{"__typename": "Bot", "login": "cr"},
				}},
			},
		})
	}
	page, err := json.Marshal(map[string]any{"data": map[string]any{
		"repository": map[string]any{"pullRequest": map[string]any{
			"reviewThreads": map[string]any{
				"pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
				"nodes":    nodes,
			},
		}},
	}})
	require.NoError(t, err)
	return page
}

// threadIDFor is the shim's node id for the thread one comment became, in
// GitHub's own prefix so a test reading posted.json sees the shape §8.3.3
// records.
func threadIDFor(at int) string {
	return "PRRT_kwDOShim" + string(rune('A'+at))
}

// builtPayload is the payload §8.5.1's dry run printed, which is the payload
// the confirmed run over the same state will send.
func builtPayload(t *testing.T) *post.Review {
	t.Helper()
	printed, err := runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	var report dryRun
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	require.NotNil(t, report.Payload)
	return report.Payload
}

// §8.5.2 in the affirmative: with `--confirm` the round reaches GitHub as one
// review, and everything §8.3.3, §9.1, §9.3.6 and §7.3.1 owe a sent round is
// written.
//
// The single call is §8.3.1 measured rather than described: the shim counts
// every invocation carrying a request body, and one review is one notification
// for the author. The body it was given is the payload §8.3.3 wrote, byte for
// byte, handed over on standard input, which is what `--input -` means.
func TestConfirmSendsOneReviewAndSettlesTheRound(t *testing.T) {
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"))
	redraft(t)
	built := builtPayload(t)
	shim := ghShimming(t, built)

	printed, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	assert.Contains(t, printed, `"posted": true`,
		"§12.6: this run performed the network write, and §8.5.4 bounds the claim to that")

	sent := shim.writes(t)
	require.Len(t, sent, 1, "§8.3.1: all of a round's comments are posted as one review")
	assert.Equal(t,
		"api repos/"+draftOwner+"/"+draftRepo+"/pulls/"+draftPR+"/reviews --method POST --input -",
		sent[0])
	payload, err := built.Payload()
	require.NoError(t, err)
	assert.Equal(t, string(payload)+"\n", string(shim.sentBody(t)),
		"§8.3.3: the bytes written before the call are the bytes the call sends")

	// §9.1's `queued` → `posted` row, and §9.3.6's index beside it.
	stored, err := state.ReadStamped[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	for i := range stored {
		assert.Equalf(t, finding.StatePosted, stored[i].State,
			"§9.1: `cr post --confirm` moves a queued record to posted, and %s is not there",
			stored[i].ID)
	}
	index, err := finding.PostedIndex(layout, draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	assert.Len(t, index, 1,
		"§9.3.6 holds one entry per key, and both records share §7.4.1's key")
	meta, err := layout.ReadMeta(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	assert.False(t, meta.PostUnresolved,
		"§8.4.4: the flag set before the call is cleared once the records are posted")

	// §8.3.3's thread ids, keyed by the record each comment came from.
	document := readPosted(t, layout)
	var threads map[string]string
	require.NoError(t, json.Unmarshal(document[postedThreads], &threads))
	assert.Equal(t, map[string]string{"f1": threadIDFor(0), "f2": threadIDFor(1)}, threads,
		"§8.3.3: the ids the call produced, keyed by the record each comment came from")

	// §7.3.1: one outcome event per queued record, written after the call
	// and beside the `raised` events `cr draft` wrote when it queued them.
	assert.Equal(t,
		[]string{"f1:raised", "f2:raised", "f1:kept", "f2:kept"}, ledger(t, layout),
		"§7.3.1: `cr post --confirm` settles every queued record exactly once")
}

// §8.4.2: a review GitHub refuses marks nothing posted, and §7.3.1's outcome
// events are not written either.
//
// The second half is round 8's triage-event-key-permits-contradiction: the
// reviewer's ordinary retry — mark one record `wrong` and run again — must
// write that record's only outcome, rather than a second one contradicting a
// `kept` from the run GitHub refused.
//
// Both halves run through `cr post --confirm` against a `gh` on PATH: the
// first run meets a gh that refuses the review, the reviewer marks f1 `wrong`
// in the draft, and the second run meets a gh that accepts it. Each record then
// holds exactly one outcome against its one raise.
func TestARejectedCallMarksNothingPosted(t *testing.T) {
	records := []*finding.Finding{aCitedRecord("f1"), aCitedRecord("f2")}
	records[1].Anchor.Path = "internal/api/second.go"
	layout := draftedHome(t, records...)
	redraft(t)
	rejectingShim(t)

	_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")

	require.Error(t, err)
	assert.Equal(t, ExitState, exitCodeFor(err), "§8.4.2 exits 4")
	assert.Contains(t, err.Error(), "f1", "§8.4.2 names the record behind the position")

	stored, err := state.ReadStamped[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	for i := range stored {
		assert.Equal(t, finding.StateQueued, stored[i].State, "§8.4.2: no state is marked posted")
	}

	index, err := finding.PostedIndex(layout, draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	assert.Empty(t, index, "§9.3.6's index records what the author received")
	assert.Equal(t, []string{"f1:raised", "f2:raised"}, ledger(t, layout),
		"§7.3.1's outcomes describe a review that exists, so a refused call leaves none")

	// The retry: one record re-triaged, and the same command run again
	// against a gh that accepts the review.
	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f1", `disposition=""`, `disposition="wrong"`))
	accepted := ghShimming(t, builtPayload(t))

	_, err = runPost(t, draftPR, "--repo", draftSlug, "--confirm")
	require.NoError(t, err)

	require.Len(t, accepted.writes(t), 1, "the retry sends one review")
	assert.Equal(t, []string{"f1:raised", "f2:raised", "f1:discarded-wrong", "f2:kept"}, ledger(t, layout),
		"§7.3.1: each record ends with one outcome against its one raise")
}

// rejectingShim installs a `gh` that fails the review-creation call the way
// GitHub refuses one: the error document on standard output, a one-line summary
// on standard error, and a non-zero exit.
//
// The document names the position rather than the record, because GitHub has
// never heard of a record — which is why §8.4.2 has cr map it back.
func rejectingShim(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte(
		"#!/bin/sh\n"+
			"case \"$*\" in\n"+
			"  *'--method POST'*)\n"+
			`    printf '%s' '{"message":"Validation Failed","errors":`+
			`[{"resource":"PullRequestReviewComment","field":"line",`+
			`"code":"invalid","message":"line must be part of the diff"}],"status":"422"}'`+"\n"+
			"    echo 'gh: Validation Failed (HTTP 422)' >&2\n"+
			"    exit 1 ;;\n"+
			"  *) echo '{}' ;;\n"+
			"esac\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// §8.5.3's four channels, each attempted and each failing to supply the gate.
//
// The assertion is the same for all four and is made at the write door rather
// than about the code: the run is given the channel, the `gh` on its PATH
// records everything it is asked, and no invocation carries a request body. A
// channel that had worked would show up there as the review-creation call and
// nowhere else.
//
// Two of the four are refused outright, which is §2.7 doing the work: a `CR_`
// variable or a config key whose name would address the confirmation gate is
// rejected with exit code 3. The other two are ignored — a profile carries no
// such field and an unknown key in one is not read, and a flag cr does not
// register is §11.2's malformed invocation. All four end the same way, which is
// what §8.5.3 asks: the flag is the only input the gate has.
func TestNoChannelButTheFlagSuppliesTheConfirmationGate(t *testing.T) {
	for name, channel := range map[string]func(*testing.T, state.Layout) []string{
		"a configuration setting": func(t *testing.T, l state.Layout) []string {
			t.Helper()
			require.NoError(t, os.WriteFile(l.Config(),
				[]byte(`{"post":{"confirm":true}}`), 0o600))
			return []string{draftPR, "--repo", draftSlug}
		},
		"an environment variable": func(t *testing.T, _ state.Layout) []string {
			t.Helper()
			t.Setenv("CR_POST_CONFIRM", "true")
			return []string{draftPR, "--repo", draftSlug}
		},
		"a profile field": func(t *testing.T, l state.Layout) []string {
			t.Helper()
			require.NoError(t, os.WriteFile(l.Profile("generic"), []byte(
				`{"id":"generic","match":{"files":[],"globs":["**/*"]},`+
					`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},`+
					`"confirm":true,"post":{"confirm":true}}`), 0o600))
			require.NoError(t, l.EnsureRepo(draftOwner, draftRepo))
			require.NoError(t, os.WriteFile(l.RepoConfig(draftOwner, draftRepo),
				[]byte(`{"profile":"generic"}`), 0o600))
			return []string{draftPR, "--repo", draftSlug}
		},
		"an alias": func(t *testing.T, _ state.Layout) []string {
			t.Helper()
			return []string{draftPR, "--repo", draftSlug, "--yes"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			layout := draftedHome(t, aCitedRecord("f1"))
			redraft(t)
			shim := ghShimming(t, &post.Review{})
			argv := channel(t, layout)

			printed, err := runPost(t, argv...)

			assert.Empty(t, shim.writes(t),
				"§8.5.3: %s does not supply --confirm, so nothing is sent", name)
			if err == nil {
				assert.Contains(t, printed, `"posted": false`,
					"a run that was not confirmed reports that it sent nothing")
			}
			stored, err := state.ReadStamped[finding.Finding](
				layout, draftOwner, draftRepo, draftPRNum, state.FileFindings, draftRound)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			assert.NotEqual(t, finding.StatePosted, stored[0].State,
				"§9.1 moves a record to posted only on a run that posted")
		})
	}
}

// §8.5.3's fourth channel from the other side: nothing in the command tree
// spells the gate a second way.
//
// An alias is the one of the four channels that would leave no file behind, so
// it is read off the tree rather than attempted: a cobra alias on `cr post`, a
// shorthand on `--confirm`, or a third local flag would each be a second way to
// ask for the network write, and the behavioural half above can only try the
// spellings somebody thought of.
func TestThePostCommandOffersTheGateExactlyOneSpelling(t *testing.T) {
	found, _, err := newRootCmd().Find([]string{"post"})
	require.NoError(t, err)

	assert.Empty(t, found.Aliases, "§8.5.3: an alias would be a second name for the gated command")
	confirm := found.Flag("confirm")
	require.NotNil(t, confirm, "§8.5.2 makes --confirm a flag")
	assert.Empty(t, confirm.Shorthand, "§8.5.3: a shorthand is an alias for the flag")

	local := make([]string, 0)
	found.LocalFlags().VisitAll(func(flag *pflag.Flag) { local = append(local, flag.Name) })
	assert.ElementsMatch(t, []string{"confirm", "reconcile"}, local,
		"§11 gives `cr post` two flags, and a third is a channel §8.5.3 forbids")

	var aliased []string
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if len(cmd.Aliases) > 0 {
			aliased = append(aliased, cmd.CommandPath())
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(newRootCmd())
	assert.Empty(t, aliased,
		"§11 is the whole surface, so no command answers to a second name either")
}
