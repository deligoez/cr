package sandbox

import (
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
