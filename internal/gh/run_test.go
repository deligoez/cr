package gh

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubGh puts an executable named gh at the front of PATH, so a test can
// exercise the real invocation — the argument list, the environment, and the
// exit status — without a token, a network call, or the gh binary being
// installed at all.
func stubGh(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\n"+script+"\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// A run is given an allowlisted environment, not the ambient one. GH_HOST and
// its neighbours decide which server answers, and the same owner and name
// resolve to a different repository on a different host: cr would then ingest
// a stranger's threads as this pull request's, and §3.5.4 lets an ingested
// thread suppress a finding. §2.1.1 makes the host the invocation's business
// and not the shell's.
func TestAReadIgnoresTheAmbientGhEnvironment(t *testing.T) {
	recorded, err := filepath.Abs(filepath.Join("testdata", "threads.json"))
	require.NoError(t, err)
	seen := filepath.Join(t.TempDir(), "environment")
	stubGh(t, "env > "+seen+"\ncat "+recorded)
	t.Setenv("GH_HOST", "ghe.example.invalid")
	t.Setenv("GH_TOKEN", "not-a-real-token")
	t.Setenv("GH_CONFIG_DIR", filepath.Join(t.TempDir(), "not-the-users-gh"))

	threads, err := New().Threads("cli", "cli", 11451)
	require.NoError(t, err)
	assert.Len(t, threads, 2)

	environment, err := os.ReadFile(seen)
	require.NoError(t, err)
	assert.NotContains(t, string(environment), "ghe.example.invalid")
	assert.NotContains(t, string(environment), "not-a-real-token")
	assert.NotContains(t, string(environment), "not-the-users-gh")
	assert.Contains(t, string(environment), "GH_PAGER=cat")
	assert.Contains(t, string(environment), "GH_PROMPT_DISABLED=1")
}

