package cli

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// clockField is the one field a reproduction is compared without, and
// clockedFiles are the only files it is masked in.
//
// §2.1.1 makes a command reproducible given the same state directory, the same
// head SHA and the same inputs, and the wall clock is none of the three.
// §9.1.1's transition journal records when a record moved because the spec asks
// it to, and its `at` is the time of the run and nothing else about the line
// is. Everything else — every other file, every other field of the journal, and
// every byte the command prints — is compared exactly. The triage events and
// rule statistics carry an `at` of their own, and they are not masked: none of
// the commands below writes either, and a line of any file not listed here is
// compared whole, so a new timestamp reaching one of them fails rather than
// hiding behind this mask.
const clockField = "at"

var clockedFiles = []string{state.FileTransitions}

// §2.1.1: every command is reproducible given the same state directory, the same
// head SHA, and the same inputs.
//
// Each command runs twice over byte-identical state: the tree is captured, the
// command runs, the tree is put back exactly as captured, and it runs again.
// Running it twice in a row would not test the rule for a command that writes,
// because the second run would start from the first run's state rather than the
// same one. The head is the fixture's fixed commit and the input files are the
// same paths both times.
//
// What is compared is what a run produces: the bytes it printed, and the state
// tree it left, with clockField masked in clockedFiles and nowhere else. The
// commands are a writer of cells and a writer of records over `cr record`'s
// fixture, and a reader of both over `cr status`'s, so the comparison covers a
// run that leaves state behind and one that must leave none.
func TestACommandRunTwiceOverTheSameStateProducesTheSameBytes(t *testing.T) {
	t.Run("writers", func(t *testing.T) {
		recordedHomeWithCells(t)
		cells := filepath.Join(t.TempDir(), "cells.ndjson")
		require.NoError(t, os.WriteFile(cells, []byte(`{"unit":"u1","role":"convention","result":"pass"}`+"\n"), 0o600))
		records := writeRecordFile(t, "merged.ndjson", aRecord("f2", "u1"), aRecord("f3", "u2"))

		wrote := runTwiceOverTheSameState(t, true, "cells", "record", recordPR, cells, "--repo", recordSlug)
		wrote = append(wrote, runTwiceOverTheSameState(t, true, "record", recordPR, records, "--repo", recordSlug)...)
		assert.Positive(t, clockedLines(wrote...), "no clocked line was written, so the mask was never exercised")
	})
	t.Run("reader", func(t *testing.T) {
		statusHome(t)
		runTwiceOverTheSameState(t, false, "status", fixturePR, "--repo", fixtureSlug)
	})
}

// runTwiceOverTheSameState runs args over the state CR_HOME holds, puts that
// state back, runs them again, and asserts the two runs printed the same bytes
// and left the same tree. A writer must have changed the tree and a reader must
// not have. It returns the tree the first run left, and leaves the second's in
// place for whatever runs next.
func runTwiceOverTheSameState(t *testing.T, writes bool, args ...string) []map[string][]byte {
	t.Helper()
	root := os.Getenv(state.HomeEnv)
	name := "cr " + strings.Join(args[:2], " ")
	before := captureTree(t, root)
	first, err := runCLIPrinting(t, args...)
	require.NoError(t, err, name)
	afterFirst := captureTree(t, root)
	restoreTree(t, root, before)
	second, err := runCLIPrinting(t, args...)
	require.NoError(t, err, name)
	afterSecond := captureTree(t, root)

	require.NotEmpty(t, first, "%s printed nothing, so there was nothing to compare", name)
	assert.Equal(t, first, second, "§2.1.1: %s printed different bytes over the same state", name)
	assert.Equal(t, maskClock(t, afterFirst), maskClock(t, afterSecond),
		"§2.1.1: %s left a different state tree over the same state", name)
	if writes {
		assert.NotEqual(t, before, afterFirst, "%s wrote nothing, so the state comparison proved nothing", name)
	} else {
		assert.Equal(t, before, afterSecond, "%s is a reader and changed the state tree", name)
	}
	return []map[string][]byte{afterFirst}
}

// captureTree is every directory and file under root, by path relative to it; a
// directory maps to nil so an empty one is kept.
func captureTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	tree := map[string][]byte{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == root {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			tree[rel] = nil
			return nil
		}
		body, err := os.ReadFile(path)
		tree[rel] = append([]byte{}, body...)
		return err
	}))
	return tree
}

// restoreTree replaces everything under root with tree, directories first.
func restoreTree(t *testing.T, root string, tree map[string][]byte) {
	t.Helper()
	require.NoError(t, os.RemoveAll(root))
	require.NoError(t, os.MkdirAll(root, 0o700))
	paths := make([]string, 0, len(tree))
	for rel := range tree {
		paths = append(paths, rel)
	}
	slices.Sort(paths)
	for _, rel := range paths {
		path := filepath.Join(root, rel)
		if tree[rel] == nil {
			require.NoError(t, os.MkdirAll(path, 0o700))
			continue
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, tree[rel], 0o600))
	}
}

// maskClock is tree with clockField replaced on every line of clockedFiles, and
// every other byte left as it was.
func maskClock(t *testing.T, tree map[string][]byte) map[string]string {
	t.Helper()
	out := make(map[string]string, len(tree))
	for rel, body := range tree {
		if !slices.Contains(clockedFiles, filepath.Base(rel)) {
			out[rel] = string(body)
			continue
		}
		lines := make([]string, 0)
		for line := range bytes.Lines(body) {
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(line, &fields), "%s", rel)
			fields[clockField] = json.RawMessage(`"<clock>"`)
			masked, err := json.Marshal(fields)
			require.NoError(t, err)
			lines = append(lines, string(masked))
		}
		out[rel] = strings.Join(lines, "\n")
	}
	return out
}

// clockedLines counts the lines of clockedFiles across trees.
func clockedLines(trees ...map[string][]byte) int {
	n := 0
	for _, tree := range trees {
		for rel, body := range tree {
			if slices.Contains(clockedFiles, filepath.Base(rel)) {
				n += bytes.Count(body, []byte{'\n'})
			}
		}
	}
	return n
}
