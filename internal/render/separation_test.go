package render

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/finding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// persistence is what code would have to reach for to put a value on disk: the
// filesystem, an encoder, and cr's own state package, which owns every path of
// §2.2 and every write to one.
var persistence = []string{
	"os",
	"io",
	"io/fs",
	"path/filepath",
	"encoding/json",
	"github.com/deligoez/cr/internal/state",
}

// recordKeepers are the packages that hold or write what cr stores: §2.2's
// paths and locks, §6.1's findings, §3.6's notes, §3.3's claims, §4.6's units,
// and §5.3's coverage cells.
var recordKeepers = []string{"state", "finding", "note", "intent", "unit", "testadequacy"}

// importsOf reads the import paths of every non-test Go file in dir.
func importsOf(t *testing.T, dir string) map[string][]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	byFile := make(map[string][]string, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ImportsOnly)
		require.NoError(t, err)
		paths := make([]string, 0, len(parsed.Imports))
		for _, imported := range parsed.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			require.NoError(t, err)
			paths = append(paths, path)
		}
		byFile[filepath.Join(dir, name)] = paths
	}
	require.NotEmpty(t, byFile, "a scan of no files proves nothing about %s", dir)
	return byFile
}

// "Findings are stored in English; reader-facing prose is produced at draft
// time" is a separation, and this is the first of its two halves: no rendering
// decision reaches stored state.
//
// This package is where every rendering decision v0.1 makes lives — the
// language, the built-in question labels, and the set of bodies the language
// governs. It is asserted structurally rather than by inspection, from both
// directions. Nothing here can write: no file imports the filesystem, an
// encoder, or internal/state, so a label cannot be persisted even by accident.
// And nothing that stores a record can render: none of the packages that hold
// cr's state imports this one, so no stored value can be chosen by a language.
//
// internal/finding is imported here and is deliberately not a contradiction: a
// grade is the record's own stored vocabulary, in English, and reading it is
// what keys the label table. The dependency runs from the renderer to the
// record, which is the direction §8.1.2 has it run.
func TestNoRenderingDecisionCanReachStoredState(t *testing.T) {
	for file, paths := range importsOf(t, ".") {
		for _, path := range paths {
			assert.NotContainsf(t, persistence, path,
				"%s imports %s: a rendering decision must not be able to reach disk", file, path)
		}
	}

	const self = "github.com/deligoez/cr/internal/render"
	for _, pkg := range recordKeepers {
		dir := filepath.Join("..", pkg)
		for file, paths := range importsOf(t, dir) {
			assert.NotContainsf(t, paths, self,
				"%s imports the render domain: what cr stores is decided before any language is", file)
		}
	}
}

// prose names what a row of a stored record would be called if it held
// reader-facing text: the rendered body, the §8.1.4 label prepended to it, a
// language, or a translation of any of them.
var prose = []string{"body", "label", "comment", "lang", "prose", "translat", "rendered"}

