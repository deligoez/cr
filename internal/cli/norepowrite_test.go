package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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
