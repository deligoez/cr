package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"

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
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cannot encode %s: %w", state.FileSandboxBaseline, err)
	}
	return append(body, '\n'), nil
}

// recordBaseline publishes the sandbox's post-setup baseline.
//
// The write takes the pull request's exclusive advisory lock, because §2.3.1
// admits no exception: the baseline is per-PR state and is written like the
// rest of it.
func (src *Sources) recordBaseline(path string) error {
	recorded, err := SnapshotBaseline(path, src.Head)
	if err != nil {
		return err
	}
	body, err := encodeBaseline(recorded)
	if err != nil {
		return err
	}
	return src.underLock(func(held *state.Lock) error {
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
