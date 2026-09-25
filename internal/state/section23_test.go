package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// specVersion is the contract this build implements, and the document specRows
// reads. It moves with the code rather than with the newest file in `spec/`: a
// spec written ahead of its implementation is not yet what cr does, and a guard
// pointed at one would fail on every row the build has not reached.
const specVersion = "0.16.0"

// specRows reads §2.3's table out of the normative document and returns the
// file name each row names, in the table's own order.
//
// The spec is parsed rather than transcribed here, because the claim being made
// is about the spec: every file cr writes under a pull request's state
// directory has a row, and cr writes nothing the table omits. A copy of the
// table kept in this file could satisfy both directions while the document
// said something else entirely.
func specRows(t *testing.T) []string {
	t.Helper()
	spec, err := os.ReadFile(filepath.Join("..", "..", "spec", specVersion+".md"))
	require.NoError(t, err)
	document := string(spec)

	start := strings.Index(document, "\n### 2.3 Per-PR state\n")
	require.GreaterOrEqual(t, start, 0, "spec/"+specVersion+".md has no §2.3 heading")
	end := strings.Index(document[start:], "\n1. All writes")
	require.Positive(t, end, "§2.3's table has no numbered list after it")

	names := make([]string, 0)
	for line := range strings.SplitSeq(document[start:start+end], "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		names = append(names, strings.SplitN(line, "`", 3)[1])
	}
	require.NotEmpty(t, names, "a guard over none of §2.3's rows proves nothing")
	return names
}

// §2.3's table and the names this package admits are one list, in one order.
//
// Both directions matter and for different reasons. A row with no name behind
// it is a file the spec promises and no command can write; a name with no row
// is state cr keeps where nobody documented it, which is how `notes.ndjson` and
// a per-pull-request `rule-stats.ndjson` were found in a v0.1-era state tree
// long after both had moved elsewhere.
func TestSection23sTableIsTheFileSetThisPackageAdmits(t *testing.T) {
	admitted := PRNamed()
	for _, name := range RoundNamed() {
		admitted = append(admitted, roundsDirName+"/<n>/"+name)
	}

	assert.Equal(t, specRows(t), admitted,
		"§2.3's table is the whole of what a pull request's state directory holds, in its order")
}

// The lock admits every name §2.3 gives and refuses every other, so the table
// decides what a pull request's state directory holds rather than whichever
// call site somebody remembered.
//
// The refusals are the half with teeth. Write is the one door to that
// directory, and before checkPRFile a caller could publish any name that stayed
// inside it — which no test over a populated tree would catch unless some
// command happened to do it.
func TestTheLockWritesSection23sFilesAndRefusesEveryOther(t *testing.T) {
	admitted := PRNamed()
	for _, name := range RoundNamed() {
		admitted = append(admitted, filepath.Join(roundsDirName, "3", name))
	}

	l := New(t.TempDir())
	require.NoError(t, l.EnsurePR("acme", "web", 42))
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, held.Unlock()) })
	require.NoError(t, held.EnsureRound(3))

	for _, name := range admitted {
		assert.NoErrorf(t, held.Write(name, []byte("{}\n")), "§2.3 gives a pull request %s", name)
	}

	for name, because := range map[string]string{
		"notes.ndjson":                   "a file of no row at all",
		"rule-stats.ndjson":              "§2.2 keeps this one per repository, not per pull request",
		"draft.md":                       "a round's artefact does not belong beside the flat files",
		"rounds/3/notes.md":              "a name no round is given",
		"rounds/0/draft.md":              "meta.json's 0 means no round has been opened",
		"rounds/later/draft.md":          "a round's directory is numbered",
		"rounds/3/extra/draft.md":        "nothing nests below a round",
		"fanout/3/u1/review-role.ndjson": "§4.6.2's output files are the agent's, and cr writes none of them",
		filepath.Join("..", "meta.json"): "a name that climbs out of the directory",
		"sandbox-baseline.json.tmp":      "a name that only looks like a row",
	} {
		assert.Errorf(t, held.Write(name, []byte("{}\n")), "%s: %s", name, because)
		_, err := os.Stat(filepath.Join(l.PRDir("acme", "web", 42), filepath.FromSlash(name)))
		assert.Truef(t, os.IsNotExist(err), "%s was refused and written anyway", name)
	}
}
