package text

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fencedDigests are the import paths through which a second hashing site could
// be built. The two halves of §1.4's normalised hash are a digest and a
// lowercase hex rendering, and everything the standard library offers for
// either is listed: the crypto digests, the whole `hash` tree down to the
// non-cryptographic ones, and `encoding/hex`.
//
// The digests carry the fence and `encoding/hex` only reinforces it, because
// hex is spellable without the package — fmt's %x does it in one verb. That is
// the same shape as internal/role's layer fence: what is fenced is the
// ingredient a caller could not otherwise obtain, and here that is the digest.
// A package that cannot reach a digest cannot compute a hash to disagree with
// this one, whatever it can format.
//
// Adding a path here is harmless. Removing one, or granting an exclusion, is
// how the fence would be lost quietly, which is why the floors below insist the
// fence still has something real behind it.
var fencedDigests = []string{
	"crypto/md5",
	"crypto/sha1",
	"crypto/sha256",
	"crypto/sha512",
	"encoding/hex",
	"hash",
	"hash/adler32",
	"hash/crc32",
	"hash/crc64",
	"hash/fnv",
	"hash/maphash",
}

// hashFenceSelfPath is this file's own path on disk, which is how the guard
// finds both the module root and the directory of the package it fences.
// runtime.Caller answers rather than a hard-coded name, so moving the file
// cannot silently take the exclusion with it.
func hashFenceSelfPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "the compiler kept no path for this file, so the guard cannot place itself")
	return file
}

// hashFenceModuleRoot is the directory holding go.mod, walked up to from this
// file rather than from the working directory, which is the package directory
// under `go test` and something else entirely under a test binary run by hand.
func hashFenceModuleRoot(t *testing.T) string {
	t.Helper()
	dir := filepath.Dir(hashFenceSelfPath(t))
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above %s", hashFenceSelfPath(t))
		dir = parent
	}
}

// digestsImported returns which of fencedDigests a file imports. The import
// path is read rather than the package name, so an aliased import — the obvious
// way around a guard that matched identifiers — is caught by the same walk.
func digestsImported(file *ast.File) []string {
	var found []string
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if slices.Contains(fencedDigests, path) {
			found = append(found, path)
		}
	}
	return found
}

// §1.4 defines one normalised hash, and five sections spend it: §3.4.6's unit
// hash, §7.4.1's waiver key, §8.3.3's payload hash, §9.2's anchor content hash,
// and the span and issue hashes §6.1 marks computed. §2.6.3.1's grouping by
// normalised body and §3.3's drift check read the same values back.
//
// The acceptance criterion this guard belongs to says NormalisedHash is the
// sole hashing entry point so those sections cannot drift apart, and absence is
// not what that asks for. Each of the five would test its own hash and each
// would pass: a site truncating to twelve characters, or hashing before
// normalising, is stable, deterministic and self-consistent, and nothing local
// to it can notice. The disagreement is only visible from above, which is
// where this sits.
//
// So the fence is on the ingredient rather than on the act. Outside
// internal/text no package can reach a digest at all, and calling
// NormalisedHash is what is left.
//
// # What it cannot catch
//
// Tests are out of the surface, for the reason the other source guards leave
// them out: what is fenced is the binary a colleague runs. A digest computed by
// an external command, or by a module outside the standard library, is likewise
// out of reach — the go.mod is the place that would show up.
//
// # Why the two floors
//
// None of the six callers exists yet, so a walk finding no violation today
// finds none whatever fencedDigests says, and on its own would mean nothing.
// The value is prospective: it fires on the first caller that reinvents the
// hash. Two things are therefore asserted rather than assumed — that the scan
// really read the module, and that internal/text still imports the digest the
// fence exists to keep in one place. A hash moved out from under this list
// fails here instead of quietly emptying the fence.
func TestNothingOutsideThisPackageCanReachADigest(t *testing.T) {
	pinned, err := NormalisedHash("a fence is worth something only beside the door it leaves open")
	require.NoError(t, err)
	require.Len(t, pinned, hashLength, "the door NormalisedHash leaves open is the point of the fence")

	root := hashFenceModuleRoot(t)
	thisPackage, err := filepath.Rel(root, filepath.Dir(hashFenceSelfPath(t)))
	require.NoError(t, err)

	scanned := 0
	var inside, found []string
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

		imported := digestsImported(parsed)
		if filepath.Dir(rel) == thisPackage {
			inside = append(inside, imported...)
			return nil
		}
		for _, name := range imported {
			found = append(found, rel+" imports "+name)
		}
		return nil
	}))

	require.Greater(t, scanned, 10, "only %d files were scanned, so this guard proved nothing", scanned)
	require.Contains(t, inside, "crypto/sha256",
		"%s no longer takes the digest §1.4 names, so fencing it fences nothing", thisPackage)

	slices.Sort(found)
	assert.Empty(t, found,
		"§1.4 defines one normalised hash, so the digest stays inside internal/text and every caller calls NormalisedHash")
}
