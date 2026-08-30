package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// Invariant 2, which §2.2 states as normative text: all state lives under
// `~/.cr/`, cr MUST NOT write inside the repository under review, and it MUST
// NOT modify that repository's tracked files, index, HEAD, stash, or any
// branch. §5.1.1 carves out the single exception — the worktree registration
// under `.git/worktrees/` that `cr sandbox create` leaves behind.
//
// nowrite_test.go in this package guards a differently-named sentence: §2.1.2's
// prohibition on *network* writes and the confirmation gate §8.5 puts in front
// of its one exception. Nothing there reads a path, so nothing there covers
// this.
//
// # Where a write could come from
//
// cr reaches the repository under review two ways and no third. It can address
// a path itself, which needs one of the os functions that change something on
// disk; or it can drive git, behind which every asset §2.2 names — the index,
// HEAD, the stash, every branch — sits. internal/git is the only door to git,
// which TestCrReachesTheNetworkThroughOneRunnerAndNoOtherWay already holds
// shut, so the two static guards here fence the two routes: which files may
// call an os write, and which git subcommands may be run. The behavioural guard
// then drives the built binary against a real repository and reads the result
// off disk, which is the only one of the three that can see a write nobody
// predicted.

// filesystemWrites are the os functions that change something on disk, or hand
// back a handle that can.
//
// The constructors are listed with the direct writes because a handle is the
// same write one line later: *os.File's Write, WriteString, Truncate and Chmod
// are unreachable without one of Create, CreateTemp, OpenFile, NewFile or
// OpenRoot, and os.Open cannot produce one that writes. Fencing the five is
// therefore the whole of it, and it needs no type information — which matters,
// because a guard that had to resolve types would be a guard that stops working
// the first time a file fails to type-check.
var filesystemWrites = map[string]bool{
	// Direct writes.
	"WriteFile": true,
	"Mkdir":     true,
	"MkdirAll":  true,
	"MkdirTemp": true,
	"Remove":    true,
	"RemoveAll": true,
	"Rename":    true,
	"Link":      true,
	"Symlink":   true,
	"Truncate":  true,
	"Chmod":     true,
	"Chown":     true,
	"Lchown":    true,
	"Chtimes":   true,
	// Handles that can write.
	"Create":     true,
	"CreateTemp": true,
	"OpenFile":   true,
	"NewFile":    true,
	"OpenRoot":   true,
}

// statePackage is the one package allowed to call them. Its doc comment says
// why in a sentence: it derives every path cr writes, and every path it derives
// is resolved against one root, which
// TestEveryPathTheStateLayoutHandsOutIsUnderItsRoot checks is really so.
//
// Widening this list is how invariant 2 would be lost, and it is deliberately a
// thing a reviewer sees rather than something a call site does by existing.
var statePackage = filepath.Join("internal", "state") + string(filepath.Separator)

// eachSourceFile parses the Go files cr ships — its own source, tests excluded —
// and hands each one's repository-relative path and syntax tree to visit.
//
// Tests are out of the surface on purpose. They write temporary trees by design
// and are the wrong thing to police: what invariant 2 is about is the binary a
// colleague runs against their own checkout.
func eachSourceFile(t *testing.T, visit func(rel string, file *ast.File)) {
	t.Helper()
	root := moduleRoot(t)

	scanned := 0
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		require.NoError(t, err)

		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)

		scanned++
		visit(rel, parsed)
		return nil
	}))

	// A walk that found nothing would pass and would mean nothing.
	require.Greater(t, scanned, 10, "only %d files were scanned, so this guard proved nothing", scanned)
}

// osWrite reports the os function a call invokes and whether it writes.
//
// The package qualifier is matched as text, which is exactly as strong as the
// import guard beside it: `os` under an alias would read as some other package
// here and slip past, so the caller fails an aliased import outright rather
// than trusting a name.
func osWrite(call *ast.CallExpr) (string, bool) {
	fun, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := fun.X.(*ast.Ident)
	if !ok || pkg.Name != "os" {
		return "", false
	}
	return fun.Sel.Name, filesystemWrites[fun.Sel.Name]
}

// lowLevelPackages reach the filesystem underneath os, so importing one outside
// internal/state would put a write where the call scan cannot read it.
var lowLevelPackages = map[string]bool{
	"syscall":                  true,
	"golang.org/x/sys/unix":    true,
	"golang.org/x/sys/windows": true,
}

// Nothing outside internal/state writes to the filesystem at all.
//
// This is invariant 2's first half — cr never writes inside the repository
// under review — proved from the other end. Rather than trying to decide, at
// each call site, whether the path being written happens to sit inside the
// user's checkout, the guard fixes where a write may be spelled: one package,
// whose every path is rooted at `~/.cr`. A write into the repository under
// review then has nowhere to be written from.
//
// Two ways around a call scan are closed alongside it. An aliased import of os
// would make every write invisible here, and a package that reaches the
// filesystem beneath os would never mention os at all.
func TestOnlyTheStatePackageWritesToTheFilesystem(t *testing.T) {
	var found []string
	eachSourceFile(t, func(rel string, file *ast.File) {
		if strings.HasPrefix(rel, statePackage) {
			return
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			if path == "os" && spec.Name != nil {
				found = append(found, rel+" imports os as "+spec.Name.Name)
			}
			if lowLevelPackages[path] {
				found = append(found, rel+" imports "+path)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if name, writes := osWrite(call); writes {
				found = append(found, rel+" calls os."+name)
			}
			return true
		})
	})

	slices.Sort(found)
	assert.Empty(t, found,
		"invariant 2: internal/state derives every path cr writes, so no other package may write at all")
}

// Every path internal/state hands out lands under its own root.
//
// This is the other half of the sentence above. Confining the writes to one
// package is worth something only if that package cannot address anything but
// `~/.cr`, and §2.2's root is a field nothing outside internal/state can set:
// state.New and state.Default are the two constructors, and every path method
// joins onto it.
//
// The methods are enumerated by reflection rather than listed, so a path added
// later is checked by existing. The filter is the signature §2.2's table
// implies — strings and pull request numbers in, one path out — and a method
// that takes an argument this guard cannot supply fails rather than being
// passed over, because an unchecked path is the one this test exists to find.
//
// The boundary it does not claim: a caller who hands a path segment of their
// own — `l.PRFile(owner, repo, pr, "../../escape")` — walks out of the tree, and
// no reflection over the signature can see that. What is fenced is the root,
// which is where a write into the repository under review would have to come
// from.
func TestEveryPathTheStateLayoutHandsOutIsUnderItsRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state-root")
	layout := reflect.ValueOf(state.New(root))
	surface := layout.Type()

	checked := 0
	for i := range surface.NumMethod() {
		method := surface.Method(i)
		signature := method.Type
		if signature.NumOut() != 1 || signature.Out(0).Kind() != reflect.String {
			continue
		}

		// The receiver first, then one argument per parameter. The values
		// are arbitrary: what is asserted is where the path lands, and
		// every §2.2 path is a join onto the root whatever the segments say.
		args := make([]reflect.Value, 0, signature.NumIn())
		args = append(args, layout)
		for at := 1; at < signature.NumIn(); at++ {
			switch signature.In(at).Kind() {
			case reflect.String:
				args = append(args, reflect.ValueOf("segment"))
			case reflect.Int:
				args = append(args, reflect.ValueOf(7))
			default:
				require.Fail(t, "unsupported parameter",
					"%s takes a %s, which this guard cannot supply, so its path went unchecked",
					method.Name, signature.In(at))
			}
		}

		produced := method.Func.Call(args)[0].String()
		inside, err := filepath.Rel(root, produced)
		require.NoError(t, err)
		assert.False(t, inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)),
			"§2.2: %s resolves to %s, which is outside the state root", method.Name, produced)
		checked++
	}

	// The floor is the size of §2.2's table, so a filter that quietly
	// stopped matching fails here rather than passing over an empty set.
	require.Greater(t, checked, 20, "only %d paths were checked, so this guard proved nothing", checked)
}

// gitReads are the git subcommands internal/git is allowed to run, each with
// the section that asks for it.
//
// This is the second route into the repository under review, and the one §2.2's
// second sentence is about: the index, HEAD, the stash and every branch are
// files under `.git/`, and cr does not address them — git does, on cr's behalf.
// So the guard is over the verb rather than over the path, and it is an
// allowlist of what cr runs today rather than a denylist of what git can
// destroy, because the denylist is the one that goes stale in the unsafe
// direction.
//
// §5.1.1's exception is not here. `cr sandbox create` will run `git worktree
// add`, which is a write, and the sandbox-worktree task adds `worktree` to this
// map with §5.1.1 as its reason. Widening the map is then a deliberate act with
// a reviewer, which is the point — and the behavioural guard below independently
// fixes how far that write may reach, so the map growing by one word cannot
// quietly grant more than the one path §2.2 names.
var gitReads = map[string]string{
	"diff":       "§3.4.1's diff of the head against the merge base",
	"merge-base": "§3.4.1's merge base",
}

// gitSourceFiles parses internal/git's own source, tests excluded.
func gitSourceFiles(t *testing.T) []*ast.File {
	t.Helper()
	dir := filepath.Join(moduleRoot(t), "internal", "git")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "internal/git is cr's only door to git; if it moved, move this guard with it")

	parsed := make([]*ast.File, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		parsed = append(parsed, file)
	}
	require.NotEmpty(t, parsed, "no source was read, so this guard proved nothing")
	return parsed
}

// packageValues collects the package-level variables of internal/git by name.
//
// They are collected across the whole package rather than per file, because
// the argument list a call spreads and the runner it is spread into need not
// live in the same file — diffArgs and run do not.
func packageValues(files []*ast.File) map[string]ast.Expr {
	values := make(map[string]ast.Expr)
	for _, file := range files {
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, spec := range general.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for at, name := range value.Names {
					if at < len(value.Values) {
						values[name.Name] = value.Values[at]
					}
				}
			}
		}
	}
	return values
}

// resolutionDepth bounds the walk back from a call site to a literal. Nothing
// in internal/git needs more than three steps, and the bound is what keeps a
// cycle from hanging the suite rather than failing it.
const resolutionDepth = 8

// firstElement reports the first string an expression's argument list begins
// with: the literal itself, a slice's first element, the variable it names, or
// the first element of what slices.Clone and append were given.
//
// Clone and append are followed because both keep their first argument's first
// element, which is where the subcommand sits. Anything else returns false, and
// the caller turns that into a failure rather than into silence.
func firstElement(values map[string]ast.Expr, within *ast.FuncDecl, expr ast.Expr, depth int) (string, bool) {
	if depth > resolutionDepth {
		return "", false
	}
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind != token.STRING {
			return "", false
		}
		name, err := strconv.Unquote(node.Value)
		return name, err == nil
	case *ast.CompositeLit:
		if len(node.Elts) == 0 {
			return "", false
		}
		return firstElement(values, within, node.Elts[0], depth+1)
	case *ast.Ident:
		return namedArgv(values, within, node.Name, depth+1)
	case *ast.CallExpr:
		if len(node.Args) == 0 || !keepsItsFirstElement(node.Fun) {
			return "", false
		}
		return firstElement(values, within, node.Args[0], depth+1)
	}
	return "", false
}

// keepsItsFirstElement reports whether a call hands back a slice beginning with
// its own first argument's first element.
func keepsItsFirstElement(fun ast.Expr) bool {
	if builtin, ok := fun.(*ast.Ident); ok {
		return builtin.Name == "append"
	}
	selector, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "slices" && selector.Sel.Name == "Clone"
}

// namedArgv resolves a variable back to the argument list it holds: a
// package-level one by name, or a local one through the first assignment made
// to it in the function the call sits in.
//
// The first assignment is the one read because it is the one that establishes
// what the list begins with; every later append adds to the tail, which is
// arguments rather than the subcommand.
func namedArgv(values map[string]ast.Expr, within *ast.FuncDecl, name string, depth int) (string, bool) {
	if origin, ok := values[name]; ok {
		return firstElement(values, within, origin, depth+1)
	}
	if within == nil || within.Body == nil {
		return "", false
	}
	var origin ast.Expr
	ast.Inspect(within.Body, func(n ast.Node) bool {
		if origin != nil {
			return false
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for at, target := range assign.Lhs {
			ident, isIdent := target.(*ast.Ident)
			if isIdent && ident.Name == name && at < len(assign.Rhs) {
				origin = assign.Rhs[at]
				return false
			}
		}
		return true
	})
	if origin == nil {
		return "", false
	}
	return firstElement(values, within, origin, depth+1)
}

// unreadableSubcommand is what a call reports when the guard cannot say which
// subcommand it runs. It is never in gitReads, so the unanswerable case fails
// rather than passing: a reader who cannot tell what a git invocation does is
// exactly the reader §2.2 is written for.
const unreadableSubcommand = "a subcommand named at run time"

// gitSubcommand reports the git subcommand a call runs, and whether the call
// runs one at all.
func gitSubcommand(values map[string]ast.Expr, within *ast.FuncDecl, call *ast.CallExpr) (string, bool) {
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "run" {
		return "", false
	}
	// The directory first, then the argument list, whose head is the
	// subcommand.
	if len(call.Args) < 2 {
		return unreadableSubcommand, true
	}
	if name, resolved := firstElement(values, within, call.Args[1], 0); resolved {
		return name, true
	}
	return unreadableSubcommand, true
}

// Every git subcommand cr runs is a read.
//
// §2.2's second sentence names five things cr must not modify — tracked files,
// the index, HEAD, the stash, and any branch — and all five are reached by
// running git, never by cr writing a path. Fixing the verbs is therefore how
// that sentence is held: `git diff` and `git merge-base` cannot move any of the
// five, whatever arguments they are given, and no other subcommand is reachable
// from internal/git at all.
//
// It is a source guard rather than a run, so it covers what internal/git can do
// rather than what today's commands happen to ask of it. The behavioural guard
// below is the complement, and neither is worth much without the other: this
// one sees a write added to internal/git before any command wires it, and that
// one sees a write arriving by a route nobody thought to fence.
func TestEveryGitSubcommandCrRunsIsARead(t *testing.T) {
	files := gitSourceFiles(t)
	values := packageValues(files)

	invocations := 0
	var found []string
	for _, file := range files {
		for _, decl := range file.Decls {
			within, _ := decl.(*ast.FuncDecl)
			ast.Inspect(decl, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name, runs := gitSubcommand(values, within, call)
				if !runs {
					return true
				}
				invocations++
				if _, read := gitReads[name]; !read {
					found = append(found, "internal/git runs `git "+name+"`")
				}
				return true
			})
		}
	}

	require.Greater(t, invocations, 1, "only %d git invocations were read, so this guard proved nothing", invocations)
	slices.Sort(found)
	assert.Empty(t, slices.Compact(found),
		"§2.2: cr must not modify the repository's tracked files, index, HEAD, stash, or any branch")
}
