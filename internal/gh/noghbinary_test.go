package gh

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refusingGh is the program the fence installs under the name `gh`, and the
// twin of internal/cli's. The two are separate test binaries with separate
// environments, so each installs its own; a shared helper would have to be a
// package cr ships in order to be importable from both, which is a wider
// surface than twenty lines repeated once.
const refusingGh = "#!/bin/sh\n" +
	"echo 'refused: a test reached the production gh. " +
	"Install a shim on the test own PATH and assert on what it received.' >&2\n" +
	"exit 97\n"

// TestMain puts a `gh` that refuses at the front of this test binary's PATH.
//
// This package is the only one that starts the gh binary — nowrite_test.go's
// `runners` fixes that — so the two fences together are the whole of the
// guarantee that `go test` cannot reach GitHub. The reason is internal/cli's,
// where it was measured: a test there ran a real `gh api ... --method POST`
// against api.github.com and failed on a TLS error, which is a property of the
// machine and not of the repository.
//
// Every test here already installs its own stub through stubGh, which prepends
// to PATH and so wins over this. What the fence adds is the one written next
// month that forgets to.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "cr-gh-fence")
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "gh"), []byte(refusingGh), 0o700)
	}
	if err == nil {
		err = os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot install the gh fence, so a test could reach GitHub:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// The fence is in force at both doors, which this package can assert and
// internal/cli cannot: the mint lives here, so a write can be attempted with a
// real token rather than with the zero value that is refused before it starts
// anything.
//
// Both doors reach the binary through one unexported invoke, so what is
// measured is that the binary on PATH is the fence. The refusal's own words are
// the assertion, because a machine with no gh installed also fails — and on
// that machine an error alone would prove nothing.
func TestNoTestInThisPackageCanReachTheProductionGh(t *testing.T) {
	for name, attempt := range map[string]func() (string, error){
		"the read door": func() (string, error) {
			return Run("api", "graphql", "-f", "query=query{viewer{login}}")
		},
		"the write door": func() (string, error) {
			return Confirm(true).Write(nil, "api", "repos/cli/cli/pulls/1/reviews", "--method", "POST")
		},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := attempt()

			require.Error(t, err)
			assert.Empty(t, out)
			var ran *CommandError
			require.ErrorAs(t, err, &ran,
				"the fence is a program, so reaching it is a command failure")
			assert.Contains(t, ran.Stderr, "a test reached the production gh",
				"the gh on PATH is the fence, and not a real one that happens to be absent")
		})
	}
}
