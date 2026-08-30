package role

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// layerNames are every identifier through which the layer a role was resolved
// from can be named at all: §2.5.4's type, its three constants, and — because
// the field is spelled the same — Resolved's own Layer.
//
// Adding a name here is harmless. Removing one is how the fence below would be
// lost, so the guard requires each of them to still be spelled this way inside
// internal/role rather than trusting the list to have kept up.
var layerNames = []string{"Layer", "RepoLayer", "GlobalLayer", "BuiltinLayer"}

// fenceSelfPath is this file's own path on disk, which is how the guard finds
// both the module root and the directory of the package it fences. runtime.Caller
// answers rather than a hard-coded name, so moving the file cannot silently take
// the exclusion with it.
func fenceSelfPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "the compiler kept no path for this file, so the guard cannot place itself")
	return file
}

// moduleRoot is the directory holding go.mod, walked up to from this file
// rather than from the working directory, which is the package directory under
// `go test` and something else entirely under a test binary run by hand.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir := filepath.Dir(fenceSelfPath(t))
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above %s", fenceSelfPath(t))
		dir = parent
	}
}

// namesUsed reports how often each of layerNames is named anywhere in a file.
//
// An identifier is enough, and no type information is needed: `role.RepoLayer`
// and `resolved.Layer` both reach the guard as the Ident on the right of a
// selector, and a guard that had to resolve types would be a guard that stops
// working the first time a file fails to type-check.
func namesUsed(file *ast.File) map[string]int {
	used := make(map[string]int, len(layerNames))
	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if ok && slices.Contains(layerNames, ident.Name) {
			used[ident.Name]++
		}
		return true
	})
	return used
}

// Corpus order is internal/role's to hand out, and Order is the door.
//
// §6.4.2 picks a duplicate group's representative by "the earliest role in
// corpus order per §2.5.5", and the acceptance this guard belongs to asks that
// the ordering be exposed as one function so §6.4.2 cannot reimplement it.
// Exposing it is half; this is the other half, and it fences the ingredient
// rather than the act. §2.5.5's order is resolution layer first, and the layer
// is the one part of it a caller could not otherwise obtain — ascending role id
// is a thing anyone can spell, but ordering by id is not §2.5.5's order and
// never was. So outside internal/role the layer is not nameable, no other
// package can order by it, and calling Order is what is left.
//
// The limit it does not claim: a package handed the corpus can still shuffle
// the slice into some other order. What it cannot do is put §2.5.5's order back
// afterwards, which is the whole of why Order is worth calling.
//
// Tests are out of the surface, for the reason the other source guards leave
// them out: what is fenced is the binary a colleague runs.
//
// # Why the two floors
//
// Nothing outside internal/role imports it yet — §4.5.1's activation and
// §6.4.2's dedup are later tasks — so a walk finding no violation today would
// find none whatever the names in layerNames said, and would mean nothing. Two
// things are therefore asserted rather than assumed: that the scan really read
// the module, and that every fenced name is still spelled that way inside
// internal/role. A constant renamed out from under this list fails here instead
// of quietly emptying the fence.
func TestNothingOutsideThisPackageCanNameTheLayerARoleResolvedFrom(t *testing.T) {
	require.NotNil(t, Order(nil), "a fence is worth something only beside the door it leaves open")

	root := moduleRoot(t)
	thisPackage, err := filepath.Rel(root, filepath.Dir(fenceSelfPath(t)))
	require.NoError(t, err)

	scanned := 0
	inside := make(map[string]int, len(layerNames))
	var found []string
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
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		scanned++

		used := namesUsed(parsed)
		if filepath.Dir(rel) == thisPackage {
			for name, count := range used {
				inside[name] += count
			}
			return nil
		}
		for name := range used {
			found = append(found, rel+" names "+name)
		}
		return nil
	}))

	require.Greater(t, scanned, 10, "only %d files were scanned, so this guard proved nothing", scanned)
	for _, name := range layerNames {
		require.NotZero(t, inside[name],
			"%s is not named in %s any more, so fencing it fences nothing", name, thisPackage)
	}

	slices.Sort(found)
	assert.Empty(t, found,
		"§2.5.5's corpus order is layer first, so the layer stays inside internal/role and §6.4.2 calls Order")
}
