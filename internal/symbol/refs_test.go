package symbol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §4.4.1's "the symbols they reference", read out of §4.3.1's index: the
// identifiers of a test file that some head declaration is named, in the order
// the file first names them, once each.
//
// Each exclusion is a line of the fixture. TestLoad is declared by the test
// file itself, on its own line, so that occurrence is the file declaring it and
// not referring to it; the comment names Store, which the head declares, and
// is not code; `lib` and `t` name nothing the head declares. Parse is named
// twice and attached once, and Load comes before Parse because the file names
// it first, whatever order the index holds them in.
func TestAFileReferencesTheHeadSymbolsItNames(t *testing.T) {
	index, built := Build("go", []File{
		{Path: "lib.go", Lines: []string{
			"package lib", "", "func Parse() {}", "", "func Load() {}", "", "func Store() {}",
		}},
		{Path: "lib_test.go", Lines: testFile},
	})
	require.True(t, built)

	assert.Equal(t, []string{"Load", "Parse"}, index.Referenced("lib_test.go", testFile))
}

// testFile is a Go test file calling two of lib.go's functions.
var testFile = []string{
	"package lib",
	"",
	"// Store is exercised elsewhere.",
	"func TestLoad(t *T) {",
	"\tLoad()",
	"\tParse()",
	"\tParse()",
	"}",
}

// A name the file declares on one line and names on another is referenced on
// the other: only the declaration's own line is the file declaring it.
func TestADeclaredNameUsedElsewhereInItsFileIsAReference(t *testing.T) {
	lines := []string{"package lib", "", "func helper() {}", "", "func TestIt() {", "\thelper()", "}"}
	index, built := Build("go", []File{{Path: "lib_test.go", Lines: lines}})
	require.True(t, built)

	assert.Equal(t, []string{"helper"}, index.Referenced("lib_test.go", lines))
}

// A file naming nothing the head declares references nothing, as an empty list
// rather than a nil one, per §12.
func TestAFileNamingNoHeadSymbolReferencesNothing(t *testing.T) {
	index, built := Build("go", []File{{Path: "lib.go", Lines: []string{"func Load() {}"}}})
	require.True(t, built)

	named := index.Referenced("other_test.go", []string{"package lib", "func TestNothing() {}"})

	require.NotNil(t, named)
	assert.Empty(t, named)
}
