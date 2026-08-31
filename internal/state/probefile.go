package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ProbeFileExistsError reports a file already standing where §5.4.2 places a
// gap probe's test.
//
// §5.4.2 has the placement abort rather than write over it, and the reason is
// the removal in the same sentence: cr places the test, runs it, and removes it
// again. Overwriting a file that was already there would therefore destroy it
// and then delete it, and the file most likely to be standing at a path the
// profile's `tests.globs` point into is a real test.
//
// §5.1.6 cannot catch this case, which is why the abort is here. Its leftover
// scan is scoped to untracked files, so a tracked test file at the probe path —
// or one `sandbox.copy` or `sandbox.setup` puts there — passes the cleanliness
// check and reaches the placement intact.
type ProbeFileExistsError struct {
	// Path is the sandbox-relative path the template resolved to.
	Path string
	// Sandbox is the worktree it was resolved inside.
	Sandbox string
}

func (e *ProbeFileExistsError) Error() string {
	return fmt.Sprintf(
		"%s already exists in the sandbox at %s: §5.4.2 places a gap probe's test there and removes it "+
			"again, so cr would destroy the file and then delete it; "+
			"point tests.probe_path_template at a path nothing else writes",
		e.Path, e.Sandbox,
	)
}

// UnderSandboxTestFile places a gap probe's test file in the pull request's
// sandbox, runs during, and removes it — on success, on failure, and on panic.
//
// This is invariant 6's other half, and it is a second door rather than a reuse
// of UnderSandboxMutation because the two obligations are opposites.
// UnderSandboxMutation rewrites a file that has to be there and puts its
// content back; §5.4.2 creates a file that has to *not* be there and takes it
// away again. Sharing one door would mean a single undo that has to decide
// which of the two it is, and the decision would sit inside the one function
// invariant 6 exists to leave no room for a decision in.
//
// What is shared is the shape, and deliberately: applySandboxTestFile is
// unexported with this as its only caller, invariant 2's guard already forbids
// every other package to write a file at all, and
// TestOnlyTheRevertingWrappersTouchTheSandbox holds both doors to the same
// three conditions. So a placement that does not remove cannot be written.
//
// The undo runs in a deferred call rather than after during returns, because
// the failures §5.4.2 names arrive differently: a run that fails or times out
// returns an error, and a panic unwinds. A deferred removal answers both, and
// the panic keeps propagating afterwards. The one failure left is cr killed
// outright, which §5.1.6 answers on the next invocation — a leftover file under
// the template's glob is exactly what it scans for.
//
// The removal's own failure is joined rather than swallowed, for the reason the
// restore's is: a probe file that could not be taken off disk is what the next
// run has to be told about.
func (l Layout) UnderSandboxTestFile(
	owner, repo string, pr int, rel, content string, during func() error,
) (err error) {
	remove, err := l.applySandboxTestFile(owner, repo, pr, rel, content)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, remove()) }()
	return during()
}

// applySandboxTestFile writes the test file and returns the undo for what it
// wrote.
//
// It is unexported, and that is the whole of its design: UnderSandboxTestFile
// is its only caller, so there is no way to place a probe file without having
// already arranged to remove it.
//
// A placement that fails partway undoes what it has already done before
// returning. The caller receives no undo in that case, so the obligation is
// discharged here rather than handed back beside an error — which is the shape
// a caller could drop.
func (l Layout) applySandboxTestFile(
	owner, repo string, pr int, rel, content string,
) (func() error, error) {
	target, created, err := l.freeInSandbox(owner, repo, pr, rel)
	if err != nil {
		return nil, err
	}
	remove := func() error {
		var failures []error
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			failures = append(failures, fmt.Errorf("cannot remove %s: %w", target, err))
		}
		// The directories the placement had to make go with it. They
		// were not there before, so §5.1.6's next check finds the
		// sandbox as the post-setup baseline left it rather than
		// carrying an empty tree cr created and nobody removed.
		if created != "" {
			if err := os.RemoveAll(created); err != nil {
				failures = append(failures, fmt.Errorf("cannot remove %s: %w", created, err))
			}
		}
		return errors.Join(failures...)
	}

	if created != "" {
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return nil, errors.Join(
				fmt.Errorf("cannot create %s: %w", filepath.Dir(target), err), remove())
		}
	}
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		return nil, errors.Join(fmt.Errorf("cannot write %s: %w", target, err), remove())
	}
	return remove, nil
}

// FreeInSandbox resolves the sandbox path §5.4.2 places a gap probe's test at,
// and refuses one that is taken or that does not land inside the sandbox.
//
// It is exported so a caller can refuse before running anything, which is what
// round 12's unbounded-patch-target finding asks of the mutation side and what
// §5.4.2's abort asks of this one: a probe path that is already occupied is
// refused as the situation it is, rather than after §5.2.6 has performed a
// whole baseline suite for it. It is the same function the writer calls, so the
// answer a caller gets ahead of the run and the answer the write is held to
// cannot come apart.
func (l Layout) FreeInSandbox(owner, repo string, pr int, rel string) (string, error) {
	path, _, err := l.freeInSandbox(owner, repo, pr, rel)
	return path, err
}

// freeInSandbox resolves rel inside the sandbox for a file that is not there
// yet, and reports the outermost directory the placement would have to create.
//
// The containment is checked against the deepest ancestor that does exist,
// after its symbolic links are resolved. A path whose leaf is absent cannot be
// resolved as a whole, and a lexical check alone would miss what the mutation
// side's resolution catches: a copied tree full of links, one of which points
// out of the sandbox.
func (l Layout) freeInSandbox(
	owner, repo string, pr int, rel string,
) (path, created string, err error) {
	box := l.Sandbox(owner, repo, pr)
	refuse := func(problem string) (string, string, error) {
		return "", "", &OutsideSandboxError{Path: rel, Sandbox: box, Problem: problem}
	}
	switch cleaned := filepath.Clean(filepath.FromSlash(rel)); {
	case rel == "" || cleaned == ".":
		return refuse("it names no file")
	case filepath.IsAbs(rel):
		return refuse("it is absolute, and tests.probe_path_template is relative to the sandbox")
	case cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)):
		return refuse("it leaves the tree it is placed in")
	}

	root, err := filepath.EvalSymlinks(box)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve the sandbox at %s: %w", box, err)
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	switch _, err := os.Lstat(target); {
	case err == nil:
		return "", "", &ProbeFileExistsError{Path: filepath.ToSlash(rel), Sandbox: box}
	case !errors.Is(err, fs.ErrNotExist):
		return "", "", fmt.Errorf("cannot inspect %s: %w", target, err)
	}

	// Upwards to the first directory that is there. Everything passed on
	// the way is what the placement creates and the removal takes back,
	// and the loop terminates at the sandbox root at the latest, because
	// the cleaned path above cannot leave it.
	existing := filepath.Dir(target)
	for {
		_, err := os.Stat(existing)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", fmt.Errorf("cannot inspect %s: %w", existing, err)
		}
		created = existing
		existing = filepath.Dir(existing)
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve %s: %w", existing, err)
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return refuse("its nearest existing directory resolves to " + resolved)
	}
	return target, created, nil
}
