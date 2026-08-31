package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
