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
// §5.3's coverage cells, and §5.5's probe records.
//
// probe is here because this package imports it: §8.1.7's evidence region
// carries a probe record verbatim, which is the direction §8.1.2 has the
// dependency run — the renderer reads the record. Listing it closes the
// reverse, so no probe record can be shaped by a language.
var recordKeepers = []string{"state", "finding", "note", "intent", "unit", "testadequacy", "probe"}

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

// The second half of the separation: no stored record carries reader-facing
// prose.
//
// §6.1's field table is the whole of what `findings.ndjson` holds, and §8.1.2
// says what the two text rows it does have are for — `summary` and `evidence`
// are English, and cr renders a block's *initial* body from them. The prose the
// author reads is written over that body in `draft.md` and kept in
// `rounds/<n>/rendered.json` per §7.1.5, which is a round's artefact and not a
// record: a record never holds a line in render.lang, so nothing stored has to
// be re-rendered when the setting changes.
func TestNoStoredRecordCarriesReaderFacingProse(t *testing.T) {
	rows := append(finding.Fields(), finding.CitationFields()...)
	require.NotEmpty(t, rows)
	for _, row := range rows {
		for _, name := range prose {
			assert.NotContainsf(t, row.Name, name,
				"§6.1's %q row would hold reader-facing prose in a stored record", row.Name)
		}
	}

	// The one thing a label and a record do share is the grade, and they
	// share it in the record's direction: the stored English word is carried
	// into the Turkish line, rather than a Turkish word being carried into
	// the record.
	for _, grade := range grades {
		line, ok := QuestionLabel(LangTR, grade)
		require.True(t, ok)
		assert.Contains(t, line, string(grade),
			"the reader is shown the word the record stores, so they can find the record")
	}
}
