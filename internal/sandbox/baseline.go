package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// Baseline is §5.1.6's post-setup baseline: the sandbox's tracked-file state as
// it stood once §5.1.2's copies and §5.1.3's setup had finished.
//
// It is the thing cleanliness is measured against, and it exists because HEAD
// is not. §5.1.3 runs commands whose job is to change the checkout — `composer
// install` writes a lock file, a build step regenerates a checked-in artefact —
// so a sandbox that is exactly as §5.1 left it already differs from HEAD, and a
// check against HEAD would condemn every sandbox on its first use and recreate
// it without end.
//
// Where it lives is the other half of the design, and round 8's unhomed-state
// finding settled it: this is durable state one `cr` invocation writes and a
// later one reads, so it is a row of §2.3's per-PR table
// (state.FileSandboxBaseline). Keeping it in the sandbox would put it inside a
// worktree of the repository under review, which §2.2 permits cr no write into,
// and would also make the record vanish with the checkout it describes.
type Baseline struct {
	// Head is the revision the sandbox was created at. §5.1.6 checks the
	// sandbox's HEAD against the pull request head, and a baseline
	// recorded at some other head describes a sandbox that is already
	// gone, so the check reads the two together.
	Head string `json:"head"`
	// Diff is the tracked-file deviation from HEAD, kept whole rather than
	// digested. §1.4 defines cr's one hash and internal/text fences the
	// ingredient so no second one can be built, and that hash is the wrong
	// tool here anyway: it normalises the text before digesting it, so a
	// mutation that changed only whitespace would hash to the baseline's
	// value and pass the check. Keeping the bytes compares exactly, and the
	// bytes are bounded by what §5.1.3's setup changed — for a suite whose
	// setup installs into ignored paths, the empty string.
	Diff string `json:"diff"`
	// Paths are the tracked paths that deviated, so a failed check can
	// name files instead of announcing that something differs.
	Paths []string `json:"paths"`
	// Forced is why ForceRecreation condemned the sandbox, and empty for a
	// baseline Create recorded. A file carrying it describes no sandbox:
	// §5.1.6 reads it as one to recreate, and names this as the reason.
	Forced string `json:"forced,omitempty"`
	// SetupChanged are the `sandbox.copy` entries the newly prepared
	// sandbox held apart from the checkout's, by name only: rewritten or
	// created by §5.1.3's setup, or held by the worktree alone. The
	// copied-file check before a run compares them by the checkout's
	// presence only, so a setup that rewrites a copied file does not
	// rebuild the sandbox before every run.
	SetupChanged []string `json:"setup_changed,omitempty"`
	// SetupRemoved are the `sandbox.copy` entries the checkout held and
	// the newly prepared sandbox did not, removed by §5.1.3's setup. The
	// copied-file check before a run asks only that the sandbox still
	// lacks them, so a setup that deletes a copied file does not rebuild
	// the sandbox before every run. A baseline an earlier cr recorded
	// carries none.
	SetupRemoved []string `json:"setup_removed"`
	// Generation names the sandbox this baseline was taken of: the moment
	// Create recorded it. A recreation records a new one, so a run stamped
	// with an earlier generation measured a sandbox that no longer exists,
	// and §5.2.6 does not resolve it as a baseline.
	Generation string `json:"generation,omitempty"`
	// Profile is the id of the profile the sandbox was built under, and
	// points at the empty string when §2.4.4 matched none. It is nil in a
	// baseline an earlier cr recorded, which names no profile at all, so
	// the absence of a record stays apart from the record of no profile.
	Profile *string `json:"profile,omitempty"`
}

// SnapshotBaseline reads the tracked-file state of the sandbox at path and
// records it as the baseline for head.
func SnapshotBaseline(path, head string) (*Baseline, error) {
	tracked, err := git.TrackedState(path)
	if err != nil {
		return nil, err
	}
	return &Baseline{Head: head, Diff: tracked.Patch, Paths: tracked.Paths}, nil
}

// encodeBaseline renders the baseline file: pretty-printed with two-space
// indentation and newline-terminated, as every other JSON document cr writes.
func encodeBaseline(b *Baseline) ([]byte, error) {
	out := *b
	// §12.3: an empty collection serialises as [], never as null.
	out.Paths = append(make([]string, 0, len(b.Paths)), b.Paths...)
	out.SetupRemoved = append(make([]string, 0, len(b.SetupRemoved)), b.SetupRemoved...)
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cannot encode %s: %w", state.FileSandboxBaseline, err)
	}
	return append(body, '\n'), nil
}

// decodeBaseline reads one baseline file back, naming the file when it cannot.
// A baseline cr wrote and cannot now parse is a file something else has been in,
// and §12.4 asks the error to name what to look at.
func decodeBaseline(body []byte, file string) (*Baseline, error) {
	var recorded Baseline
	if err := json.Unmarshal(body, &recorded); err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", file, err)
	}
	return &recorded, nil
}

// recordBaseline publishes the sandbox's post-setup baseline.
//
// The write takes the pull request's exclusive advisory lock, because §2.3.1
// admits no exception: the baseline is per-PR state and is written like the
// rest of it.
func (src *Sources) recordBaseline(path string, entries []string) (string, error) {
	recorded, err := SnapshotBaseline(path, src.Head)
	if err != nil {
		return "", err
	}
	if recorded.SetupChanged, recorded.SetupRemoved, err = src.setupChanged(path, entries); err != nil {
		return "", err
	}
	recorded.Generation = time.Now().UTC().Format(time.RFC3339Nano)
	profile := src.Profile
	recorded.Profile = &profile
	body, err := encodeBaseline(recorded)
	if err != nil {
		return "", err
	}
	return recorded.Generation, src.underLock(func(held *state.Lock) error {
		return held.Write(state.FileSandboxBaseline, body)
	})
}

// invalidateBaseline drops whatever baseline the previous sandbox left behind.
//
// It runs before the worktree is added rather than after setup, and the gap is
// the point. Between the two moments there is a checkout on disk whose
// post-setup state nothing has measured yet, and the honest record of that is
// no record: §5.1.6 reads a missing baseline as a sandbox to recreate, so a run
// that dies partway through §5.1.3 is caught by the same check that catches a
// probe which failed to revert. Leaving the old file in place would instead
// have the next run measure a half-prepared sandbox against a baseline taken
// from a sandbox that no longer exists.
func (src *Sources) invalidateBaseline() error {
	return src.underLock(func(held *state.Lock) error {
		return held.Remove(state.FileSandboxBaseline)
	})
}

// ForceRecreation makes the next run rebuild the sandbox, whatever state it is
// left in, which is what §5.1.7 requires of a probe whose post-run cleanliness
// check failed.
//
// It works by replacing the post-setup baseline with one that says why, rather
// than by removing the worktree, and both halves of that are deliberate.
// Replacing the baseline is enough because §5.1.6 reads a forced one as a
// sandbox to recreate, and it is what makes the forcing unconditional: relying
// on the sandbox still looking dirty next time would be relying on the very
// check that has just proved untrustworthy, and a probe that left an untracked
// file §5.1.6 ignores would pass the next check while §5.1.7 was demanding a
// rebuild. Keeping cause in it is what lets the next run's recreation notice
// name what the probe left behind rather than a baseline that is missing. Not
// removing the worktree here is the other half — §5.1.7 forces the recreation
// "before the next run", and a run that deleted the checkout on its way out
// would take the probe's own output with it before anyone had read it.
func ForceRecreation(src *Sources, cause string) error {
	body, err := encodeBaseline(&Baseline{Head: src.Head, Forced: cause})
	if err != nil {
		return err
	}
	return src.underLock(func(held *state.Lock) error {
		return held.Write(state.FileSandboxBaseline, body)
	})
}

// underLock runs one write against the pull request's state under the exclusive
// advisory lock of §2.3.1, and releases it whether or not the write succeeded.
func (src *Sources) underLock(write func(*state.Lock) error) error {
	held, err := src.Layout.LockPR(src.Owner, src.Repo, src.PR)
	if err != nil {
		return err
	}
	// Joined rather than branched: the lock is released whether or not the
	// write landed, and neither failure is traded away for the other.
	return errors.Join(write(held), held.Unlock())
}
