package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// Recreated is §5.1.6's recreation notice: a sandbox failed the cleanliness
// check, so it was rebuilt, and the reader is told.
//
// It satisfies finding.HonestyDisclosure, which is what §11.1 exempts from
// `--quiet`. The exemption matters here more than the wording does: a run whose
// sandbox was silently rebuilt is a run whose earlier probe left something
// behind, or whose HEAD moved under it, and a reader who is not told reads the
// results as if nothing had happened.
type Recreated struct {
	// Path is the sandbox that was rebuilt.
	Path string `json:"path"`
	// Reason is what the check found, in the words the check found it in.
	Reason string `json:"reason"`
}

// Disclosure is the text printed whatever the flags say. It is derived from the
// two fields, so what a reader is told and what a caller reads as data cannot
// drift apart.
func (r Recreated) Disclosure() string {
	return "sandbox " + r.Path + " recreated, per §5.1.6: " + r.Reason
}

// Ready is a sandbox §5.1.6 admits to a probe or test run.
type Ready struct {
	// Path is the sandbox to run in.
	Path string
	// Recreated is the notice, and is nil when the sandbox passed the
	// check as it stood. A caller that finds one MUST report it.
	Recreated *Recreated
	// Stopped is the runner an earlier cr run left alive in the sandbox
	// and this check killed, and is nil when there was none. A caller that
	// finds one reports it too.
	Stopped *StoppedRunner
}

// Ensure returns a sandbox fit to run in, rebuilding the one on disk when it is
// not (§5.1.6).
//
// leftoverGlob is the profile's `tests.probe_path_template` with its
// `<probe-id>` position replaced by `*`, which profile.LeftoverGlob produces. An
// empty glob is a profile with no test axis, and therefore nothing to scan for
// rather than a pattern matching everything.
//
// The recreation is returned rather than logged. §11.1 exempts this notice from
// `--quiet`, and an exemption belongs to the writer that prints it, so what this
// hands back is the disclosure and not a line on a stream.
func Ensure(src *Sources, leftoverGlob string) (*Ready, error) {
	path := src.Layout.Sandbox(src.Owner, src.Repo, src.PR)
	// A runner an earlier cr left alive is stopped before the check reads
	// the sandbox, since it may still be writing to it, and before a
	// recreation or a run of this command's own starts beside it.
	stopped, err := stopLeftRunner(src)
	if err != nil {
		return nil, err
	}
	reason, err := Unclean(src, leftoverGlob)
	if err != nil {
		return nil, err
	}
	// A sandbox clean against its baseline can still lack a file §5.1.2
	// copies, or hold an older copy of one. The check runs here and not in
	// Unclean, because §5.1.7's post-run check reads Unclean and a run does
	// not make a copied file stale.
	if reason == "" {
		recorded, err := ReadBaseline(src.Layout, src.Owner, src.Repo, src.PR)
		if err != nil {
			return nil, err
		}
		if reason, err = stale(src, recorded); err != nil {
			return nil, err
		}
	}
	if reason == "" {
		return &Ready{Path: path, Stopped: stopped}, nil
	}
	if err := recreate(src, path); err != nil {
		return nil, err
	}
	return &Ready{Path: path, Recreated: &Recreated{Path: path, Reason: reason}, Stopped: stopped}, nil
}

// Unclean reports why the pull request's sandbox cannot be run in, and the
// empty string when it can. It is §5.1.6's check, without §5.1.6's recreation.
//
// The two halves are separate because §5.1.7 needs the first without the
// second. Its post-run check runs "before the probe record is written", and
// what it decides is what the record says — rebuilding the sandbox at that
// moment would destroy the evidence the check just found, and the recreation
// §5.1.7 requires is "before the next run" rather than before the record.
// Ensure is the caller that wants both, and it is the one that runs before a
// run rather than after one.
//
// The order is cheapest-first and, more importantly, most-fundamental-first: a
// sandbox that is not there cannot have a HEAD read, and a HEAD that moved makes
// every later comparison a comparison against the wrong code. The first answer
// found is the one reported, because a reader acts on the first thing that is
// wrong and the rest are consequences of it.
func Unclean(src *Sources, leftoverGlob string) (string, error) {
	path := src.Layout.Sandbox(src.Owner, src.Repo, src.PR)
	switch _, err := os.Stat(path); {
	case errors.Is(err, fs.ErrNotExist):
		return "there is no sandbox at that path", nil
	case err != nil:
		return "", fmt.Errorf("cannot inspect %s: %w", path, err)
	}

	at, err := git.Head(path)
	if err != nil {
		return "", err
	}
	if at != src.Head {
		return fmt.Sprintf("the sandbox is at %s, and the round's head is %s", at, src.Head), nil
	}

	recorded, err := ReadBaseline(src.Layout, src.Owner, src.Repo, src.PR)
	if err != nil {
		return "", err
	}
	if recorded == nil {
		return "no post-setup baseline was recorded for it", nil
	}
	if recorded.Forced != "" {
		return "§5.1.7 forced its recreation: " + recorded.Forced, nil
	}
	// A baseline taken at some other head describes a sandbox that is
	// already gone, whatever this one now holds.
	if recorded.Head != src.Head {
		return fmt.Sprintf(
			"its post-setup baseline was taken at %s, and the round's head is %s", recorded.Head, src.Head), nil
	}

	current, err := git.TrackedState(path)
	if err != nil {
		return "", err
	}
	if current.Patch != recorded.Diff {
		return "tracked files differ from the post-setup baseline: " +
			strings.Join(differing(current.Paths, recorded.Paths), ", "), nil
	}

	leftover, err := leftoverArtefacts(path, leftoverGlob)
	if err != nil {
		return "", err
	}
	if len(leftover) > 0 {
		return "a probe artefact was left behind: " + strings.Join(leftover, ", "), nil
	}
	return "", nil
}

// differing names the tracked files to report when the deviation from HEAD has
// changed.
//
// The paths present in one list and not the other are the ones that gained or
// lost a deviation, and they are what a reader wants first. When both lists
// agree the change is inside a file that was already deviating — §5.1.3's setup
// touched it and something has touched it again — so the shared paths are
// reported instead, rather than an empty list saying that something, somewhere,
// differs.
func differing(current, recorded []string) []string {
	// The sum is not only a capacity hint: `make` panics on a negative
	// capacity, and a difference is negative as soon as the baseline lists
	// more paths than the sandbox now does, which is what a reverted
	// deviation leaves behind. Measured, and asserted rather than annotated.
	changed := make([]string, 0, len(current)+len(recorded))
	for _, path := range current {
		if !slices.Contains(recorded, path) {
			changed = append(changed, path)
		}
	}
	for _, path := range recorded {
		if !slices.Contains(current, path) {
			changed = append(changed, path)
		}
	}
	if len(changed) == 0 {
		changed = append(changed, current...)
	}
	slices.Sort(changed)
	return slices.Compact(changed)
}

// leftoverArtefacts returns the untracked files under the leftover glob, which
// is §5.1.6's second half.
//
// Untracked is the whole of the scoping, and round 12's
// artefact-glob-not-scoped-to-untracked finding is why it is spelled out. A
// `tests.probe_path_template` is required to sit where the profile's
// `tests.globs` point, so its glob can perfectly well match a test file the
// repository tracks — and a check that counted one would find a leftover
// artefact in every sandbox, on every run, for ever: the recreation §5.1.6
// mandates would never converge, because recreating restores the tracked file
// it is objecting to.
//
// The other untracked files are ignored, per §5.1.6, because `sandbox.copy` and
// `sandbox.setup` create them by design.
func leftoverArtefacts(path, leftoverGlob string) ([]string, error) {
	if leftoverGlob == "" {
		return nil, nil
	}
	// The sandbox path is derived by internal/state from an owner, a
	// repository and a number, so it contributes no glob metacharacter of
	// its own; every wildcard in the joined pattern comes from the profile.
	matched, err := filepath.Glob(filepath.Join(path, filepath.FromSlash(leftoverGlob)))
	if err != nil {
		return nil, fmt.Errorf("cannot scan %s for a leftover probe artefact: %w", leftoverGlob, err)
	}
	relative := make([]string, 0, len(matched))
	for _, found := range matched {
		within, err := filepath.Rel(path, found)
		if err != nil {
			return nil, fmt.Errorf("cannot place %s inside %s: %w", found, path, err)
		}
		relative = append(relative, filepath.ToSlash(within))
	}
	tracked, err := git.TrackedAmong(path, relative)
	if err != nil {
		return nil, err
	}
	untracked := make([]string, 0, len(relative))
	for _, within := range relative {
		if !tracked[within] {
			untracked = append(untracked, within)
		}
	}
	return untracked, nil
}

// recreate rebuilds the sandbox, which is what §5.1.6 requires of a failed
// check.
//
// The removal comes first and is §5.1.5's, because Create refuses to build over
// a sandbox that is there — it does not delete a checkout it did not just make,
// and this is the one place that decision is taken deliberately rather than
// inherited.
func recreate(src *Sources, path string) error {
	if _, err := removeSandbox(src, path); err != nil {
		return err
	}
	_, err := Create(src)
	return err
}

// ReadBaseline reads §5.1.6's post-setup baseline, and reports a pull request
// that has none as a nil baseline rather than as a failure.
//
// The absence is a real state and not an error: it is what a pull request no
// sandbox has been prepared for looks like, and what Create leaves behind when
// it dies between invalidating the old baseline and recording the new one.
// §5.1.6 answers both the same way — recreate — so the caller reads it as a
// value.
//
// The read takes no lock, per §2.3.2.
func ReadBaseline(l state.Layout, owner, repo string, pr int) (*Baseline, error) {
	body, err := l.ReadPR(owner, repo, pr, state.FileSandboxBaseline)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return decodeBaseline(body, l.PRFile(owner, repo, pr, state.FileSandboxBaseline))
}
