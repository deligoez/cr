package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readsOf is everything cr reads out of filesFixture's pull request that
// attributes can change: the file list, the generated answer, and the patch.
type readsOf struct {
	files     []ChangedFile
	generated map[string]bool
	patch     string
}

func readFixture(t *testing.T, dir, base, head string) readsOf {
	t.Helper()
	files, err := ChangedFiles(dir, base, head)
	require.NoError(t, err)
	generated, err := Generated(dir, head, []string{"edited.txt", "api.pb.go"})
	require.NoError(t, err)
	diff, err := DiffAgainstMergeBase(dir, base, head)
	require.NoError(t, err)
	return readsOf{files: files, generated: generated, patch: diff.Patch}
}

// §2.1.1 over a user's own attributes: a global core.attributesFile giving
// edited.txt the diff driver `opaque`, which the same user's configuration
// declares binary, and marking it `linguist-generated` leaves every read cr
// makes exactly as it is with no such file — because the runner pins the
// setting and the repository's attributes alone decide.
//
// The driver is declared binary rather than given a textconv or a command on
// purpose: --no-textconv and --no-ext-diff already neutralise those, and
// `binary = true` is the one driver setting no diff flag overrides. Measured on
// git 2.55.0 with the pin removed: every assertion below fails — the patch reads
// "Binary files /dev/null and b/edited.txt differ", numstat lists edited.txt as
// binary, and edited.txt is reported generated.
//
// The global file is reached through HOME, which the runner inherits on
// purpose for `safe.directory`.
func TestAUsersGlobalAttributesChangeNothingCrReads(t *testing.T) {
	dir, base, head := filesFixture(t)
	t.Setenv("HOME", t.TempDir())
	without := readFixture(t, dir, base, head)

	home := t.TempDir()
	attributes := filepath.Join(home, "attributes")
	require.NoError(t, os.WriteFile(attributes,
		[]byte("edited.txt diff=opaque linguist-generated\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(
		"[core]\n\tattributesFile = "+attributes+"\n[diff \"opaque\"]\n\tbinary = true\n"), 0o600))
	t.Setenv("HOME", home)
	with := readFixture(t, dir, base, head)

	assert.Equal(t, without.files, with.files, "edited.txt stays a text file with a changed line")
	assert.Equal(t, without.generated, with.generated, "edited.txt is not reported generated")
	assert.Equal(t, without.patch, with.patch, "the patch is the same bytes")
	assert.Contains(t, with.patch, "+a line", "and it is the text diff, not a binary notice")
}
