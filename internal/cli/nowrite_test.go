package cli

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
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

// runners are the files allowed to start an external process. Every other file
// reaches git and gh through one of them.
var runners = map[string]bool{
	filepath.Join("internal", "gh", "run.go"):  true,
	filepath.Join("internal", "git", "run.go"): true,
}

// crSource walks the Go files cr ships — its own source, tests excluded — and
// hands each one's path and its import paths to visit.
//
// Imports are read rather than the file's text, because an import is what a
// call needs and cannot be spelled around: a package cannot be reached by
// aliasing it, by building the name at run time, or by any of the ways a
// substring search over source can be defeated.
func crSource(t *testing.T, visit func(rel string, imports []string)) {
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
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		require.NoError(t, err)

		imports := make([]string, 0, len(parsed.Imports))
		for _, spec := range parsed.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			imports = append(imports, name)
		}
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)

		scanned++
		visit(rel, imports)
		return nil
	}))

	// A walk that found nothing would pass and would mean nothing.
	require.Greater(t, scanned, 10, "only %d files were scanned, so this guard proved nothing", scanned)
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
