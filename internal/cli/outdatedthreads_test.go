package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// outdatedThreadsAnswer is the reviewThreads page the gh shim answers with: on
// lib.go, a thread GitHub marks outdated (no current line, original line 4)
// and a current one on head line 4, both opened by a human.
const outdatedThreadsAnswer = `{"data":{"repository":{"pullRequest":{"reviewThreads":{` +
	`"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[` +
	`{"id":"PRRT_outdated","isResolved":false,"isOutdated":true,"path":"lib.go",` +
	`"line":null,"startLine":null,"originalLine":4,"originalStartLine":null,"diffSide":"RIGHT",` +
	`"comments":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[` +
	`{"id":"PRRC_outdated","url":"https://example.invalid/1","body":"parse ignores its error",` +
	`"createdAt":"2026-09-01T10:00:00Z","author":{"__typename":"User","login":"ayse"}},` +
	`{"id":"PRRC_reply","url":"https://example.invalid/2","body":"fixed in the next push",` +
	`"createdAt":"2026-09-01T11:00:00Z","author":{"__typename":"User","login":"mehmet"}}]}},` +
	`{"id":"PRRT_current","isResolved":true,"isOutdated":false,"path":"lib.go",` +
	`"line":4,"startLine":null,"originalLine":4,"originalStartLine":null,"diffSide":"RIGHT",` +
	`"comments":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[` +
	`{"id":"PRRC_current","url":"https://example.invalid/3","body":"why parse first?",` +
	`"createdAt":"2026-09-02T10:00:00Z","author":{"__typename":"User","login":"ayse"}}]}}` +
	`]}}}}}`

// outdatedThreadsHome is a checkout behind a pull request that changes lib.go
// and other.go, with the `generic` profile configured and a gh shim answering
// the pull request and outdatedThreadsAnswer, and nothing briefed yet. It
// returns the intent file the brief reads.
func outdatedThreadsHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("lib.go", "package lib\n\nfunc Load() {}\n")
	write("other.go", "package lib\n\nfunc Save() {}\n")
	mustGit(t, dir, "add", "lib.go", "other.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("lib.go", "package lib\n\nfunc Load() {\n\tparse()\n\tstore()\n}\n")
	write("other.go", "package lib\n\nfunc Save() {\n\tflush()\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })

	shim, answers := t.TempDir(), t.TempDir()
	pr := filepath.Join(answers, "pr.json")
	require.NoError(t, os.WriteFile(pr, []byte(`{"data":{"repository":{"pullRequest":{`+
		`"number":`+fixturePR+`,"title":"retry the upload","body":"","headRefName":"`+fixtureHeadBranch+`",`+
		`"headRefOid":"`+head+`","baseRefName":"main","baseRefOid":"`+base+`"}}}}`), 0o600))
	threads := filepath.Join(answers, "threads.json")
	require.NoError(t, os.WriteFile(threads, []byte(outdatedThreadsAnswer), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(shim, "gh"), []byte(
		"#!/bin/sh\ncase \"$*\" in\n  *reviewThreads*) exec cat "+threads+" ;;\n  *) exec cat "+pr+" ;;\nesac\n"), 0o700))
	t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	config := layout.RepoConfig(fixtureOwner, fixtureProject)
	require.NoError(t, os.MkdirAll(filepath.Dir(config), 0o700))
	require.NoError(t, os.WriteFile(config, []byte(`{"profile":"generic"}`), 0o600))

	intentFile := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(intentFile, []byte("Loading parses before it stores.\n"), 0o600))
	return intentFile
}

// promptSection is the body of the prompt section titled title: the text after
// its heading line and blank line, up to the next section.
func promptSection(t *testing.T, prompt, title string) string {
	t.Helper()
	heading := "\n## " + title + "\n\n"
	at := strings.Index(prompt, heading)
	require.GreaterOrEqual(t, at, 0, "the prompt has no section %q", title)
	body := prompt[at+len(heading):]
	if end := strings.Index(body, "\n## "); end >= 0 {
		body = body[:end]
	}
	return body
}

// QA D-W3-2: a thread GitHub marks outdated, ingested through gh, is listed
// marked outdated with its original line, author, state, body and replies in
// the prompts of every unit on its file and in no other file's, while §3.5.3's
// attachment of the current thread on that file is what it was.
func TestAnOutdatedThreadIsListedInThePromptsOfItsFilesUnitsOnly(t *testing.T) {
	intentFile := outdatedThreadsHome(t)
	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", intentFile)
	require.NoError(t, err)
	var briefed struct {
		Units []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"units"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	paths := make(map[string]string, len(briefed.Units))
	for _, u := range briefed.Units {
		paths[u.ID] = u.Path
	}
	require.ElementsMatch(t, []string{"lib.go", "other.go"}, []string{paths["u1"], paths["u2"]},
		"the fixture is only worth anything with a unit on each file")

	prompts := fanOut(t, "--axis", axis.Intent).Prompts
	require.Len(t, prompts, len(briefed.Units), "one intent prompt per unit")
	listed := "GitHub marks these threads outdated: a later push changed the code they were written on, so " +
		"they name no current line and are not attached to any unit by position (§3.5.3). Each shows the " +
		"lines it named in the diff it was written against, which need not be this unit's code. Whether one " +
		"already covers a finding is your decision; when it does, the finding is recorded with suppressed_by " +
		"naming the thread (§3.5.4).\n" +
		"- PRRT_outdated, outdated, originally at lib.go:4-4 (RIGHT), by ayse, resolved false:\n" +
		"  parse ignores its error\n" +
		"  - reply by mehmet: fixed in the next push\n"
	attached := "Whether a thread already covers a finding is your decision; when it does, the " +
		"finding is recorded with suppressed_by naming the thread (§3.5.4).\n" +
		"- PRRT_current at lib.go:4-4 (RIGHT), by ayse, resolved true:\n" +
		"  why parse first?\n"
	for _, prompt := range prompts {
		switch paths[prompt.Unit] {
		case "lib.go":
			assert.Equal(t, listed, promptSection(t, prompt.Text, "Outdated human threads on lib.go (§3.5.1)"))
			assert.Equal(t, attached,
				promptSection(t, prompt.Text, "Existing human threads near this unit (§3.5.3)"))
		case "other.go":
			assert.Equal(t, "No human thread on other.go is outdated.\n",
				promptSection(t, prompt.Text, "Outdated human threads on other.go (§3.5.1)"))
			assert.Equal(t, "No human thread is anchored within 10 lines of this unit.\n",
				promptSection(t, prompt.Text, "Existing human threads near this unit (§3.5.3)"))
		default:
			t.Fatalf("prompt for unit %s on an unexpected file %q", prompt.Unit, paths[prompt.Unit])
		}
	}
}
