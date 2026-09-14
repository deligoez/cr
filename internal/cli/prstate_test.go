package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// closedAt is the time the shims below report a pull request closed or merged.
const closedAt = "2026-09-10T08:00:00Z"

// pullRequestStates are the two states a pull request is disclosed in, with the
// sentence each is disclosed with, less the pull request's name.
var pullRequestStates = map[string]string{
	gh.StateClosed: " is closed: GitHub reports it closed at " + closedAt,
	gh.StateMerged: " is merged: GitHub reports it merged at " + closedAt,
}

// closureSentence is closureDisclosure's sentence for the pull request named.
func closureSentence(name, prState string) string {
	return name + pullRequestStates[prState] + ", so a review posted now reaches a pull request " +
		"that is no longer open; cr does not refuse the post, and whether to send it stays --confirm's"
}

// pullRequestAnswer is GitHub's answer to cr's pull request query, in state.
func pullRequestAnswer(t *testing.T, number, head, base, prState string) []byte {
	t.Helper()
	node := map[string]any{
		"number": json.Number(number), "title": fixtureIssue + " retry the upload", "body": "Closes " + fixtureIssue + ".",
		"headRefName": fixtureHeadBranch, "headRefOid": head, "baseRefName": "main", "baseRefOid": base,
		"state": prState, "closedAt": nil, "mergedAt": nil,
	}
	if prState != "OPEN" {
		node["closedAt"] = closedAt
	}
	if prState == gh.StateMerged {
		node["mergedAt"] = closedAt
	}
	body, err := json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": node}}})
	require.NoError(t, err)
	return body
}

// readPullRequestFromGitHub puts GitHub's own read back behind the seam crHome
// stubs, so the pull request's state comes from the gh shim on PATH.
func readPullRequestFromGitHub(t *testing.T) {
	t.Helper()
	restore := currentPullRequest
	currentPullRequest = githubPullRequest
	t.Cleanup(func() { currentPullRequest = restore })
}

// stateHome is a pull request whose head changes one file, with a gh shim on
// PATH answering the pull request query in prState, briefed through `cr brief`
// so `cr status` has a round to report.
func stateHome(t *testing.T, prState string) (briefed string) {
	t.Helper()
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.go"), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("package lib\n\nfunc Load() {}\n")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("package lib\n\nfunc Load() {\n\tparse()\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))
	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })

	answers := t.TempDir()
	pr := filepath.Join(answers, "pr.json")
	threads := filepath.Join(answers, "threads.json")
	require.NoError(t, os.WriteFile(pr, pullRequestAnswer(t, fixturePR, head, base, prState), 0o600))
	require.NoError(t, os.WriteFile(threads, []byte(`{"data":{"repository":{"pullRequest":{`+
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}}}`), 0o600))
	shim := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(shim, "gh"), []byte(
		"#!/bin/sh\ncase \"$*\" in\n  *reviewThreads*) exec cat "+threads+" ;;\n  *) exec cat "+pr+" ;;\nesac\n"), 0o700))
	t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(rerecordIssue), 0o600))
	readPullRequestFromGitHub(t)
	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	return printed
}

// cr brief and cr status disclose a closed or merged pull request, with the
// state and the time GitHub reports, first in the honesty channel and in the
// text a person reads; an open pull request is disclosed as nothing.
//
// Measured on release QA before the fix (D-W1-6): brief, review, status and
// post read the same for closed PR #10 and merged PR #11 as for an open one.
func TestBriefAndStatusDiscloseAClosedOrMergedPullRequest(t *testing.T) {
	for prState := range pullRequestStates {
		t.Run(prState, func(t *testing.T) {
			want := closureSentence(fixtureSlug+"#"+fixturePR, prState)
			assert.Equal(t, want, postHonesty(t, stateHome(t, prState))[0], "cr brief")
			brief := throughATerminal(t, "brief", fixturePR, "--repo", fixtureSlug, "--issue", fixtureIssue,
				"--intent-file", writeIssueFile(t), "--no-color")
			lines := strings.Split(strings.ReplaceAll(brief, "\r\n", "\n"), "\n")
			require.Greater(t, len(lines), 1)
			assert.Equal(t, "  "+want, lines[1], "the terminal says it right under the pull request's name")

			status, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
			require.NoError(t, err)
			assert.Equal(t, want, postHonesty(t, status)[0], "cr status")
			assert.Contains(t, strings.Split(strings.ReplaceAll(
				throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--quiet"), "\r\n", "\n"), "\n"),
				want, "the terminal prints it on a line of its own, --quiet or not")
		})
	}
	t.Run("OPEN", func(t *testing.T) {
		assert.NotContains(t, postHonesty(t, stateHome(t, "OPEN"))[0], "GitHub reports it",
			"the control: an open pull request is disclosed as nothing")
		status, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
		require.NoError(t, err)
		for _, said := range postHonesty(t, status) {
			assert.NotContains(t, said, "no longer open")
		}
	})
}

// writeIssueFile writes the issue text stateHome briefs with.
func writeIssueFile(t *testing.T) string {
	t.Helper()
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(rerecordIssue), 0o600))
	return issue
}

// runPostTo is runPost whose standard error is the file at stderr.
func runPostTo(t *testing.T, stderr string, args ...string) (printed string, err error) {
	t.Helper()
	sink, err := os.Create(stderr)
	require.NoError(t, err)
	t.Cleanup(func() { sink.Close() })
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(sink)
	cmd.SetArgs(append([]string{"post"}, args...))
	err = cmd.Execute()
	return out.String(), err
}

// cr post discloses a closed or merged pull request in its dry run, first in
// the text a person reads, and a confirmed run prints it again on standard
// error before the review-creation request — and sends the review all the
// same, because the state warns and never refuses.
//
// Measured on release QA before the fix (D-W1-6): `cr post --confirm` on merged
// PR #11 went straight to the POST without a word about the state.
func TestPostDisclosesAClosedOrMergedPullRequestBeforeTheRequest(t *testing.T) {
	for prState := range pullRequestStates {
		t.Run(prState, func(t *testing.T) {
			draftedHome(t, aCitedRecord("f1"))
			redraft(t)
			shim := ghShimming(t, builtPayload(t))
			answers := t.TempDir()
			pr := filepath.Join(answers, "pr.json")
			require.NoError(t, os.WriteFile(pr, pullRequestAnswer(t, draftPR, draftHead, draftHead, prState), 0o600))
			stderr := filepath.Join(answers, "stderr")
			ghWrapping(t, shim, "case \"$*\" in\n  *headRefOid*) exec cat '"+pr+"' ;;\n"+
				"  *'--method POST'*) cp '"+stderr+"' '"+stderr+".at-post' ;;\nesac")
			readPullRequestFromGitHub(t)
			want := closureSentence(draftSlug+"#"+draftPR, prState)

			printed, err := runPost(t, draftPR, "--repo", draftSlug)
			require.NoError(t, err)
			assert.Equal(t, []string{want}, postHonesty(t, printed), "the dry run's document")
			first, _, _ := strings.Cut(throughATerminal(t, "post", draftPR, "--repo", draftSlug, "--no-color"), "\n")
			assert.Equal(t, want, strings.TrimSuffix(first, "\r"), "the dry run's terminal says it first")

			printed, err = runPostTo(t, stderr, draftPR, "--repo", draftSlug, "--confirm")
			require.NoError(t, err, "the state warns and never refuses")
			require.Len(t, shim.writes(t), 1, "the review was sent")
			atPost, err := os.ReadFile(stderr + ".at-post")
			require.NoError(t, err, "the request reached the shim")
			assert.Equal(t, want+"\n", string(atPost), "said on standard error before the request left")
			assert.Equal(t, []string{want}, postHonesty(t, printed), "and in the confirmed run's document")
		})
	}
}
