package render

import (
	"go/ast"
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

// namesOf renders a set of bodies as the names they go by, so the whole set can
// be asserted at once and in order.
func namesOf(set []Body) []string {
	names := make([]string, 0, len(set))
	for _, body := range set {
		names = append(names, body.name)
	}
	return names
}

// Round 12's finding unspecified-render-language asked which language §8.4.3's
// review body is written in. The user's 2026-09-13 decision answered English,
// and v0.14.0's §8.4.3 reverses it: the framing and the axis names are built in
// per render.lang. So the set of bodies Setting's language governs is the
// comment body of §8.1.1 and the review body of §8.4.3.
func TestTheCommentAndReviewBodiesAreWrittenInTheConfiguredLanguage(t *testing.T) {
	assert.Equal(t, []string{"comment", "review"}, namesOf(bodies),
		"§8.1.1's comment body and §8.4.3's review body are the bodies render.lang governs")

	for _, body := range bodies {
		assert.Truef(t, body.AuthorFacing(), "%s is written in the configured language", body.name)
	}
	assert.False(t, Body{}.AuthorFacing(), "a body nothing named is no body")
}

// withoutBody runs within with body taken out of the set, and puts the set back
// afterwards.
func withoutBody(body Body, within func()) {
	kept := bodies
	defer func() { bodies = kept }()
	bodies = make([]Body, 0, len(kept))
	for _, member := range kept {
		if member != body {
			bodies = append(bodies, member)
		}
	}
	within()
}

// The set is what the renderer reads, not a sentence beside it: a body taken
// out of the set is rendered in no language at all, and the same call renders
// once the body is back.
//
// The comment body is driven through §8.1.4's label region, which
// internal/draft places in every question's comment, and the refusal is the one
// it already gives for a language it has no built-in text for. The review body
// taken out keeps its entries and the payload hash and loses every framing
// line; each body taken out leaves the other as it was.
func TestABodyOutsideTheSetIsRenderedInNoLanguage(t *testing.T) {
	const hash = "420012ebffc2b992"
	before := ReviewBody(LangEN, nil, nil, hash)

	withoutBody(BodyComment, func() {
		_, err := QuestionLabelRegion(LangEN, finding.GradeArgued)
		assert.Equal(t, &NoLabelError{Lang: LangEN, Grade: finding.GradeArgued}, err,
			"the comment body's label is looked up through the set")
		assert.Equal(t, before, ReviewBody(LangEN, nil, nil, hash),
			"the review body is a body of its own, so the comment body's absence does not reach it")
	})
	withoutBody(BodyReview, func() {
		assert.Equal(t, "\n\n\n\n"+regionSeparator+PayloadHashComment(hash), ReviewBody(LangEN, nil, nil, hash),
			"the review body's framing is looked up through the set")
		_, err := QuestionLabelRegion(LangEN, finding.GradeArgued)
		assert.NoError(t, err, "and the comment body's label still is")
	})

	_, err := QuestionLabelRegion(LangEN, finding.GradeArgued)
	require.NoError(t, err)
	assert.Equal(t, before, ReviewBody(LangEN, nil, nil, hash))
}

// productionFiles parses every non-test Go file of this package.
func productionFiles(t *testing.T) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)
	files := make([]*ast.File, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(".", name), nil, 0)
		require.NoError(t, err)
		files = append(files, parsed)
	}
	require.NotEmpty(t, files, "a scan of no files proves nothing")
	return files
}

// packageVars yields every package-level variable of files with the type and
// the single value it is declared with, either of which may be nil.
func packageVars(files []*ast.File, visit func(name string, typ, value ast.Expr)) {
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, ident := range vs.Names {
					var value ast.Expr
					if i < len(vs.Values) {
						value = vs.Values[i]
					}
					visit(ident.Name, vs.Type, value)
				}
			}
		}
	}
}

// languageKeyedTypes names every struct type of files carrying a field of type
// Lang: the row type of a table of built-in text, one row per language.
func languageKeyedTypes(files []*ast.File) map[string]bool {
	keyed := make(map[string]bool)
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			fields, ok := spec.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range fields.Fields.List {
				if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "Lang" {
					keyed[spec.Name.Name] = true
				}
			}
			return true
		})
	}
	return keyed
}

// elementOf is the element type name of a slice type expression, or "".
func elementOf(expr ast.Expr) string {
	slice, ok := expr.(*ast.ArrayType)
	if !ok {
		return ""
	}
	if ident, ok := slice.Elt.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// A built-in text keyed by language is text of some author-facing body, and
// Setting's language may only choose among its rows for a body the set names.
// So every function that reads such a table asks exactly one named Body whether
// it is AuthorFacing, and the bodies asked across the package are the set: none
// is rendered outside it, and none in it goes unrendered.
//
// This is what makes the set the renderer's rather than a declaration beside
// it. A third author-facing body arrives as a new table of text with a lang
// field; the function reading it fails here until it asks a body, and the body
// it asks fails here until it joins the set.
func TestEveryLanguageKeyedTextIsReadThroughTheBodySet(t *testing.T) {
	files := productionFiles(t)
	keyed := languageKeyedTypes(files)

	tables := make(map[string]bool)
	named := make(map[string]string)
	packageVars(files, func(name string, typ, value ast.Expr) {
		literal, _ := value.(*ast.CompositeLit)
		if typ == nil && literal != nil {
			typ = literal.Type
		}
		if keyed[elementOf(typ)] {
			tables[name] = true
		}
		if ident, ok := typ.(*ast.Ident); ok && ident.Name == "Body" && literal != nil && len(literal.Elts) == 1 {
			if lit, ok := literal.Elts[0].(*ast.BasicLit); ok {
				unquoted, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				named[name] = unquoted
			}
		}
	})
	require.NotEmpty(t, tables, "a scan that finds no language-keyed table proves nothing")
	require.NotEmpty(t, named, "a scan that finds no named body proves nothing")

	asked := make([]string, 0, len(tables))
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			reads, bodiesAsked := false, make([]string, 0, 1)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.Ident:
					if tables[node.Name] {
						reads = true
					}
				case *ast.CallExpr:
					selector, ok := node.Fun.(*ast.SelectorExpr)
					if !ok || selector.Sel.Name != "AuthorFacing" {
						return true
					}
					if receiver, ok := selector.X.(*ast.Ident); ok {
						if name, known := named[receiver.Name]; known {
							bodiesAsked = append(bodiesAsked, name)
						}
					}
				}
				return true
			})
			if !reads {
				continue
			}
			require.Lenf(t, bodiesAsked, 1,
				"%s reads a language-keyed table, so it asks exactly one named body whether Setting's language governs it",
				fn.Name.Name)
			asked = append(asked, bodiesAsked[0])
		}
	}

	assert.ElementsMatch(t, namesOf(bodies), uniqueOf(asked),
		"the bodies the renderer asks are exactly the set: none rendered outside it, none in it unrendered")
}

// uniqueOf is names with each one kept once, in first-seen order.
func uniqueOf(names []string) []string {
	seen := make(map[string]bool, len(names))
	unique := make([]string, 0, len(names))
	for _, name := range names {
		if !seen[name] {
			seen[name] = true
			unique = append(unique, name)
		}
	}
	return unique
}
