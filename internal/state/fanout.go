package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// DirFanOut is the directory inside one pull request's state directory that
// §4.6.2's output files sit in: one subdirectory per round, and in it one per
// unit, each holding the file every role reviewing that unit writes to.
//
// It sits beside the §2.3 table rather than inside rounds/<n>/, because what it
// holds is the agent's and not cr's. rounds/<n>/ is fenced to the four
// artefacts §2.3 names, which cr writes; these are files a role writes and
// `cr merge` reads, and §4.6.3 has `cr review` write none of them. A directory
// per unit is what lets the roles of one round run side by side: two prompts for
// the same role over different units name different files, so neither appends
// into a file the other is writing.
const DirFanOut = "fanout"

// fanOutDir is one unit's fan-out directory of one round, relative to the pull
// request's state directory.
func fanOutDir(round int, unit string) string {
	return filepath.Join(DirFanOut, strconv.Itoa(round), unit)
}

// FanOutDir is the directory §4.6.2's output files for one unit of one round
// sit in (§2.2: under the pull request's state directory, never in the
// repository under review).
func (l Layout) FanOutDir(owner, repo string, pr, round int, unit string) string {
	return filepath.Join(l.PRDir(owner, repo, pr), fanOutDir(round, unit))
}

// FannedOut reports whether one unit of one round has its fan-out directory,
// which `cr review` creates for every unit of the round before it emits a
// prompt, so a round no `cr review` has emitted prompts for has none. It takes
// no lock, per §2.3.2.
func (l Layout) FannedOut(owner, repo string, pr, round int, unit string) (bool, error) {
	dir := l.FanOutDir(owner, repo, pr, round, unit)
	_, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, FileFailure("read", dir, dirHint, err)
	}
	return true, nil
}

// EnsureFanOut creates one round's fan-out directory for every unit named, so
// a role can write its file without having to create a directory under the
// state root itself. It creates directories and no file, and a directory
// already there is left as it is, so emitting a round's prompts twice discards
// nothing a role has written.
//
// A unit id that is not a single path segment is refused rather than joined:
// §3.4.6 forms every id as u<n>, and an id that could climb out of its round's
// directory is not one cr recorded.
func (k *Lock) EnsureFanOut(round int, units []string) error {
	if err := checkRound(round); err != nil {
		return err
	}
	dirs := make([]string, 0, len(units))
	for _, unit := range units {
		if unit == "" || unit == "." || unit == ".." || filepath.Base(unit) != unit {
			return fmt.Errorf("unit id %q is not a single path segment, and §3.4.6 forms every unit id as u<n>", unit)
		}
		dirs = append(dirs, filepath.Join(k.dir, fanOutDir(round, unit)))
	}
	return makeDirs(dirs)
}
