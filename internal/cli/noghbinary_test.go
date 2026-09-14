package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/intent"
)

// refusingGh is the program the fence below installs under the name `gh`. It
// writes a sentence naming what to do and exits non-zero, so a test that
// reaches it fails with an instruction rather than with a network call.
const refusingGh = "#!/bin/sh\n" +
	"echo 'refused: a test reached the production gh. " +
	"Install a shim on the test own PATH and assert on what it received.' >&2\n" +
	"exit 97\n"

// trackerFenceLog is the variable naming the file the tracker fence appends
// each command line it was given to. TestMain points it at a file of its own
// and fails the package when that file is not empty after the run; the test
// that proves the fence points it elsewhere, so its own reach is not counted.
const trackerFenceLog = "CR_TEST_TRACKER_FENCE_LOG"

// refusingTracker is the program the fence installs under the name `jira`,
// §3.1.2's default `intent.cmd`. It names the command line it was given on
// stderr, which §3.1.3 surfaces in the error, records that line, and exits
// non-zero.
const refusingTracker = "#!/bin/sh\n" +
	"echo \"refused: a test reached the production tracker: jira $*. " +
	"Pass --intent-file, or install a shim on the test own PATH.\" >&2\n" +
	"if [ -n \"$" + trackerFenceLog + "\" ]; then printf '%s\\n' \"jira $*\" >> \"$" + trackerFenceLog + "\"; fi\n" +
	"exit 97\n"

// TestMain puts a `gh` and a `jira` that refuse at the front of this test
// binary's PATH, so no test in this package can start the real ones.
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
// The tracker half exists for the same reason. A test that ran `cr brief`
// without `--intent-file` started `jira issue view CR-7` against the machine's
// own tracker, because §3.1.2's default `intent.cmd` names it and nothing stood
// in front of it. A refusal alone would not always fail such a test — one
// asserting only that the brief fails passes on the refusal — so the fence also
// records each command line it was given, and a package whose run left one
// there fails, naming it.
//
// The fence is prepended rather than made the whole of PATH, for two reasons.
// A test that installs its own shim prepends again and wins, which is how every
// test that means to exercise the call works; and `go build`, `git` and the
// other programs the fixtures drive are still reachable, so the fence removes
// exactly the two programs that reach past the machine.
//
// A process started with runAsCR set is not a test run at all: it is cr, and
// TestMain hands it straight to Execute. See spawnProbe for why.
func TestMain(m *testing.M) {
	if os.Getenv(runAsCR) != "" {
		Execute()
		os.Exit(ExitOK)
	}
	dir, err := os.MkdirTemp("", "cr-gh-fence")
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "gh"), []byte(refusingGh), 0o700)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "jira"), []byte(refusingTracker), 0o700)
	}
	reached := filepath.Join(dir, "tracker-reached")
	if err == nil {
		err = os.Setenv(trackerFenceLog, reached)
	}
	if err == nil {
		err = os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot install the gh and tracker fences, so a test could reach GitHub or a tracker:", err)
		os.Exit(1)
	}
	code := m.Run()
	if lines, readErr := os.ReadFile(reached); readErr == nil && len(lines) > 0 {
		fmt.Fprintf(os.Stderr, "FAIL: a test reached the production tracker; pass --intent-file or install a shim. "+
			"The command lines it was given:\n%s", lines)
		code = 1
	}
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

// The tracker fence is in force: `cr brief` given an issue key and no
// `--intent-file` runs §3.1.2's default `intent.cmd`, and the `jira` it starts
// is the fence, which refuses naming the command line it was given and records
// that line.
//
// The fence's record is pointed at this test's own file, so the reach this test
// makes on purpose is not the one TestMain fails the package for. Both halves
// are asserted on their whole values: the stderr the refusal wrote, which a
// `jira` that is not installed could never produce, and the line recorded.
func TestNoTestInThisPackageCanReachTheProductionTracker(t *testing.T) {
	detectedHome(t)
	reached := filepath.Join(t.TempDir(), "reached")
	t.Setenv(trackerFenceLog, reached)

	err := runCLI(t, "brief", fixturePR, "--repo", fixtureSlug, "--issue", "CR-7")

	var ran *intent.CommandError
	require.ErrorAs(t, err, &ran, "the tracker on PATH is a program, so reaching it is a command failure")
	assert.Equal(t, []string{"jira", "issue", "view", "CR-7", "--plain"}, ran.Args)
	assert.Equal(t, "refused: a test reached the production tracker: jira issue view CR-7 --plain. "+
		"Pass --intent-file, or install a shim on the test own PATH.", ran.Stderr)
	assert.Equal(t, ExitFile, exitCodeFor(err), "§3.1.3 codes a tracker command that failed 3")
	recorded, readErr := os.ReadFile(reached)
	require.NoError(t, readErr)
	assert.Equal(t, "jira issue view CR-7 --plain\n", string(recorded))
}
