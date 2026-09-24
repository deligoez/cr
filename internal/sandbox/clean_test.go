package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The leftover glob a profile's `tests.probe_path_template` resolves to: the
// template with its `<probe-id>` position replaced by `*`.
const fixtureLeftoverGlob = "cr_probe_*.txt"

// sandboxed builds the repository under review, creates its sandbox, and drops
// an untracked sentinel into it.
//
// The sentinel does two jobs at once. It is a file §5.1.6 says to ignore —
// `sandbox.copy` and `sandbox.setup` create untracked files by design — and it
// is how a recreation is proved to have happened rather than merely reported: a
// rebuilt sandbox is a fresh checkout, so the sentinel is gone.
func sandboxed(t *testing.T) (src *Sources, path, sentinel string) {
	t.Helper()
	dir, head := repository(t)
	src = sources(t, dir, head)
	created, err := Create(src)
	require.NoError(t, err)

	sentinel = filepath.Join(created.Path, "left-behind.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("copied in by setup\n"), 0o600))
	return src, created.Path, sentinel
}

// §5.1.6: a sandbox failing any of the checks is recreated, and the recreation
// is reported.
//
// Each row breaks one of the four things the section verifies, and every row
// asserts the same three facts. The reason names what was found, because a
// notice saying only that something was rebuilt tells a reader nothing they can
// act on. The sentinel is gone, because a check that reported a recreation
// without performing one would leave every later probe running in the sandbox
// that failed. And the second call comes back with no notice at all — §5.1.6's
// recreation has to converge, or a run would rebuild its sandbox before every
// probe for ever.
func TestAnUncleanSandboxIsRecreatedAndTheRecreationIsReported(t *testing.T) {
	for name, breaks := range map[string]struct {
		spoil  func(t *testing.T, src *Sources, path string)
		reason string
	}{
		"the sandbox is gone": {
			spoil: func(t *testing.T, src *Sources, path string) {
				t.Helper()
				runGit(t, src.RepoDir, "worktree", "remove", "--force", path)
			},
			reason: "there is no sandbox at that path",
		},
		"the head moved": {
			spoil: func(t *testing.T, _ *Sources, path string) {
				t.Helper()
				runGit(t, path, "checkout", "--detach", "--quiet", "main")
			},
			reason: "the round's head is",
		},
		"a tracked file was dirtied": {
			spoil: func(t *testing.T, _ *Sources, path string) {
				t.Helper()
				require.NoError(t, os.WriteFile(
					filepath.Join(path, "app.txt"), []byte("a mutation nobody reverted\n"), 0o600))
			},
			reason: "tracked files differ from the post-setup baseline: app.txt",
		},
		"a probe artefact was left behind": {
			spoil: func(t *testing.T, _ *Sources, path string) {
				t.Helper()
				require.NoError(t, os.WriteFile(
					filepath.Join(path, "cr_probe_p1.txt"), []byte("a gap probe's test\n"), 0o600))
			},
			reason: "a probe artefact was left behind: cr_probe_p1.txt",
		},
		"no baseline was recorded": {
			spoil: func(t *testing.T, src *Sources, _ string) {
				t.Helper()
				require.NoError(t, os.Remove(
					src.Layout.PRFile(fixtureOwner, fixtureRepo, fixturePR, state.FileSandboxBaseline)))
			},
			reason: "no post-setup baseline was recorded for it",
		},
	} {
		t.Run(name, func(t *testing.T) {
			src, path, sentinel := sandboxed(t)
			breaks.spoil(t, src, path)

			ready, err := Ensure(src, fixtureLeftoverGlob)
			require.NoError(t, err)

			require.NotNil(t, ready.Recreated, "§5.1.6: the sandbox failed a check and was not rebuilt")
			assert.Equal(t, path, ready.Path)
			assert.Contains(t, ready.Recreated.Reason, breaks.reason)
			assert.Contains(t, ready.Recreated.Disclosure(), breaks.reason,
				"§11.1 prints the notice whatever the flags say, so the reason has to be in it")
			assert.NoFileExists(t, sentinel, "the recreation was reported but never performed")
			assert.Equal(t, src.Head, runGit(t, path, "rev-parse", "HEAD"))

			again, err := Ensure(src, fixtureLeftoverGlob)
			require.NoError(t, err)
			assert.Nil(t, again.Recreated,
				"§5.1.6's recreation has to converge, or every run rebuilds its sandbox for ever")
		})
	}
}

// §5.1.6's leftover-artefact scan is evaluated over untracked files only, and
// every other untracked file is ignored.
//
// Round 12's artefact-glob-not-scoped-to-untracked finding is the first half,
// and the glob here is deliberately one that matches a file the repository
// tracks. §2.4 requires each profile's `tests.probe_path_template` to resolve
// where its own `tests.globs` point, so a template whose glob also catches a
// committed test file is not a misconfiguration to be refused — it is the
// ordinary case. A scan that counted the tracked match would find a leftover
// artefact in every sandbox on every run, and §5.1.6's mandated recreation
// would never converge: rebuilding restores the very file it objected to.
//
// The second half is the sentence after it. `sandbox.copy` and `sandbox.setup`
// create untracked files by design, so an untracked file outside the glob is
// not evidence of anything.
//
// The mixed case is what makes the scoping observable rather than merely
// asserted: one glob, two matches, and only the untracked one may be named.
func TestOnlyAnUntrackedFileUnderTheGlobIsALeftoverArtefact(t *testing.T) {
	// Matches app.txt, which the fixture repository tracks.
	const overlapping = "app*.txt"

	t.Run("a tracked match and a benign untracked file", func(t *testing.T) {
		src, path, sentinel := sandboxed(t)
		require.FileExists(t, filepath.Join(path, "app.txt"),
			"the glob has to match something tracked, or this proves nothing")

		ready, err := Ensure(src, overlapping)
		require.NoError(t, err)

		assert.Nil(t, ready.Recreated,
			"a tracked file under the glob made the sandbox permanently unclean")
		assert.FileExists(t, sentinel,
			"§5.1.6 ignores the untracked files sandbox.copy and sandbox.setup create")
	})

	t.Run("a tracked match beside an untracked one", func(t *testing.T) {
		src, path, _ := sandboxed(t)
		require.NoError(t, os.WriteFile(
			filepath.Join(path, "app-probe.txt"), []byte("a gap probe's test\n"), 0o600))

		ready, err := Ensure(src, overlapping)
		require.NoError(t, err)

		require.NotNil(t, ready.Recreated)
		assert.Contains(t, ready.Recreated.Reason, "app-probe.txt")
		assert.NotContains(t, ready.Recreated.Reason, " app.txt",
			"the tracked match is not a leftover artefact and must not be named as one")
	})
}

// §11.1 exempts the sandbox recreation notice of §5.1.6 from `--quiet`, and the
// exemption belongs to the writer rather than to each call site, so the notice
// has to arrive there as a disclosure.
//
// Implementing finding.HonestyDisclosure is what makes that possible before the
// writer exists — the same contract activation.Disabled, intent.Unavailable,
// testadequacy.Unavailable, profile.MissingProfile and finding.CommentCap
// already satisfy. A notice that only knew how to print itself would be
// silenced by the flag, and a sandbox quietly rebuilt is a run whose previous
// probe left something behind or whose head moved underneath it.
//
// The text is asserted against the fields rather than against a literal, because
// the two must not be able to drift: a recreation recorded in the data and left
// out of the printed report would be honest to a caller reading JSON and silent
// to the human reading a terminal.
func TestTheRecreationNoticeIsAnHonestyDisclosure(t *testing.T) {
	var _ finding.HonestyDisclosure = Recreated{}

	notice := Recreated{
		Path:   "/home/dev/.cr/state/acme/web/pr-42/sandbox",
		Reason: "a probe artefact was left behind: tests/cr_probe_p1.php",
	}

	disclosed := notice.Disclosure()
	assert.Contains(t, disclosed, notice.Path, "a reader told a sandbox was rebuilt wants to know which")
	assert.Contains(t, disclosed, notice.Reason)
	assert.Contains(t, disclosed, "§5.1.6")
}

// M-1.4: a sandbox directory that is no longer a readable worktree is an
// unclean sandbox, not a failure of the check.
//
// The directory is there, so §5.1.6's first question is answered, and every
// question after it is asked of git — which cannot answer any of them, because
// the `.git` file that makes the directory a worktree is broken or gone. A check
// that returned that error would stop `cr test` and `cr probe run` at the one
// state recreation exists to clear: the S10 field note's "missing but already
// registered worktree" is the same state seen from `cr sandbox create`, and
// nothing cr offers gets out of it.
//
// Measured against the unfixed code, both rows: Ensure returned git's own
// `fatal: not a git repository` and rebuilt nothing, so the sandbox stayed
// broken for every later run.
//
// The rows are the two ways the directory stops being a worktree in the field —
// the repository under review re-cloned or `git worktree prune`d out from under
// it, and the `.git` file truncated by a crash — and the recreation has to cope
// with both: the registration may still name the path, or may not.
func TestASandboxDirectoryThatIsNotAReadableWorktreeIsRecreated(t *testing.T) {
	for name, breaks := range map[string]func(t *testing.T, path string){
		"its .git file is corrupt": func(t *testing.T, path string) {
			t.Helper()
			require.NoError(t, os.WriteFile(filepath.Join(path, ".git"), []byte("not a gitdir\n"), 0o600))
		},
		"its .git file is gone": func(t *testing.T, path string) {
			t.Helper()
			require.NoError(t, os.Remove(filepath.Join(path, ".git")))
		},
	} {
		t.Run(name, func(t *testing.T) {
			src, path, sentinel := sandboxed(t)
			breaks(t, path)

			ready, err := Ensure(src, fixtureLeftoverGlob)
			require.NoError(t, err, "§5.1.6 recreates a sandbox it cannot read rather than refusing to run")

			require.NotNil(t, ready.Recreated, "the sandbox failed a check and was not rebuilt")
			assert.Contains(t, ready.Recreated.Reason, "is not a readable worktree")
			assert.NoFileExists(t, sentinel, "the recreation was reported but never performed")
			assert.Equal(t, src.Head, runGit(t, path, "rev-parse", "HEAD"),
				"the rebuilt sandbox is a worktree at the round's head")

			again, err := Ensure(src, fixtureLeftoverGlob)
			require.NoError(t, err)
			assert.Nil(t, again.Recreated,
				"§5.1.6's recreation has to converge, or every run rebuilds its sandbox for ever")
		})
	}
}

// The negative direction: a sandbox git answers for is left alone.
//
// It is the assertion that keeps the row above from being satisfied by a check
// that rebuilds every sandbox it is handed. A recreation costs a fresh checkout
// and §5.1.3's whole setup, and §5.1.6 spends that only on a sandbox that failed
// a check.
func TestASandboxThatIsStillAWorktreeIsNotRecreated(t *testing.T) {
	src, _, sentinel := sandboxed(t)

	ready, err := Ensure(src, fixtureLeftoverGlob)
	require.NoError(t, err)

	assert.Nil(t, ready.Recreated, "the sandbox was readable and clean")
	assert.FileExists(t, sentinel, "an intact sandbox was rebuilt anyway")
}

// A tracked file §5.1.3's setup already modified, modified again, is named.
//
// This is the case where the set of deviating paths says nothing. `sandbox.setup`
// rewrote `app.txt`, so the baseline already lists it; something has now rewritten
// it a second time, and the two path lists are identical — only the content of the
// deviation differs. A report built from the difference between the lists would be
// empty here, and the reader would be told that tracked files differ from the
// baseline without being told which, on the one occasion where the file involved is
// the least obvious.
//
// gremlins found this: the fallback in `differing` survived CONDITIONALS_NEGATION
// because nothing reached it.
func TestATrackedFileSetupAlreadyTouchedIsNamedWhenItChangesAgain(t *testing.T) {
	dir, head := repository(t)
	scripts := t.TempDir()
	src := sources(t, dir, head)
	src.Setup = []string{script(t, scripts, "setup.sh", "echo touched-by-setup > app.txt\n")}

	created, err := Create(src)
	require.NoError(t, err)
	recorded, err := ReadBaseline(src.Layout, fixtureOwner, fixtureRepo, fixturePR)
	require.NoError(t, err)
	require.Equal(t, []string{"app.txt"}, recorded.Paths,
		"the baseline has to already list the file, or this covers the other branch")

	require.NoError(t, os.WriteFile(
		filepath.Join(created.Path, "app.txt"), []byte("a mutation nobody reverted\n"), 0o600))

	ready, err := Ensure(src, fixtureLeftoverGlob)
	require.NoError(t, err)

	require.NotNil(t, ready.Recreated)
	assert.Contains(t, ready.Recreated.Reason, "app.txt",
		"the one file involved was left out of the report that exists to name it")

	// The rebuild ran setup again, so the sandbox is back at the baseline
	// rather than at HEAD, and the next run has nothing to report.
	again, err := Ensure(src, fixtureLeftoverGlob)
	require.NoError(t, err)
	assert.Nil(t, again.Recreated)
}

// The deviating set can shrink. §5.1.3's setup leaves two tracked files
// modified, something reverts one of them, and the next check compares a
// one-path current state against a two-path baseline — the deviation changed,
// so §5.1.6 owes the reader the path that changed.
//
// gremlins found this, and what it found was a wrong annotation rather than a
// missing assertion. The sum sizing the result was recorded here as an
// equivalent capacity hint; it is not. `len(current)-len(recorded)` goes
// negative as soon as the baseline lists more paths than the sandbox now does,
// and `make` panics with `makeslice: cap out of range` instead of allocating a
// smaller slice. Every fixture that reached `differing` had the two lists at
// the same length, where the difference is zero and nothing shows.
func TestDifferingNamesThePathsWhenTheDeviationSetShrinks(t *testing.T) {
	assert.Equal(t, []string{"b.txt"}, differing([]string{"a.txt"}, []string{"a.txt", "b.txt"}),
		"the path that stopped deviating is the one that changed")
	assert.Equal(t, []string{"b.txt", "c.txt"}, differing(nil, []string{"b.txt", "c.txt"}),
		"a sandbox back at HEAD still names what the baseline listed")
}

// A baseline an earlier cr recorded names no generation, and no run could be
// tied to the sandbox it describes, so an otherwise clean sandbox is recreated
// and the reason says why. What comes back is the recreated sandbox's own
// generation, the one its new baseline records, because every run in it is
// stamped with that value and an empty one would tie the run to nothing.
func TestASandboxWhoseBaselineNamesNoGenerationIsRecreatedWithOne(t *testing.T) {
	src, path, sentinel := sandboxed(t)
	file := src.Layout.PRFile(fixtureOwner, fixtureRepo, fixturePR, state.FileSandboxBaseline)
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	var recorded map[string]any
	require.NoError(t, json.Unmarshal(body, &recorded))
	delete(recorded, "generation")
	body, err = json.Marshal(recorded)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, body, 0o600))

	ready, err := Ensure(src, fixtureLeftoverGlob)

	require.NoError(t, err)
	require.NotNil(t, ready.Recreated, "a baseline naming no generation was admitted")
	assert.Contains(t, ready.Recreated.Reason, "names no sandbox generation")
	assert.NoFileExists(t, sentinel)
	assert.Equal(t, path, ready.Path)
	rebuilt, err := ReadBaseline(src.Layout, src.Owner, src.Repo, src.PR)
	require.NoError(t, err)
	require.NotEmpty(t, rebuilt.Generation)
	assert.Equal(t, rebuilt.Generation, ready.Generation)
}
