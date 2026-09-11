package symbol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// built indexes one file of lang whose lines are given one per argument.
func built(t *testing.T, lang, path string, lines ...string) *Index {
	t.Helper()
	index, ok := Build(lang, []File{{Path: path, Lines: lines}})
	require.True(t, ok)
	return index
}

// enclosed asks the index what encloses each line from 1 through last, and
// returns the answers in order, "" for a line nothing encloses.
func enclosed(index *Index, path string, last int) []string {
	names := make([]string, 0, last)
	for line := 1; line <= last; line++ {
		key, ok := index.Enclosing(path, line)
		if !ok {
			key = ""
		}
		names = append(names, key)
	}
	return names
}

// §3.4.4's symbol branch reads a Go file's spans: a function covers its
// declaration line through the brace that closes it, a line between two
// declarations is enclosed by neither, a one-line function covers its own
// line, and a struct type covers its fields.
//
// Two methods named String on two receivers are two symbols. §3.4.4 groups by
// the enclosing symbol, and an index that keyed on the spelling would gather
// edits to both into one unit named after neither.
func TestAGoDeclarationEnclosesTheLinesOfItsBody(t *testing.T) {
	index := built(t, "go", "a.go",
		"package a",                          //  1
		"",                                   //  2
		"type Money struct {",                //  3
		"\tcents int",                        //  4
		"}",                                  //  5
		"",                                   //  6
		"func (m Money) String() string {",   //  7
		"\treturn \"}\" // a } in a comment", //  8
		"}",                                  //  9
		"",                                   // 10
		"var rate = 3",                       // 11
		"",                                   // 12
		"func (r Rate) String() string { return \"\" }", // 13
		"",                                    // 14
		"func Total(v struct{ n int }) int {", // 15
		"\treturn v.n",                        // 16
		"}",                                   // 17
	)

	assert.Equal(t, []string{
		"", "", "Money@3", "Money@3", "Money@3", "",
		"String@7", "String@7", "String@7", "", "", "",
		"String@13", "", "Total@15", "Total@15", "Total@15",
	}, enclosed(index, "a.go", 17))
}

// A brace inside a block comment or a string counts for nothing, an escaped
// quote does not end its string, and a backslash inside a Go raw string does not
// escape the backtick that closes it — each of which, misread, would carry the
// span past the brace that really closes the body, or leave it unclosed.
func TestABraceInsideACommentOrAStringCountsForNothing(t *testing.T) {
	index := built(t, "go", "parse.go",
		"func Parse() string {",
		"\t/* a { that opens nothing",
		"\t   and a } that closes nothing */",
		"\ts := \"\\\"{\"",
		"\tr := `\\`",
		"\treturn s + r",
		"}",
		"func After() {}",
	)

	assert.Equal(t, []string{
		"Parse@1", "Parse@1", "Parse@1", "Parse@1", "Parse@1", "Parse@1", "Parse@1", "After@8",
	}, enclosed(index, "parse.go", 8))
}

// A PHP class holds its methods, and a line inside a method is enclosed by the
// method rather than the class: naming the class would put every edit to the
// file into one unit. The braces PSR-12 opens on the line after the signature
// are found there, an abstract method's `;` covers its own line, and a PHP 8
// attribute inside a signature is not read as a comment that hides the `)`.
func TestAPhpMethodIsEnclosedByItselfAndNotByItsClass(t *testing.T) {
	index := built(t, "php", "app/Money.php",
		"<?php",                       //  1
		"abstract class Money",        //  2
		"{",                           //  3
		"    private int $cents = 0;", //  4
		"    public function add(#[Sensitive] $n)", //  5
		"    {",                            //  6
		"        $this->cents += $n; // }", //  7
		"        return \"{$n}\";",         //  8
		"    }",                            //  9
		"    abstract public function rate(): int;", // 10
		"}", // 11
	)

	assert.Equal(t, []string{
		"", "Money@2", "Money@2", "Money@2",
		"add@5", "add@5", "add@5", "add@5", "add@5",
		"rate@10", "Money@2",
	}, enclosed(index, "app/Money.php", 11))
}

// A body the scan cannot bound gets no span, and every line of it falls
// through to adjacency per §3.4.3 rather than being named after a symbol that
// might not contain it: a function the file ends inside, and a Go declaration
// with no body whose search for a brace meets a blank line before the next
// declaration's. The file is still indexed, because §3.4.3 asks that of the
// file and the file was read.
func TestABodyTheScanCannotBoundEnclosesNothing(t *testing.T) {
	index := built(t, "go", "stub.go",
		"package a",           // 1
		"",                    // 2
		"func assembly() int", // 3
		"",                    // 4
		"func Open() {",       // 5
		"\tif true {",         // 6
	)

	assert.True(t, index.Indexed("stub.go"))
	assert.Equal(t, []string{"", "", "", "", "", ""}, enclosed(index, "stub.go", 6))
}

// §3.4.3 is asked per file: a file the index was built over is indexed even
// when it declares nothing, a file it was not built over is not, and a nil
// index — a head cr could not index — covers nothing and encloses nothing.
func TestTheIndexCoversTheFilesItWasBuiltOver(t *testing.T) {
	index := built(t, "go", "empty.go", "package a")

	assert.True(t, index.Indexed("empty.go"))
	assert.False(t, index.Indexed("other.go"))
	var none *Index
	assert.False(t, none.Indexed("empty.go"))
	_, found := none.Enclosing("empty.go", 1)
	assert.False(t, found)
}

// A long body is bounded however far its closing brace is, because the
// signatureLines bound limits only the search for the opening brace.
func TestABodyLongerThanTheSignatureBoundIsStillBounded(t *testing.T) {
	lines := []string{"package a", "", "func Long() {"}
	for range signatureLines * 2 {
		lines = append(lines, "\tstep()")
	}
	lines = append(lines, "}")
	index := built(t, "go", "long.go", lines...)

	key, found := index.Enclosing("long.go", len(lines))
	require.True(t, found)
	assert.Equal(t, "Long@3", key)
}
