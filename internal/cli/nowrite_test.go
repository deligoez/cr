package cli

import (
	"go/ast"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
)

// Invariant 3 and §2.1.2, asserted from outside internal/gh, which is the only
// place the assertion means anything: a boundary tested only by the package
// that declares it proves that the package agrees with itself.
//
// This is the shape a caller that has not been through §8.5's gate is in. It
// can reach the read door, which refuses a write. It can name the type, and
// the only value of it available here is the zero value, because granted is
// unexported and Confirm is the sole constructor — so the write door refuses
// it too. There is no third door: gh.Run and Confirmation.Write are the whole
// exported surface that starts the binary.
func TestAWriteFromOutsideTheGateIsRefused(t *testing.T) {
	review := []string{"api", "repos/cli/cli/pulls/11451/reviews", "--method", "POST", "--input", "-"}
	mutation := []string{"api", "graphql", "-f", "query=mutation{addComment(input:{body:\"x\"}){clientMutationId}}"}

	for name, attempt := range map[string]func() (string, error){
		"a POST through the read door":     func() (string, error) { return gh.Run(review...) },
		"a mutation through the read door": func() (string, error) { return gh.Run(mutation...) },
		"a POST with a fabricated token": func() (string, error) {
			return gh.Confirmation{}.Write(review...)
		},
		"a mutation with a fabricated token": func() (string, error) {
			return gh.Confirmation{}.Write(mutation...)
		},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := attempt()

			var refused *gh.WriteRefusedError
			require.ErrorAs(t, err, &refused)
			assert.Empty(t, out)
			assert.Contains(t, refused.Error(), "§2.1.2")
		})
	}
}

// runners are the files allowed to start an external process: one to each of
// §14.1's three runtime dependencies, and one for the profile's own setup
// commands. Every other file reaches git, gh, the tracker, and §5.1.3 through
// one of them.
//
// Two of the four start a program the user names, so a user who configures
// intent.cmd or sandbox.setup to reach GitHub reaches GitHub. That is the user
// writing their own command line, not cr routing around §2.1.2, and cr could
// not police it in any case: every tracker CLI talks to a network and so does
// every dependency installer. What this list keeps true is that cr itself has
// four doors and no fifth.
var runners = map[string]bool{
	filepath.Join("internal", "gh", "run.go"):      true,
	filepath.Join("internal", "git", "run.go"):     true,
	filepath.Join("internal", "intent", "run.go"):  true,
	filepath.Join("internal", "sandbox", "run.go"): true,
}

// crSource walks the Go files cr ships — its own source, tests excluded — and
// hands each one's path and its import paths to visit.
//
// Imports are read rather than the file's text, because an import is what a
// call needs and cannot be spelled around: a package cannot be reached by
// aliasing it, by building the name at run time, or by any of the ways a
// substring search over source can be defeated.
//
// The surface is eachSourceFile's, which is the same surface this walk used to
// define for itself. Both guards fence what cr ships, so a file the one reads
// and the other does not would be a hole in whichever missed it.
func crSource(t *testing.T, visit func(rel string, imports []string)) {
	t.Helper()
	eachSourceFile(t, func(rel string, file *ast.File) {
		imports := make([]string, 0, len(file.Imports))
		for _, spec := range file.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			imports = append(imports, name)
		}
		visit(rel, imports)
	})
}

// Every outbound call goes through one of two runners, and there is no way out
// of cr that does not.
//
// §2.1.2's boundary is worth exactly as much as the claim that internal/gh is
// the only route to GitHub, and that claim has two ways to fail. A second file
// could start gh itself, which os/exec is the only means of; and cr could
// address the API in process, which needs a network transport. Both are
// imports, so both are visible here.
//
// net/url is the deliberate carve-out. It parses and it opens nothing, and a
// guard that convicted it would be measuring the wrong thing.
func TestCrReachesTheNetworkThroughOneRunnerAndNoOtherWay(t *testing.T) {
	var found []string
	crSource(t, func(rel string, imports []string) {
		for _, name := range imports {
			switch {
			case name == "os/exec" && !runners[rel]:
				found = append(found, rel+" starts an external process of its own")
			case name == "net" || strings.HasPrefix(name, "net/") && name != "net/url":
				found = append(found, rel+" imports the transport "+name)
			case strings.HasPrefix(name, "golang.org/x/net"):
				found = append(found, rel+" imports the transport "+name)
			}
		}
	})

	assert.Empty(t, found,
		"§2.1.2: gh is cr's only route to GitHub, so nothing may reach it around internal/gh")
}

// mintSites are the paths allowed to name gh.Confirm: internal/gh, where the
// mint and the tests that exercise it live, and internal/cli/post.go, which is
// §8.5's gate.
//
// The second entry arrived with dry-run-posting, and it is one file rather than
// a directory because that is the whole claim: §8.5.3 allows one gate, the flag
// is its only input, and the token therefore comes into being where the flag is
// read. Widening this further is a deliberate act with a reviewer, not
// something a call site does by existing — a second caller is a second gate.
var mintSites = []string{
	filepath.Join("internal", "gh") + string(filepath.Separator),
	filepath.Join("internal", "cli", "post.go"),
}

// mints are the two ways a token can come into being: the constructor, and the
// composite literal that would set its unexported field. They are searched for
// as text, that being what a file outside internal/gh cannot write and still
// compile.
//
// The list is shared with gatedCommands, which reads out of it §12.6's set of
// commands that could have performed a network write. §8.5.3 allows exactly one
// gate, so the commands that can mint and the commands that could have written
// are one set, and they are read from one list rather than kept as two.
var mints = []string{"gh.Confirm(", "Confirmation{granted"}

// thisFile is this guard's own path, so the scan can exclude the file whose
// data is the string it searches for. runtime.Caller answers instead of a
// literal name, so moving or renaming the file cannot silently take the
// exclusion with it.
func thisFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "the compiler kept no path for this file, so the guard cannot exclude itself")
	return file
}

// Nothing outside internal/gh mints a Confirmation.
//
// The type's own design already stops a token being fabricated: granted is
// unexported, so every Confirmation composed elsewhere is the zero value and
// writes nothing. What Go cannot express is that the one constructor has one
// caller, and this is that sentence. Without it §8.5's gate is a convention —
// any command could mint a token, pass true, and post without ever having read
// a flag — and §8.5.3 forbids exactly that: no setting, variable, field, or
// alias may supply --confirm implicitly, and a second mint is all four at once.
//
// Tests are scanned like everything else. A test that mints a token outside
// this package is a test asserting cr can post without the gate, which is the
// claim this guard exists to keep false.
func TestNothingOutsideTheGhPackageMintsAConfirmation(t *testing.T) {
	root := moduleRoot(t)
	self := thisFile(t)

	scanned := 0
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
		if filepath.Ext(path) != ".go" || path == self {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		for _, site := range mintSites {
			if strings.HasPrefix(rel, site) {
				return nil
			}
		}

		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		scanned++
		for _, mint := range mints {
			if strings.Contains(string(raw), mint) {
				found = append(found, rel+" names "+mint)
			}
		}
		return nil
	}))

	require.Greater(t, scanned, 10, "only %d files were scanned, so this guard proved nothing", scanned)
	assert.Empty(t, found,
		"§8.5: the confirmation gate is the only mint, so no other path may build a token")
}
