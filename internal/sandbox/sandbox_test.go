package sandbox

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// The pull request every fixture here stands for.
const (
	fixtureOwner = "acme"
	fixtureRepo  = "web"
	fixturePR    = 42
)

// runGit runs one command in a fixture repository and returns its trimmed output.
// A failure fails the test: a fixture that did not build proves nothing about
// the code under test.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// repository builds the repository under review and returns it with the head
// the pull request is at.
//
// The head is on a branch other than the checked-out one, so a sandbox created
// at the wrong revision is visible rather than accidentally right.
func repository(t *testing.T) (dir, head string) {
	t.Helper()
	dir = t.TempDir()
	runGit(t, dir, "init", "--quiet", "--initial-branch=main")
	runGit(t, dir, "config", "user.email", "fixture@cr.test")
	runGit(t, dir, "config", "user.name", "cr fixture")
	runGit(t, dir, "config", "commit.gpgsign", "false")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.txt"), []byte("the base\n"), 0o600))
	runGit(t, dir, "add", "app.txt")
	runGit(t, dir, "commit", "--quiet", "-m", "the base")

	runGit(t, dir, "checkout", "--quiet", "-b", "pr-head")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.txt"), []byte("under review\n"), 0o600))
	runGit(t, dir, "commit", "--quiet", "-a", "-m", "the change under review")
	head = runGit(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "checkout", "--quiet", "main")

	return dir, head
}

// sources is the fixture's Sources, with a state root of its own so no test
// touches a real ~/.cr.
func sources(t *testing.T, dir, head string) *Sources {
	t.Helper()
	return &Sources{
		Layout:  state.New(filepath.Join(t.TempDir(), ".cr")),
		Owner:   fixtureOwner,
		Repo:    fixtureRepo,
		PR:      fixturePR,
		Head:    head,
		RepoDir: dir,
	}
}

// §5.1.1: the sandbox is a worktree at the pull request head, under the pull
// request's own state directory.
//
// The reported path and the path on disk are both checked against the layout's
// answer, because a Result that named one directory while git wrote another
// would leave every later probe running somewhere the report never mentioned.
func TestCreateMakesTheWorktreeAtTheHead(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)

	created, err := Create(src)
	require.NoError(t, err)

	assert.Equal(t, src.Layout.Sandbox(fixtureOwner, fixtureRepo, fixturePR), created.Path)
	assert.Equal(t, head, created.Head)
	assert.Equal(t, head, runGit(t, created.Path, "rev-parse", "HEAD"))

	body, err := os.ReadFile(filepath.Join(created.Path, "app.txt"))
	require.NoError(t, err)
	assert.Equal(t, "under review\n", string(body))
}

// A sandbox that is already there is reported, and left exactly as it was.
//
// §5.1.1 creates the worktree and §5.1.5 removes it, so creating over one would
// be cr deleting a checkout it did not just make — possibly mid-run, possibly
// carrying the mutation §5.3.3 is about to revert. The sentinel is what proves
// the refusal is more than a message: a run that reported the conflict and had
// already emptied the directory would pass an assertion on the error alone.
func TestCreateRefusesASandboxThatIsAlreadyThere(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)

	path := src.Layout.Sandbox(fixtureOwner, fixtureRepo, fixturePR)
	require.NoError(t, os.MkdirAll(path, 0o700))
	sentinel := filepath.Join(path, "mid-run.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("a run in progress\n"), 0o600))

	created, err := Create(src)

	assert.Nil(t, created)
	var exists *ExistsError
	require.ErrorAs(t, err, &exists)
	assert.Equal(t, path, exists.Path)
	assert.Contains(t, err.Error(), "cr sandbox destroy 42 --repo acme/web",
		"§12.4: the error names the next actionable step")

	body, err := os.ReadFile(sentinel)
	require.NoError(t, err, "the refusal must leave the existing sandbox alone")
	assert.Equal(t, "a run in progress\n", string(body))
}

// script writes an executable shell script under dir and returns its path, so a
// `sandbox.setup` entry can name a real program.
//
// A script rather than an inline command line: §5.1.3's entries are split on
// whitespace and started directly, with no shell to quote for, so anything
// worth observing has to live in a file.
func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700))
	return path
}

// §5.1.2 and §5.1.3: every copy lands before the first setup command runs, and
// the commands run once each, in order, in the sandbox root.
//
// The ordering is observed rather than asserted about the code. The first setup
// command reads the two copied paths and appends what it found to a log outside
// the sandbox, and the second appends after it — so the log is a recording of
// what was true when each command ran. A copy that happened after setup would
// leave the first two lines as the shell's own "No such file" complaints; a
// command that ran twice would repeat a line; a pair that ran in the other
// order would swap them. Only the sequence §5.1 describes produces this file.
//
// `ran-here.txt` is the sandbox root half. It is written to a relative path, so
// where it lands is where the command's working directory was.
func TestEveryCopyLandsBeforeTheFirstSetupCommandRuns(t *testing.T) {
	dir, head := repository(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_ENV=testing\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "vendor", "marker.txt"), []byte("the vendor tree\n"), 0o600))

	scripts := t.TempDir()
	log := filepath.Join(scripts, "observed.log")
	first := script(t, scripts, "first.sh",
		"{ cat .env; cat vendor/marker.txt; echo first; } >> "+log+" 2>&1\n"+
			"echo ran > ran-here.txt\n")
	second := script(t, scripts, "second.sh", "echo second >> "+log+"\n")

	src := sources(t, dir, head)
	src.Copy = []string{".env", "vendor"}
	src.Setup = []string{first, second}

	created, err := Create(src)
	require.NoError(t, err)

	observed, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, "APP_ENV=testing\nthe vendor tree\nfirst\nsecond\n", string(observed),
		"§5.1.2 completes before §5.1.3 starts, and the commands run once each in order")

	assert.FileExists(t, filepath.Join(created.Path, "ran-here.txt"),
		"§5.1.3: the commands run in the sandbox root")
	assert.Equal(t, []string{".env", "vendor"}, created.Copied)
	assert.Empty(t, created.Absent)
	assert.Equal(t, []string{first, second}, created.Setup)
}

// M-1.5: §5.1.2's copy never writes through a link the head under review checks
// out, however the link is placed.
//
// `sandbox.copy` entries are held to a path inside the checkout lexically, which
// settles what the *profile* may name and nothing about what the *head* puts
// there. The head is the pull request's content: a branch may perfectly well
// track `.env` as a link to somewhere else on the machine, or track the
// directory a copied path descends through as one. The copy then opens the name
// for writing, the kernel follows the link, and the user's own `.env` — the file
// `sandbox.copy` exists to carry, credentials included — is written to a path
// outside ~/.cr entirely. Invariant 2 permits cr one write inside the repository
// under review and none anywhere else.
//
// Measured against the unfixed code: both rows overwrote the file outside, which
// held "the file outside" before the creation and "APP_ENV=testing" after it.
//
// The sandbox must still end up equal to the checkout for the copied path
// (§5.1.2), so it is asserted in the same breath: refusing the entry and
// refusing to write outside are not the same answer, and only one of them leaves
// the suite able to run.
func TestACopyDoesNotWriteThroughALinkTheHeadCheckedOut(t *testing.T) {
	for name, fixture := range map[string]struct {
		// link is the path the head tracks as a symbolic link, and
		// copy the `sandbox.copy` entry that lands on or under it.
		link, copy string
	}{
		"the copied path is itself a link":        {link: ".env", copy: ".env"},
		"the copied path descends through a link": {link: "config", copy: "config/outside.txt"},
	} {
		t.Run(name, func(t *testing.T) {
			dir, _ := repository(t)
			outside := t.TempDir()
			target := filepath.Join(outside, "outside.txt")
			require.NoError(t, os.WriteFile(target, []byte("the file outside\n"), 0o600))

			// The head tracks the link. A directory the copy
			// descends through is linked to the directory the file
			// outside sits in, so the write lands beside it.
			pointsAt := target
			if fixture.link != fixture.copy {
				pointsAt = outside
			}
			runGit(t, dir, "checkout", "--quiet", "pr-head")
			require.NoError(t, os.Symlink(pointsAt, filepath.Join(dir, fixture.link)))
			runGit(t, dir, "add", fixture.link)
			runGit(t, dir, "commit", "--quiet", "-m", "a link where a copied path goes")
			head := runGit(t, dir, "rev-parse", "HEAD")
			runGit(t, dir, "checkout", "--quiet", "main")

			// The checkout cr copies from holds an ordinary file
			// there, which is what a developer's own `.env` is.
			source := filepath.Join(dir, fixture.copy)
			require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o750))
			require.NoError(t, os.WriteFile(source, []byte("APP_ENV=testing\n"), 0o600))

			src := sources(t, dir, head)
			src.Copy = []string{fixture.copy}

			created, err := Create(src)
			require.NoError(t, err)

			body, err := os.ReadFile(target)
			require.NoError(t, err)
			assert.Equal(t, "the file outside\n", string(body),
				"invariant 2: the copy followed the head's link and wrote outside the sandbox")
			assert.Equal(t, []string{fixture.copy}, created.Copied)

			copied, err := os.ReadFile(filepath.Join(created.Path, fixture.copy))
			require.NoError(t, err, "§5.1.2: the path the profile named is in the sandbox")
			assert.Equal(t, "APP_ENV=testing\n", string(copied))

			landed, err := os.Lstat(filepath.Join(created.Path, fixture.copy))
			require.NoError(t, err)
			assert.Zero(t, landed.Mode()&os.ModeSymlink,
				"the copy replaced the head's link rather than writing through it")
		})
	}
}

// A setup command that refuses stops the run, and what it wrote reaches the
// user.
//
// §3.1.3 fixes that shape for the external commands cr drives, and a setup
// command is one: cr has never heard of the tool, so its stderr is the only
// diagnostic there is. The second command is the other half — §5.1.3 runs the
// commands in order, and an order means the next one does not start when the
// one before it failed, since it would be running against a sandbox the failed
// step never finished preparing.
func TestASetupCommandThatFailsStopsTheRun(t *testing.T) {
	dir, head := repository(t)
	scripts := t.TempDir()
	log := filepath.Join(scripts, "observed.log")
	refusing := script(t, scripts, "refuse.sh", "echo the tool refused >&2\nexit 3\n")
	after := script(t, scripts, "after.sh", "echo after >> "+log+"\n")

	src := sources(t, dir, head)
	src.Setup = []string{refusing, after}

	created, err := Create(src)

	assert.Nil(t, created)
	var failed *SetupError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, []string{refusing}, failed.Args)
	assert.Equal(t, "the tool refused", failed.Stderr)
	assert.Contains(t, err.Error(), "sandbox.setup")
	assert.Contains(t, err.Error(), "the tool refused",
		"§3.1.3: the command's stderr is the only diagnostic there is, so it has to be in the message")

	// The exec failure is reachable through the error, which is what tells
	// a command that ran and refused from one that never started at all.
	var exit *exec.ExitError
	require.ErrorAs(t, errors.Unwrap(failed), &exit)
	assert.Equal(t, 3, exit.ExitCode())
	assert.NoFileExists(t, log, "§5.1.3 runs the commands in order, so the next one does not start")
}

// A profile whose steps cannot be carried out is refused before anything is
// created.
//
// `sandbox.copy` and `sandbox.setup` are data the user writes, and each of
// these three entries would do something other than what §5.1 describes: a path
// out of the checkout writes outside the sandbox, which invariant 2 permits
// nowhere; an empty path addresses the whole checkout; and an entry with no
// command in it names no program. §2.5 item 3 gives such a field exit code 3
// with the file and the field named, which is what MalformedError carries.
//
// The absent sandbox is the second half. Refusing after the worktree was added
// would leave a checkout behind that §5.1.1 then refuses to create over, so a
// user who fixed the profile could not simply run the command again.
func TestAProfileStepThatCannotBeCarriedOutIsRefused(t *testing.T) {
	for name, step := range map[string]func(src *Sources){
		"a copy path that leaves the checkout": func(src *Sources) {
			src.Copy = []string{filepath.Join("..", "..", "elsewhere")}
		},
		"an empty copy path": func(src *Sources) { src.Copy = []string{""} },
		"an absolute copy path": func(src *Sources) {
			src.Copy = []string{filepath.Join(string(filepath.Separator), "etc", "passwd")}
		},
		"a setup entry with no command in it": func(src *Sources) {
			src.Setup = []string{"   "}
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir, head := repository(t)
			src := sources(t, dir, head)
			src.ProfileFile = filepath.Join("home", ".cr", "profiles", "laravel-pest.json")
			step(src)

			created, err := Create(src)

			assert.Nil(t, created)
			var malformed *profile.MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, src.ProfileFile, malformed.File,
				"§2.5 item 3: the abort names the file the user has to open")
			assert.Contains(t, malformed.Field, "sandbox.")
			assert.NoDirExists(t, src.Layout.Sandbox(fixtureOwner, fixtureRepo, fixturePR),
				"the refusal must leave no sandbox behind for §5.1.1 to refuse over")
		})
	}
}

// §5.1.6: once §5.1.2 and §5.1.3 have completed, the sandbox's tracked-file
// state is recorded as the post-setup baseline — and the record sits under the
// pull request's state directory, never in the worktree it describes.
//
// Where it lives is the claim with teeth, and round 8's unhomed-state finding
// is why. The sandbox is a worktree of the repository under review, so a
// baseline written into it would be cr writing inside that repository, which
// §2.2 permits for nothing but §5.1.1's registration. It would also be a record
// that disappears with the checkout it exists to describe, at the exact moment
// §5.1.6 needs it to decide whether that checkout can be trusted.
//
// The setup command modifies a tracked file on purpose. §5.1.3's commands are
// there to change the checkout — `composer install` rewrites a lock file — so a
// baseline that recorded nothing but "clean at HEAD" would condemn every
// sandbox on its first use, and §5.1.6's mandated recreation would never end.
func TestTheBaselineIsRecordedOutsideTheWorktreeAfterSetup(t *testing.T) {
	dir, head := repository(t)
	scripts := t.TempDir()

	src := sources(t, dir, head)
	src.Setup = []string{script(t, scripts, "setup.sh", "echo touched-by-setup > app.txt\n")}

	created, err := Create(src)
	require.NoError(t, err)

	recorded := src.Layout.PRFile(fixtureOwner, fixtureRepo, fixturePR, state.FileSandboxBaseline)
	require.FileExists(t, recorded, "§5.1.6 records the post-setup baseline")
	assert.NoFileExists(t, filepath.Join(created.Path, state.FileSandboxBaseline),
		"§2.2: the sandbox is a worktree of the repository under review")
	inside, err := filepath.Rel(created.Path, recorded)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(inside, ".."+string(filepath.Separator)),
		"the baseline resolves to %s, which is inside the sandbox", recorded)

	body, err := os.ReadFile(recorded)
	require.NoError(t, err)
	var baseline Baseline
	require.NoError(t, json.Unmarshal(body, &baseline))

	assert.Equal(t, head, baseline.Head, "§5.1.6 reads the baseline beside the head it was taken at")
	assert.Equal(t, []string{"app.txt"}, baseline.Paths)
	assert.Contains(t, baseline.Diff, "touched-by-setup",
		"§5.1.3 legitimately modifies tracked files, so the baseline is what they became")
}

// The baseline is invalidated when the sandbox it describes is recreated.
//
// It is dropped before the new worktree is added rather than overwritten after
// setup, and the gap between those two moments is what this covers. The second
// creation dies inside §5.1.3, so no new baseline is ever taken — and what must
// be on disk afterwards is nothing at all. Leaving the first sandbox's record in
// place would have §5.1.6 measure a half-prepared checkout against a baseline
// taken from a checkout that no longer exists, and a match there is a sandbox
// declared clean on the strength of a coincidence.
func TestTheBaselineIsInvalidatedWhenTheSandboxIsRecreated(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)

	created, err := Create(src)
	require.NoError(t, err)
	recorded := src.Layout.PRFile(fixtureOwner, fixtureRepo, fixturePR, state.FileSandboxBaseline)
	require.FileExists(t, recorded)

	// §5.1.5's removal, which is what precedes any recreation.
	runGit(t, dir, "worktree", "remove", "--force", created.Path)
	require.NoDirExists(t, created.Path)

	scripts := t.TempDir()
	src.Setup = []string{script(t, scripts, "refuse.sh", "echo the tool refused >&2\nexit 3\n")}

	again, err := Create(src)

	assert.Nil(t, again)
	var failed *SetupError
	require.ErrorAs(t, err, &failed)
	assert.NoFileExists(t, recorded,
		"the previous sandbox's baseline outlived the sandbox it described")
}

// §5.2.1: the test command runs inside the sandbox, and never in the checkout
// cr was invoked from.
//
// This is the criterion with teeth, and it is asserted from inside the running
// process rather than from the call: the script records its own working
// directory, so what is checked is where the runner stood, not what the caller
// meant. A suite run in the main checkout would be measuring the user's working
// tree — mid-edit, possibly on another branch, possibly carrying the change
// under review and possibly not — and reporting the answer under the pull
// request head's name. It would also be a run cr had no business starting
// there: §2.2 permits cr nothing inside the repository under review but
// §5.1.1's registration, and every later mutation of §5.3 happens in the
// directory this one establishes.
//
// The exit status comes back as a value. A failing suite is an ordinary outcome
// — §5.3.4's ladder is built on reading one — so a non-zero exit is reported and
// not raised.
func TestTheTestCommandRunsInTheSandboxAndNeverTheCheckout(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)
	created, err := Create(src)
	require.NoError(t, err)

	scripts := t.TempDir()
	log := filepath.Join(scripts, "observed.log")
	runner := script(t, scripts, "runner.sh", "pwd > "+log+"\necho the suite ran\nexit 3\n")

	var printed strings.Builder
	code, timedOut, err := Run([]string{runner}, created.Path, &printed, time.Minute, nil)
	require.NoError(t, err)

	assert.Equal(t, 3, code, "§5.2.1 reports the runner's exit status rather than raising it")
	assert.False(t, timedOut, "§5.2.3: a run that finished inside its budget was not killed")
	assert.Contains(t, printed.String(), "the suite ran",
		"the runner's output reaches the reader as it is produced")

	observed, err := os.ReadFile(log)
	require.NoError(t, err, "the test command never ran")
	// The sandbox path is resolved through its symlinks, because a
	// temporary directory reaches `pwd` as the path the kernel resolved and
	// not as the one the layout joined.
	sandboxPath, err := filepath.EvalSymlinks(created.Path)
	require.NoError(t, err)
	checkout, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	ran := strings.TrimSpace(string(observed))
	assert.Equal(t, sandboxPath, ran, "§5.2.1: the suite runs inside the sandbox")
	assert.NotEqual(t, checkout, ran, "§2.2: never in the repository under review")
}

// A runner that cannot be started at all is a failure and not a test result.
//
// The two are different answers and §5.3.4 reads them differently: a runner that
// ran and exited non-zero has said something about the code, while one that
// never started has said nothing. §3.1.3 gives the second the shape every
// external command cr drives fails in, and the argv is carried into it so the
// user can see what cr tried to start.
func TestARunnerThatCannotBeStartedIsAFailure(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)
	created, err := Create(src)
	require.NoError(t, err)

	code, timedOut, err := Run(
		[]string{filepath.Join(created.Path, "no-such-runner")},
		created.Path, io.Discard, time.Minute, nil)

	assert.Zero(t, code)
	assert.False(t, timedOut)
	var failed *RunError
	require.ErrorAs(t, err, &failed)
	assert.Contains(t, err.Error(), "no-such-runner")
}

// §5.2.3: a run that outlives `tests.timeout_seconds` is killed, and what is
// killed is the process group rather than the runner alone.
//
// The criterion with teeth is the group, and the test is built so that a kill
// aimed at the parent alone fails it twice over. The runner starts a child
// that loops forever and never exits on its own, then sleeps far past the
// budget. The child inherits the pipe the runner's output travels through, so
// signalling only the parent would leave cmd.Wait blocked on a pipe nothing
// ever closes — this function would not return at all, which is the first
// failure. And the child would still be running afterwards, which the pid it
// wrote down proves directly: it is gone within seconds of the kill, and the
// only thing that could have ended it is the signal sent to the group.
//
// That is not a hypothetical shape. A Laravel suite starts `php` and
// `composer` and a database client, and an orphan holding the test database is
// exactly what §5.6's lock exists to prevent — a lock cr released while a
// process it started still held the resource would serialise nothing.
func TestARunOutlivingItsBudgetIsKilledWithEverythingItStarted(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)
	created, err := Create(src)
	require.NoError(t, err)

	scripts := t.TempDir()
	noted := filepath.Join(scripts, "child.pid")
	runner := script(t, scripts, "runner.sh",
		"sh -c 'while :; do sleep 0.2; done' &\necho $! > "+noted+"\nsleep 120\n")

	var printed strings.Builder
	started := time.Now()
	code, timedOut, err := Run([]string{runner}, created.Path, &printed, 500*time.Millisecond, nil)
	took := time.Since(started)

	require.NoError(t, err, "a killed run is an outcome, not a runner that could not be started")
	assert.True(t, timedOut, "§5.2.3: the run exceeded its budget and was killed")
	assert.NotEqual(t, 0, code, "a killed process did not exit cleanly")
	assert.Less(t, took, 30*time.Second,
		"the run returned rather than waiting on a pipe a survivor still held")

	written, err := os.ReadFile(noted)
	require.NoError(t, err, "the runner never got as far as starting a child")
	child, err := strconv.Atoi(strings.TrimSpace(string(written)))
	require.NoError(t, err)

	// Signal 0 asks whether the process is still there, and ESRCH is the
	// answer that it is not. Polled rather than read once, because the kill
	// and the reaping are the kernel's to schedule.
	assert.Eventually(t, func() bool {
		return errors.Is(syscall.Kill(child, 0), syscall.ESRCH)
	}, 10*time.Second, 20*time.Millisecond,
		"the child the runner started outlived the kill, so only the parent was signalled")
}

// A timeout of zero or less is no budget rather than an expired one.
//
// §2.4 defaults `tests.timeout_seconds` to 900 and refuses a non-positive
// value, so no loaded profile reaches Run without one; what this pins is the
// direction of the fallback. Reading zero as a deadline already passed would
// kill every run of a caller that forgot to pass a budget, and report the
// suite as having exceeded a limit nobody set.
func TestARunGivenNoBudgetIsNotKilled(t *testing.T) {
	dir, head := repository(t)
	src := sources(t, dir, head)
	created, err := Create(src)
	require.NoError(t, err)

	runner := script(t, t.TempDir(), "runner.sh", "echo the suite ran\n")

	var printed strings.Builder
	code, timedOut, err := Run([]string{runner}, created.Path, &printed, 0, nil)

	require.NoError(t, err)
	assert.Zero(t, code)
	assert.False(t, timedOut, "§5.2.3 kills a run that exceeded a budget, and there was none")
	assert.Contains(t, printed.String(), "the suite ran")
}
