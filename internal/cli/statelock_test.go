package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// repoRefusal is the message splitRepo refuses slug with.
func repoRefusal(slug string) string {
	return fmt.Sprintf("invalid repository %q: pass it as owner/repo, "+
		"where neither half is . or .. or holds a separator", slug)
}

// badSlugs are `--repo` values whose halves are not each one path segment.
var badSlugs = map[string]string{
	"both halves dot-dot":              "../..",
	"a repository of dot-dot":          "acme/..",
	"an owner of dot":                  "./api",
	"a repository holding a separator": "acme/api/extra",
	"a repository holding a backslash": `acme/a\pi`,
}

// entryNames lists the names in dir.
func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// §11.1's `--repo` names one owner and one repository, and §2.2 makes each a
// directory under the state root, so a half that is not one path segment is a
// malformed command line: §11.2's code 2, refused before any state is read or
// written. Audit round 1 measured `--repo ../..` reaching the state package,
// where only a missing round stopped it.
//
// The directory the root sits in is listed afterwards, because `../..` from
// the state directory is exactly that directory: a pull request's state landing
// there is what the refusal is for.
func TestRecordRefusesARepositoryThatIsNotTwoPathSegments(t *testing.T) {
	for name, slug := range badSlugs {
		t.Run(name, func(t *testing.T) {
			layout := recordedHome(t)
			file := writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"))

			_, err := runRecord(t, recordPR, file, "--repo", slug)

			require.Error(t, err)
			assert.Equal(t, repoRefusal(slug), err.Error())
			assert.Equal(t, ExitUsage, exitCodeFor(err))
			assert.Equal(t, usageHint, hintFor(err))
			assert.Equal(t, []string{".cr"}, entryNames(t, filepath.Dir(layout.Root())),
				"nothing is created beside the state root")
			assert.Empty(t, recordStore(t, layout))
		})
	}
}

// `cr config --repo` reads the per-repository layer of §2.7 through the same
// argument, and refuses the same values.
func TestConfigRefusesARepositoryThatIsNotTwoPathSegments(t *testing.T) {
	for name, slug := range badSlugs {
		t.Run(name, func(t *testing.T) {
			crHome(t)
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"config", "--repo", slug})

			err := cmd.Execute()

			require.Error(t, err)
			assert.Equal(t, repoRefusal(slug), err.Error())
			assert.Equal(t, ExitUsage, exitCodeFor(err))
		})
	}
}

// §2.3.1's lock through `cr record`: a lock that cannot be taken is a file
// failure, §11.2's code 3 with a step that names the file, and not the usage
// error audit round 1 measured — exit 2 telling the user to retype a command
// line that was right. Nothing is recorded.
//
// The two cases fail LockPR at its two steps: a file where the repository's
// lock directory goes stops the directory creation, and a lock file nobody may
// open stops the flock with every directory already there.
func TestRecordWhoseLockCannotBeTakenExitsWithTheFileCode(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens a file whatever its mode")
	}
	for name, tc := range map[string]struct {
		block func(t *testing.T, layout state.Layout)
		hint  string
	}{
		"a file where the lock directory goes": {
			block: func(t *testing.T, layout state.Layout) {
				dir := filepath.Dir(layout.PRLockFile(recordOwner, recordRepo, recordPRNum))
				require.NoError(t, os.RemoveAll(dir))
				require.NoError(t, os.WriteFile(dir, nil, 0o600))
			},
			hint: "§2.2 keeps all of cr's state under one root; check that every parent of " +
				"the directory the message names is a directory cr can write to",
		},
		"a lock file nobody may open": {
			block: func(t *testing.T, layout state.Layout) {
				require.NoError(t, os.Chmod(layout.PRLockFile(recordOwner, recordRepo, recordPRNum), 0o000))
			},
			hint: "cr locks through a file under the state root's locks directory; " +
				"check that the path the message names is a regular file cr can open for writing",
		},
	} {
		t.Run(name, func(t *testing.T) {
			layout := recordedHome(t)
			tc.block(t, layout)
			file := writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"))

			_, err := runRecord(t, recordPR, file, "--repo", recordSlug)

			var failure *state.FileError
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, tc.hint, hintFor(err))
			assert.Empty(t, recordStore(t, layout))
		})
	}
}

// §5.6.2's timeout names one step, and it is the same one on both sides: the
// message leaves the step to the table, and the table's row prints
// state.ProbeLockedHint. Audit round 1 found the message sending the user to
// raise the setting "with `cr config`", which only prints, beside a row that
// said to wait.
func TestAHeldProbeLockNamesOneStep(t *testing.T) {
	locked := &state.ProbeLockedError{
		RepoPath: "/src/acme/web", ProfileID: "laravel-pest", Waited: 300 * time.Second,
	}
	wrapped := fmt.Errorf("running the suite: %w", locked)

	assert.Equal(t, state.ProbeLockedHint, hintFor(wrapped))
	assert.Equal(t,
		"wait for the other cr run on this repository and profile to finish, then run the command again",
		hintFor(wrapped))
	assert.Equal(t,
		"another cr run holds the probe lock for /src/acme/web under profile laravel-pest: "+
			"waited 5m0s, which is probe.lock_timeout_seconds",
		locked.Error())
}

// A path internal/state refuses as outside its root is answered by naming the
// repository on the command line, which §11.2 codes 2 with a step of its own
// rather than the general usage hint.
func TestAPathOutsideTheStateRootIsAUsageErrorNamingTheFlag(t *testing.T) {
	outside := fmt.Errorf("locking: %w", &state.OutsideRootError{Path: "/x/pr-1", Under: "/x/.cr/state"})

	assert.Equal(t, ExitUsage, exitCodeFor(outside))
	assert.Equal(t, "pass --repo <owner/repo>, where neither half is . or .. or holds a separator",
		hintFor(outside))
}
