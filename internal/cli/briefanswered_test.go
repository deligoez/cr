package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// answeredThreadHome is a briefed pull request whose author, alice, replied
// twice inside a reviewer's thread, with a record f1 an answer can be filed
// against. It returns the issue file a brief of it reads.
func answeredThreadHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(path, body string) string {
		t.Helper()
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		return path
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write(filepath.Join(dir, "lib.go"), "package lib\n\nfunc Load() {}\n")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write(filepath.Join(dir, "lib.go"), "package lib\n\nfunc Load() {\n\tparse()\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))
	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })

	answers := t.TempDir()
	pr := write(filepath.Join(answers, "pr.json"), `{"data":{"repository":{"pullRequest":{`+
		`"number":`+fixturePR+`,"title":"`+fixtureIssue+` load","body":"Closes `+fixtureIssue+`.",`+
		`"headRefName":"`+fixtureHeadBranch+`","headRefOid":"`+head+`","baseRefName":"main",`+
		`"baseRefOid":"`+base+`","author":{"login":"alice"}}}}}`)
	comment := func(id, body, login string) string {
		return `{"id":"` + id + `","url":"https://example.invalid/` + id + `","body":"` + body +
			`","createdAt":"2026-09-01T09:00:00Z","author":{"__typename":"User","login":"` + login + `"}}`
	}
	threads := write(filepath.Join(answers, "threads.json"), `{"data":{"repository":{"pullRequest":{`+
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"id":"PRRT_1",`+
		`"isResolved":false,"isOutdated":false,"path":"lib.go","line":4,"startLine":4,"originalLine":4,`+
		`"originalStartLine":null,"diffSide":"RIGHT","comments":{"pageInfo":{"hasNextPage":false,"endCursor":""},`+
		`"nodes":[`+comment("PRRC_1", "Does parse retry?", "bob")+`,`+
		comment("PRRC_2", "No: retries are out of scope for this issue.", "alice")+`,`+
		comment("PRRC_3", "Parsing is covered by the fixture tests.", "alice")+`]}}]}}}}}`)
	shims := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(shims, "gh"), []byte(
		"#!/bin/sh\ncase \"$*\" in\n  *reviewThreads*) exec cat "+threads+" ;;\n  *) exec cat "+pr+" ;;\nesac\n"),
		0o700))
	t.Setenv("PATH", shims+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	issue := write(filepath.Join(t.TempDir(), "issue.txt"), "Load parses its input.\n")
	offeredReplies(t, issue)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings,
		[]byte(`{"id":"f1","state":"posted","head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return issue
}

// offeredReplies runs `cr brief` and returns the ids of the replies it offers
// as candidate notes.
func offeredReplies(t *testing.T, issue string) []string {
	t.Helper()
	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	var report struct {
		CandidateNotes []struct {
			Reply struct {
				ID string `json:"id"`
			} `json:"reply"`
		} `json:"candidate_notes"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	ids := make([]string, 0, len(report.CandidateNotes))
	for _, offered := range report.CandidateNotes {
		ids = append(ids, offered.Reply.ID)
	}
	return ids
}

// A reply the context store already holds is no longer offered as a candidate
// note, whether `cr answer` or `cr note` stored it, and a reply it does not hold
// still is. The comparison is §1.4's, so a stored text differing only in
// whitespace counts as the reply.
//
// Measured on release QA before the fix (D-W2-2): after `cr answer` stored the
// author's reply as CR-1#n2, the next brief still offered it with a `cr note`
// command, inviting a second note.
func TestABriefStopsOfferingAReplyTheContextStoreHolds(t *testing.T) {
	issue := answeredThreadHome(t)
	require.Equal(t, []string{"PRRC_2", "PRRC_3"}, offeredReplies(t, issue), "the control: both replies are offered")

	_, err := runCLIPrinting(t, "answer", fixturePR, "f1",
		"No:  retries are out of scope\tfor this issue.  ", "--source", "thread", "--repo", fixtureSlug)
	require.NoError(t, err)
	assert.Equal(t, []string{"PRRC_3"}, offeredReplies(t, issue))

	_, err = runCLIPrinting(t, "note", fixtureIssue, "Parsing is covered by the fixture tests.",
		"--source", "thread", "--pr", fixturePR)
	require.NoError(t, err)
	assert.Equal(t, []string{}, offeredReplies(t, issue))
}
