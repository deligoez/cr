package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/deligoez/cr/internal/state"
)

// CheckRequired is §5.1.8: after §5.1.6's check and before any run, `cr test`
// and `cr probe run` abort when a `sandbox.require` path does not exist in the
// sandbox, naming the path and `sandbox.copy`.
//
// It exists because a suite that cannot find a file does not usually stop. It
// falls back — a Laravel suite with no `.env.testing` reads `.env` instead, and
// in the field trial spec/field-feedback.md item 2.1 records, that was the
// developer's own application database. The run then finishes, reports a
// result, and every probe graded against it measured something nobody asked
// for. §5.1.2 already reports the gitignored `.env*` files the sandbox did not
// receive; this is the half a profile can make binding, for the files a
// particular repository's suite genuinely cannot run without.
//
// The step names `sandbox.copy` because that is the field that would put the
// file there, and `sandbox.require` because dropping the entry is the other
// honest answer. §11.2 codes it 3: the command line is right, and what refuses
// is the configuration together with what the sandbox holds.
//
// Nothing is opened, and an Lstat is the whole check: a symbolic link a
// `sandbox.copy` reproduced counts as held whether or not it resolves, exactly
// as §5.1.2's comparison counts it.
func CheckRequired(src *Sources, path string) error {
	for _, rel := range src.Require {
		if err := entryPath(src.ProfileFile, requireField, rel); err != nil {
			return err
		}
		held := filepath.Join(path, rel)
		switch _, err := os.Lstat(held); {
		case err == nil:
			continue
		case !errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("cannot inspect %s: %w", held, err)
		}
		return state.FileFailure("run the suite without", held,
			fmt.Sprintf(
				"add %s to %s so the sandbox receives it, or drop it from %s",
				rel, src.copyField(), src.requireField()),
			fmt.Errorf("%s names it and the sandbox does not hold it", src.requireField()))
	}
	return nil
}

// requireField is the dotted §2.4 name, spelled once.
const requireField = "sandbox.require"

// requireField names `sandbox.require` with the profile file it came out of,
// when there is one, as copyField does for `sandbox.copy`.
func (src *Sources) requireField() string {
	if src.ProfileFile == "" {
		return requireField
	}
	return requireField + " in " + src.ProfileFile
}
