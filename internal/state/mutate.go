package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SandboxMutation is one file the sandbox is to hold differently for the
// length of a probe.
//
// The change arrives as a function over the file's content rather than as the
// content itself, and that is what keeps the two halves of §5.3.2 in the
// packages that own them. Reading unified diffs is internal/git's, writing
// anywhere on the filesystem is this package's under invariant 2, and neither
// has to learn the other's job: the sandbox path is resolved here, the file is
// read here, and what the patch makes of it is asked of internal/git.
type SandboxMutation struct {
	// Path is the file's path relative to the sandbox root, in slash
	// form, as a diff spells it.
	Path string
	// Apply returns what the file's content becomes. It is
	// git.PatchedFile.Apply for every caller in cr, and refusing is how
	// §5.3.4's first rung — the patch did not apply cleanly — is reached.
	Apply func(content string) (string, error)
}

// OutsideSandboxError reports a path that does not land inside the sandbox.
//
// §5.1.4 forbids cr to modify the user's main worktree, index, or branch, and
// invariant 2 forbids a write inside the repository under review altogether. A
// mutation patch is written by an agent, so `../../app.go` is a path cr will be
// handed sooner or later — and the sandbox is a worktree of the repository, so
// one `..` is all it takes to arrive in the checkout the reviewer is working
// in. The refusal is that invariant enforced rather than assumed.
type OutsideSandboxError struct {
	// Path is the path as the patch spelled it.
	Path string
	// Sandbox is the directory it had to land inside.
	Sandbox string
	// Problem says what is wrong with it.
	Problem string
}

func (e *OutsideSandboxError) Error() string {
	return fmt.Sprintf(
		"%s does not land inside the sandbox at %s: %s; §5.1.4 permits cr no write outside it",
		e.Path, e.Sandbox, e.Problem,
	)
}

// UnderSandboxMutation applies mutations to the pull request's sandbox, runs
// during, and restores every file it touched — on success, on failure, and on
// panic.
//
// This is invariant 6 made structural. §5.3.3 requires the mutation to be
// reverted "even when the run fails, times out, or cr is killed", and a revert
// spelled at the call site is a revert a later edit can return past: one early
// `return err` between the write and the undo leaves a mutated checkout on
// disk, and no test of the successful path notices. So there is no call site to
// get it wrong. applySandboxMutation is unexported and this is its only caller,
// invariant 2's guard already forbids every other package to write a file at
// all, and TestOnlyTheRevertingWrapperMutatesTheSandbox holds both halves —
// which together mean a mutation path that does not revert cannot be written.
//
// The undo runs in a deferred call rather than after during returns, because
// the failures §5.3.3 names arrive differently: a run that fails or times out
// returns an error, and a panic unwinds. A deferred restore answers both, and
// the panic keeps propagating afterwards, so the sandbox is clean and the crash
// is still a crash. The third — cr killed outright — cannot be answered from
// inside the process at all, and §5.3.3's second sentence is what covers it:
// §5.1.6's check finds the unclean sandbox on the next invocation and recreates
// it before anything runs.
//
// The restore's own failure is joined rather than swallowed. A sandbox that
// could not be put back is exactly what the next run has to be told about, and
// losing that to a successful probe would leave a mutation on disk with nothing
// said about it.
func (l Layout) UnderSandboxMutation(
	owner, repo string, pr int, mutations []SandboxMutation, during func() error,
) (err error) {
	restore, err := l.applySandboxMutation(owner, repo, pr, mutations)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, restore()) }()
	return during()
}

// applySandboxMutation rewrites each file and returns the undo for what it
// wrote.
//
// It is unexported, and that is the whole of its design: UnderSandboxMutation
// is its only caller, so there is no way to apply a mutation without having
// already arranged to revert it.
//
// A run that fails partway undoes what it has already done before returning.
// The caller receives no undo in that case, so the obligation is discharged
// here rather than handed back beside an error — which is the shape a caller
// could drop.
func (l Layout) applySandboxMutation(
	owner, repo string, pr int, mutations []SandboxMutation,
) (func() error, error) {
	sandbox := l.Sandbox(owner, repo, pr)
	root, err := filepath.EvalSymlinks(sandbox)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve the sandbox at %s: %w", sandbox, err)
	}

	type restorable struct {
		path    string
		content []byte
		mode    os.FileMode
	}
	written := make([]restorable, 0, len(mutations))
	undo := func() error {
		var failures []error
		// Backwards, so a file mutated twice ends as it began.
		for i := len(written) - 1; i >= 0; i-- {
			if err := os.WriteFile(written[i].path, written[i].content, written[i].mode); err != nil {
				failures = append(failures, fmt.Errorf("cannot restore %s: %w", written[i].path, err))
			}
		}
		return errors.Join(failures...)
	}

	for _, mutation := range mutations {
		target, mode, err := insideSandbox(root, sandbox, mutation.Path)
		if err != nil {
			return nil, errors.Join(err, undo())
		}
		body, err := os.ReadFile(target)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("cannot read %s: %w", target, err), undo())
		}
		mutated, err := mutation.Apply(string(body))
		if err != nil {
			return nil, errors.Join(err, undo())
		}
		written = append(written, restorable{path: target, content: body, mode: mode})
		if err := os.WriteFile(target, []byte(mutated), mode); err != nil {
			return nil, errors.Join(fmt.Errorf("cannot write %s: %w", target, err), undo())
		}
	}
	return undo, nil
}

// insideSandbox resolves one sandbox-relative path and holds it inside the
// sandbox, returning the mode the file is to be written back with.
//
// The containment is checked after the symbolic links are resolved, which is
// the half a lexical check misses: a copied `vendor` tree is full of links, and
// a patch addressing a path through one would land wherever the link points.
// The file has to exist already, because the mutation §5.3.1 describes breaks
// production code that is there — and because a file cr created would have no
// original to put back.
func insideSandbox(root, sandbox, rel string) (path string, mode os.FileMode, err error) {
	refuse := func(problem string) (string, os.FileMode, error) {
		return "", 0, &OutsideSandboxError{Path: rel, Sandbox: sandbox, Problem: problem}
	}
	switch cleaned := filepath.Clean(filepath.FromSlash(rel)); {
	case rel == "" || cleaned == ".":
		return refuse("it names no file")
	case filepath.IsAbs(rel):
		return refuse("it is absolute, and a diff's paths are relative to the tree it applies to")
	case cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)):
		return refuse("it leaves the tree it applies to")
	}

	target := filepath.Join(root, filepath.FromSlash(rel))
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", 0, fmt.Errorf("cannot resolve %s: %w", target, err)
	}
	if !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return refuse("it resolves to " + resolved)
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return "", 0, fmt.Errorf("cannot inspect %s: %w", resolved, err)
	}
	if !info.Mode().IsRegular() {
		return refuse("it is not a regular file")
	}
	return resolved, info.Mode().Perm(), nil
}
