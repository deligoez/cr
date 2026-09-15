package sandbox

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// copyAbsent and copyDiffers are what copyStanding finds wrong with one
// `sandbox.copy` entry, and the empty string is an entry that stands.
const (
	copyAbsent  = "absent"
	copyDiffers = "differs"
)

// stale reports why a sandbox that passed §5.1.6's check no longer holds what
// §5.1.2 copies into it, and the empty string when it does.
//
// A worktree is clean against its post-setup baseline whatever became of the
// copied files beside it, and a copied `.env.testing` is exactly the file a
// Laravel suite reads its database from: a sandbox built before `sandbox.copy`
// named it, or before the checkout held it, runs the suite against `.env`.
// So an entry the checkout holds and the sandbox does not makes the sandbox
// stale, and so does a copied regular file whose bytes differ from the
// checkout's. A directory such as `vendor` is compared by presence only, and
// an entry the checkout lacks is not compared at all, so an absent entry stays
// absent rather than rebuilding the sandbox on every run.
//
// The entries §5.1.3's setup changed after copying them are skipped: a sandbox
// the setup itself makes different would otherwise be rebuilt, and its setup
// run again, before every run. The bytes are compared in memory and never
// kept.
func stale(src *Sources, recorded *Baseline) (string, error) {
	path := src.Layout.Sandbox(src.Owner, src.Repo, src.PR)
	for _, rel := range src.Copy {
		if slices.Contains(recorded.SetupChanged, rel) {
			continue
		}
		found, err := copyStanding(filepath.Join(src.RepoDir, rel), filepath.Join(path, rel))
		if err != nil {
			return "", err
		}
		switch found {
		case copyAbsent:
			return fmt.Sprintf("%s names %s, which the checkout %s holds and the sandbox does not",
				src.copyField(), rel, src.RepoDir), nil
		case copyDiffers:
			return fmt.Sprintf("%s names %s, and the checkout %s holds a copy that differs from the sandbox's",
				src.copyField(), rel, src.RepoDir), nil
		}
	}
	return "", nil
}

// copyField names `sandbox.copy` with the profile file it came out of, when
// there is one.
func (src *Sources) copyField() string {
	if src.ProfileFile == "" {
		return "sandbox.copy"
	}
	return "sandbox.copy in " + src.ProfileFile
}

// setupChanged returns the copied entries §5.1.3's setup left standing apart
// from the checkout's, which stale then skips.
func (src *Sources) setupChanged(path string, copied []string) ([]string, error) {
	changed := make([]string, 0, len(copied))
	for _, rel := range copied {
		found, err := copyStanding(filepath.Join(src.RepoDir, rel), filepath.Join(path, rel))
		if err != nil {
			return nil, err
		}
		if found != "" {
			changed = append(changed, rel)
		}
	}
	return changed, nil
}

// copyStanding compares one `sandbox.copy` entry in the checkout with the
// sandbox's: absent when the checkout holds it and the sandbox does not,
// differs when it is a regular file in the checkout and the sandbox's is not
// the same bytes, and empty otherwise. Nothing but an Lstat each is asked of a
// symbolic link or a directory.
func copyStanding(source, target string) (string, error) {
	held, err := os.Lstat(source)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("cannot inspect %s: %w", source, err)
	}
	copied, err := os.Lstat(target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return copyAbsent, nil
	case err != nil:
		return "", fmt.Errorf("cannot inspect %s: %w", target, err)
	}
	if !held.Mode().IsRegular() {
		return "", nil
	}
	if !copied.Mode().IsRegular() || copied.Size() != held.Size() {
		return copyDiffers, nil
	}
	same, err := sameBytes(source, target)
	if err != nil || same {
		return "", err
	}
	return copyDiffers, nil
}

// sameBytes reports whether two regular files hold the same bytes, reading
// both a block at a time.
func sameBytes(first, second string) (bool, error) {
	a, err := os.Open(first)
	if err != nil {
		return false, fmt.Errorf("cannot read %s: %w", first, err)
	}
	defer a.Close()
	b, err := os.Open(second)
	if err != nil {
		return false, fmt.Errorf("cannot read %s: %w", second, err)
	}
	defer b.Close()
	blockA, blockB := make([]byte, 32*1024), make([]byte, 32*1024)
	for {
		n, errA := io.ReadFull(a, blockA)
		m, errB := io.ReadFull(b, blockB)
		if !bytes.Equal(blockA[:n], blockB[:m]) {
			return false, nil
		}
		endA, endB := ended(errA), ended(errB)
		switch {
		case errA != nil && !endA:
			return false, fmt.Errorf("cannot read %s: %w", first, errA)
		case errB != nil && !endB:
			return false, fmt.Errorf("cannot read %s: %w", second, errB)
		case endA || endB:
			return endA == endB, nil
		}
	}
}

// ended reports whether a full read stopped at the end of the file.
func ended(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
