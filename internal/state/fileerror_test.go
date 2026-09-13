package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file of §2.3's table a command requires and cannot find is a file failure
// carrying the path and the command that writes it, and still unwraps to the
// filesystem's own answer. The mapping file is the case the acceptance measured:
// `cr record` on a round without mapping.ndjson exited 2, and the step it
// should have named is `cr map record`.
func TestAMissingStateFileNamesTheCommandThatWritesIt(t *testing.T) {
	l := lockedPR(t)
	require.NoError(t, os.Remove(l.PRFile("acme", "web", 42, FileMapping)))

	_, err := l.ReadPR("acme", "web", 42, FileMapping)

	var file *FileError
	require.ErrorAs(t, err, &file)
	assert.Contains(t, err.Error(), l.PRFile("acme", "web", 42, FileMapping))
	assert.Contains(t, file.Hint(), "`cr map record`")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

// Every row of the table a command in this tree writes names that command, and
// a row nothing writes takes the general answer rather than a guessed one.
func TestEveryStateFileHintNamesAWriterOrNone(t *testing.T) {
	for _, name := range prFiles {
		hint := readHint(name)
		writes, known := prFileWriter[name]
		if !known {
			assert.NotContains(t, hint, "is the command that writes it", name)
			continue
		}
		assert.Contains(t, hint, "`"+writes+"`", name)
	}
	assert.Contains(t, readHint(filepath.Join("rounds", "3", FileFindings)), "`cr record`",
		"a per-round path is looked up by its base name")
}

// §2.3's write is a file failure too. A read-only pull-request directory makes
// the temporary file impossible to create while every read still succeeds,
// which isolates the write: measured before this type, `cr record` there exited
// 2 with a bare `cannot create a temporary file in <dir>`.
func TestAWriteThatCannotLandIsAFileFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes through a read-only directory")
	}
	l := lockedPR(t)
	held, err := l.LockPR("acme", "web", 42)
	require.NoError(t, err)
	defer func() { assert.NoError(t, held.Unlock()) }()

	dir := l.PRDir("acme", "web", 42)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err = held.Write(FileFindings, []byte("{}\n"))

	var file *FileError
	require.ErrorAs(t, err, &file)
	assert.Equal(t, writeHint, file.Hint())
	assert.ErrorIs(t, err, fs.ErrPermission)
}

// §12.4: a file failure cannot be built without its next actionable step.
func TestAFileFailureWithoutAHintCannotBeBuilt(t *testing.T) {
	assert.Panics(t, func() { _ = FileFailure("read", "x", "", errors.New("boom")) })
	assert.NotPanics(t, func() { _ = FileFailure("read", "x", "check x", nil) })
}

// The message names what the filesystem reported when it reported something,
// and ends at the path when it did not: a file read whole and unusable as
// written has no cause beneath it, and a message ending in "<nil>" would print
// one.
func TestAFileFailureNamesItsCauseOnlyWhenThereIsOne(t *testing.T) {
	assert.Equal(t, "cannot read x: boom", FileFailure("read", "x", "check x", errors.New("boom")).Error())
	assert.Equal(t, "cannot read x", FileFailure("read", "x", UnusableHint, nil).Error())
}
