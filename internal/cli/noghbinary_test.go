package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
)

// refusingGh is the program the fence below installs under the name `gh`. It
// writes a sentence naming what to do and exits non-zero, so a test that
// reaches it fails with an instruction rather than with a network call.
const refusingGh = "#!/bin/sh\n" +
	"echo 'refused: a test reached the production gh. " +
	"Install a shim on the test own PATH and assert on what it received.' >&2\n" +
	"exit 97\n"

// TestMain puts a `gh` that refuses at the front of this test binary's PATH,
// so no test in this package can start the real one.
//
// It is here because a convention was not enough, and the way it failed is the
// argument for the shape. `cr post --confirm` reached `gh api
// repos/acme/web/pulls/7/reviews --method POST` against api.github.com during
// `go test`, and the test that did it named nothing about gh at all — it ran a
// command. A source-level guard listing the files allowed to name gh.New or
// gh.Confirm would have read that file and found nothing to object to; only a
// fence at the exec can see a reach that arrives through six function calls.
//
// The run failed on a TLS error because this machine had no network, which is
// not a property of the repository. On a laptop with a token, or in CI, the
// same test posts a review to whatever `acme/web#7` resolves to.
//
// The fence is prepended rather than made the whole of PATH, for two reasons.
// A test that installs its own shim prepends again and wins, which is how every
// test that means to exercise the call works; and `go build`, `git` and the
// other programs the fixtures drive are still reachable, so the fence removes
// exactly one thing.
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

// The fence is in force: a test in this package that reaches the gh binary
// meets the refusal and never the network.
//
// It is asserted through gh.Run because that is the door this package can open
// without minting a token — nowrite_test.go's mintSites allows the mint in
// internal/cli/post.go and nowhere else, this file included. The write door is
// fenced by the same thing either way: Run and Confirmation.Write both reach
// the binary through one unexported invoke, so a PATH that cannot resolve the
// real gh cannot resolve it for either.
//
// The assertion is on the refusal's own words rather than merely on an error,
// because a gh that is not installed at all fails too — and that failure would
// make this test pass on a machine where it proves nothing.
func TestNoTestInThisPackageCanReachTheProductionGh(t *testing.T) {
	out, err := gh.Run("api", "graphql", "-f", "query=query{viewer{login}}")

	require.Error(t, err)
	assert.Empty(t, out)
	var ran *gh.CommandError
	require.ErrorAs(t, err, &ran,
		"the fence is a program, so a test that reaches it gets a command failure")
	assert.Contains(t, ran.Stderr, "a test reached the production gh",
		"the gh on PATH is the fence, and not a real one that happens to be absent")
}

// A test that installs its own shim still wins, which is what keeps the fence a
// safety net rather than a wall.
//
// Both halves matter. A fence that could not be overridden would make the send
// path untestable; a fence a shim did not override would make every such test
// pass for the wrong reason, asserting on a refusal it mistook for its own
// shim's answer.
func TestAShimOnTheTestsOwnPathOverridesTheFence(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"),
		[]byte("#!/bin/sh\nprintf '%s' '{\"answered\":\"by the shim\"}'\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, err := gh.Run("api", "graphql", "-f", "query=query{viewer{login}}")

	require.NoError(t, err)
	assert.Equal(t, `{"answered":"by the shim"}`, out)
}
