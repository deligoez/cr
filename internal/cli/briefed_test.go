package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// The pull request the guard is exercised against: one cr holds no state for at
// all, which is what a command reaches before any `cr brief` has run.
const (
	unbriefedOwner = "acme"
	unbriefedRepo  = "api"
	unbriefedSlug  = unbriefedOwner + "/" + unbriefedRepo
	unbriefedPR    = "31"
)

// briefRuns is the argv each command that reads §2.3's per-round state is
// exercised with, keyed by the command as it is typed.
//
// It is a table because cr cannot invent a command's arguments, and it is
// checked against the tree by the guard below rather than trusted: a command
// §3.7 obliges that gets built has to arrive here before it can be left out of
// the absent list, so "every command that reads them" keeps meaning every one
// that exists.
//
// Every file each command reads is prepared, so a run reaches the state read
// rather than stopping at its own arguments. A command refused at an argument
// would exit 2 and prove nothing about the refusal being tested.
func briefRuns(claims, issue, merged string) map[string][]string {
	return map[string][]string{
		"record": {"record", unbriefedPR, merged, "--repo", unbriefedSlug},
		"claims record": {
			"claims", "record", unbriefedPR, claims,
			"--repo", unbriefedSlug, "--intent-file", issue,
		},
	}
}

// section37Obliges names the three commands brief-creates-state's third
// criterion asks for, spelled the way they are typed.
//
// §4.6 gives `cr review` the unit set, §4.5.6 gives `cr cells record` the unit
// id it rejects a cell by, and §4.1.6 gives `cr map record` the same for a
// mapping. All three read units.ndjson, which §3.7 makes `cr brief`'s to write.
var section37Obliges = [][]string{
	{"review"},
	{"cells", "record"},
	{"map", "record"},
}

// unbriefedInputs writes the files the guarded commands are pointed at and
// returns their paths, alongside a state root holding no round for the pull
// request under test.
//
// Every file sits outside the state tree: they are the agent's own output, and
// what is being tested is the refusal that happens before any of them is read.
func unbriefedInputs(t *testing.T) (claims, issue, merged string) {
	t.Helper()
	root := crHome(t)
	layout := state.New(root)
	require.NoError(t, layout.Init())

	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		return path
	}
	claims = write("claims.ndjson",
		`{"id":"CR-31#c1","text":"Retry on 5xx.","source":"acceptance","span":"Retry on 5xx."}`+"\n")
	issue = write("issue.txt", "Retry on 5xx.\n")
	merged = write("merged.ndjson", `{"id":"f1","kind":"finding","role":"correctness",`+
		`"class":"unchecked-error","severity":"high","unit":"u1",`+
		`"anchor":{"path":"app.go","side":"RIGHT","start_line":1,"line":1,`+
		`"content_hash":"0123456789abcdef"},"summary":"The error is dropped.",`+
		`"evidence":"The second result is assigned to the blank identifier."}`+"\n")
	return claims, issue, merged
}

// runCLI runs one invocation against whatever CR_HOME points at and returns
// what it refused.
func runCLI(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	return cmd.Execute()
}

// Every command that reads §2.3's per-round state refuses a pull request no
// `cr brief` has opened a round on, with §11.2's code 4 and a refusal naming
// `cr brief`.
//
// This is the second half of brief-creates-state: §3.7's permission to persist
// the derived inputs is an obligation because §4.1.6, §4.5.6, §9.3.1 and §3.5.3
// read those files as authoritative. A command that recomputed the units
// instead would answer §4.1.6 against a unit set no round ever recorded, and
// nothing in the run would say so.
func TestACommandReadingPerPRStateBeforeABriefExitsFourNamingBrief(t *testing.T) {
	claims, issue, merged := unbriefedInputs(t)

	for name, argv := range briefRuns(claims, issue, merged) {
		t.Run(name, func(t *testing.T) {
			err := runCLI(t, argv...)
			require.Error(t, err)

			var missing *state.NotBriefedError
			require.ErrorAs(t, err, &missing,
				"the refusal has to be the one §11.2 codes 4")
			assert.Equal(t, ExitState, exitCodeFor(err))
			assert.Contains(t, err.Error(), "cr brief "+unbriefedPR,
				"§12.4: the refusal names the next actionable step")
		})
	}
}

