package symbol

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fileOf splits a literal source body into the File shape the index reads, so a
// test writes the source the way the source is written.
func fileOf(path, body string) File {
	return File{Path: path, Lines: strings.Split(strings.TrimSuffix(body, "\n"), "\n")}
}

// §4.3.1 builds the index "from the profile's symbols.lang", so a language cr
// cannot read has to be answerable before anything is built. The answer is a
// reported false and not an empty index: §4.3.1 sends this case to §4.5.4, and
// an empty index would say the head declares nothing — an assertion cr never
// made about a file it never read.
func TestALanguageCrCannotReadBuildsNoIndexAtAll(t *testing.T) {
	index, built := Build("cobol", []File{fileOf("main.cob", "PROCEDURE DIVISION.")})

	assert.False(t, built, "cr has no cobol scanner, so it built no cobol index")
	assert.Nil(t, index, "§4.3.1: no index is not the same answer as an index holding nothing")
	assert.False(t, Supported("cobol"))
	assert.False(t, Supported(""), "a profile declaring no symbols.lang names no language")

	for _, lang := range []string{"go", "php"} {
		assert.True(t, Supported(lang), "%s ships a scanner", lang)
	}
}

// A head that declares nothing still produces an index, and it serialises as
// `[]` rather than as `null`, per §12.
func TestAHeadThatDeclaresNothingIsAnEmptyIndexAndNotANilOne(t *testing.T) {
	index, built := Build("go", []File{fileOf("doc.go", "// Package app does things.\npackage app")})

	require.True(t, built)
	require.NotNil(t, index)
	assert.Empty(t, index.Decls)
	assert.Equal(t, "go", index.Lang)

	encoded, err := json.Marshal(index)
	require.NoError(t, err)
	assert.JSONEq(t, `{"lang":"go","decls":[]}`, string(encoded))
}

// §4.3.1's three kinds, in Go, with the declared parameter count §4.3.2 will
// rank on. The generic spellings are here because they are the ones a pattern
// anchored at the name gets wrong: the type parameter list sits between the
// name and the signature.
func TestGoDeclarationsAreIndexedWithTheirParameterCounts(t *testing.T) {
	index, built := Build("go", []File{fileOf("app/repo.go", `package app

type Repo struct {
	db *DB
}

type Store[T any] interface {
	Put(T) error
}

func New() *Repo {
	return &Repo{}
}

func (r *Repo) Find(id int) (*Row, error) {
	return nil, nil
}

func (s *Set[T]) Add(v T) {}

func Map[T any](in []T, f func(T) T) []T {
	return in
}
`)})

	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "app/repo.go", Line: 3, Name: "Repo", Kind: Class},
		{Path: "app/repo.go", Line: 7, Name: "Store", Kind: Class},
		{Path: "app/repo.go", Line: 11, Name: "New", Kind: Function},
		{Path: "app/repo.go", Line: 15, Name: "Find", Kind: Method, Params: 1},
		{Path: "app/repo.go", Line: 19, Name: "Add", Kind: Method, Params: 1},
		{Path: "app/repo.go", Line: 21, Name: "Map", Kind: Function, Params: 2},
	}, index.Decls)
}

// PHP is the language the shipped laravel-pest profile declares, and it is the
// one §4.3.2's class rule is written for: a class's declared parameter count is
// its constructor's, or zero when it declares none.
func TestPhpDeclarationsAreIndexedWithTheirParameterCounts(t *testing.T) {
	index, built := Build("php", []File{fileOf("app/Orders/Refund.php", `<?php

namespace App\Orders;

final class Refund extends Action
{
    public function __construct(private Gateway $gateway, private Clock $clock)
    {
    }

    public function handle(Order $order): void
    {
    }

    private static function rate(): float
    {
    }
}
`)})

	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "app/Orders/Refund.php", Line: 5, Name: "Refund", Kind: Class, Params: 2},
		{Path: "app/Orders/Refund.php", Line: 7, Name: "__construct", Kind: Method, Params: 2},
		{Path: "app/Orders/Refund.php", Line: 11, Name: "handle", Kind: Method, Params: 1},
		{Path: "app/Orders/Refund.php", Line: 15, Name: "rate", Kind: Method},
	}, index.Decls,
		"§4.3.2: a class's declared parameter count is its constructor's, and zero when it declares none")
}

// A class declaring no constructor keeps the zero §4.3.2 gives it, and a free
// function above every class stays a function. Both are the classScoped
// heuristic read at its two edges.
func TestAPhpClassWithNoConstructorDeclaresNoParameters(t *testing.T) {
	index, built := Build("php", []File{fileOf("app/Support/helpers.php", `<?php

function money(int $cents): string
{
}

interface Clock
{
    public function now(): DateTimeImmutable;
}

trait Loggable
{
}

enum Status: string
{
}
`)})

	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "app/Support/helpers.php", Line: 3, Name: "money", Kind: Function, Params: 1},
		{Path: "app/Support/helpers.php", Line: 7, Name: "Clock", Kind: Class},
		{Path: "app/Support/helpers.php", Line: 9, Name: "now", Kind: Method},
		{Path: "app/Support/helpers.php", Line: 12, Name: "Loggable", Kind: Class},
		{Path: "app/Support/helpers.php", Line: 16, Name: "Status", Kind: Class},
	}, index.Decls,
		"PHP spells four constructs the one way §4.3.1 calls a class")
}

// A declaration written inside a comment is not a declaration. It is the
// cheapest false hit a line scanner can make and the one a PHP docblock makes
// on almost every well-documented method.
func TestADeclarationInsideACommentIsNotIndexed(t *testing.T) {
	index, built := Build("php", []File{fileOf("app/Orders/Order.php", `<?php

/**
 * class Ghost
 * public function ghost(int $a)
 */
class Order
{
    // public function alsoGhost()
    # public function stillGhost()
    public function total(): int
    {
    }
}
`)})

	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "app/Orders/Order.php", Line: 7, Name: "Order", Kind: Class},
		{Path: "app/Orders/Order.php", Line: 11, Name: "total", Kind: Method},
	}, index.Decls)
}

// The parameter count is separators at the list's own depth, so every comma
// that is not one has to be ignored: inside a nested type, inside a nested
// call, and inside a string. §4.3.2 requires the count to be equal to the added
// symbol's, so a comma miscounted here silently drops a candidate that
// qualified.
func TestTheParameterCountIgnoresCommasThatAreNotSeparators(t *testing.T) {
	for name, source := range map[string]struct {
		lang, body string
		params     int
	}{
		"no parameters at all":       {"go", "func A() {}", 0},
		"whitespace is no parameter": {"go", "func A(  ) {}", 0},
		"one parameter":              {"go", "func A(x int) {}", 1},
		"a comma inside a map type":  {"go", "func A(m map[string]int, y int) {}", 2},
		"a comma inside a func type": {"go", "func A(f func(a, b int) error) {}", 1},
		"a comma inside a slice literal default": {
			"php", "function a($x = [1, 2], $y = 3) {}", 2},
		"a comma inside a single-quoted default": {
			"php", "function a($sep = ',', $y = 3) {}", 2},
		"an escaped quote inside a default": {
			"php", `function a($sep = 'it\'s, fine') {}`, 1},
		"a signature spread over lines": {"go", `func A(
	first int,
	second string,
) error {`, 2},
	} {
		t.Run(name, func(t *testing.T) {
			index, built := Build(source.lang, []File{fileOf("x", source.body)})

			require.True(t, built)
			require.Len(t, index.Decls, 1)
			assert.Equal(t, source.params, index.Decls[0].Params)
		})
	}
}

// An unterminated parameter list must not walk the rest of the file. The bound
// is what turns a misread signature into a wrong count for one symbol instead
// of a scan of everything below it.
func TestAnUnbalancedSignatureStopsAtTheBound(t *testing.T) {
	body := "func A(x int" + strings.Repeat("\n", signatureLines+20) + "y, z)"

	index, built := Build("go", []File{fileOf("x.go", body)})

	require.True(t, built)
	require.Len(t, index.Decls, 1)
	assert.Equal(t, 1, index.Decls[0].Params,
		"the scan stopped at the bound, so it never reached the commas past it")
}

// The other way an unterminated list ends is the file ending first: a file cut
// off mid-edit, or a `(` misread in its last lines. The scan stops at the last
// line there is, and the parameter still open counts, as params says. The case
// above runs out of bound long before it runs out of file, so it never asks
// where the file's own end is.
func TestASignatureTheFileEndsInsideStopsAtTheLastLine(t *testing.T) {
	index, built := Build("go", []File{fileOf("x.go", "func A(x int,\n\ty string")})

	require.True(t, built)
	require.Len(t, index.Decls, 1)
	assert.Equal(t, 2, index.Decls[0].Params)
}

// signatureLines counts the declaration's own line: a signature spanning that
// many lines is read whole, and one spanning a line more is "more lines than
// this" and is cut. TestAnUnbalancedSignatureStopsAtTheBound puts its commas
// twenty lines past the bound, where a bound one line out cuts them just the
// same.
func TestTheBoundCountsTheDeclarationsOwnLine(t *testing.T) {
	for name, c := range map[string]struct {
		body   string
		params int
	}{
		"closing on the last line the bound reads": {
			"func A(x int," + strings.Repeat("\n", signatureLines-1) + "y int)", 2},
		"closing one line past it": {
			"func A(x int," + strings.Repeat("\n", signatureLines) + "y, z int)", 1},
	} {
		t.Run(name, func(t *testing.T) {
			index, built := Build("go", []File{fileOf("x.go", c.body)})

			require.True(t, built)
			require.Len(t, index.Decls, 1)
			assert.Equal(t, c.params, index.Decls[0].Params)
		})
	}
}

// §4.3.2 orders candidates by similarity, then path ascending, then line
// ascending. Ordering the index itself is what makes the tail of that ordering
// free — and what makes two runs over one head agree, whatever order git
// happened to list the tree in.
func TestTheIndexIsOrderedByPathThenLine(t *testing.T) {
	index, built := Build("go", []File{
		fileOf("z/last.go", "func Zed() {}"),
		fileOf("a/first.go", "func Beta() {}\nfunc Alpha() {}"),
	})

	require.True(t, built)
	assert.Equal(t, []Decl{
		{Path: "a/first.go", Line: 1, Name: "Beta", Kind: Function},
		{Path: "a/first.go", Line: 2, Name: "Alpha", Kind: Function},
		{Path: "z/last.go", Line: 1, Name: "Zed", Kind: Function},
	}, index.Decls)
}

// A non-ASCII name is a name. `\w` in Go's regexp is ASCII, so an identifier
// pattern written with it indexes `deger` and skips `değer` — and a symbol
// missing from the index is a §4.3.1 candidate never offered, with nothing
// anywhere saying it was skipped.
func TestANonASCIINameIsIndexed(t *testing.T) {
	index, built := Build("go", []File{fileOf("x.go", "func değerHesapla(a int) {}")})

	require.True(t, built)
	require.Len(t, index.Decls, 1)
	assert.Equal(t, "değerHesapla", index.Decls[0].Name)
	assert.Equal(t, 1, index.Decls[0].Params)
}

// A word that begins with a keyword is not that keyword. `type Foostruct {` is
// the shape a pattern that lets the name and the keyword run together reports
// as a class named Foo.
func TestAWordBeginningWithAKeywordIsNotADeclaration(t *testing.T) {
	index, built := Build("go", []File{fileOf("x.go", "type Foostruct {\nvar functional = 1")})

	require.True(t, built)
	assert.Empty(t, index.Decls)
}
