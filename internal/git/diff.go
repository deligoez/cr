package git

import (
	"slices"
	"strings"
)

// diffArgs are the flags every diff carries. They pin the shape of the output
// against configuration and environment that would otherwise change it, so
// §2.1.1 holds on a machine whose owner has never heard of cr.
var diffArgs = []string{
	"diff",
	// An external differ replaces git's output wholesale, and a developer
	// who has one configured has it configured globally.
	"--no-ext-diff",
	// A textconv filter shows a file's converted text, not its content.
	"--no-textconv",
	// Colour is escape codes in the middle of the lines to be parsed.
	"--no-color",
	// `diff.relative` would cut the paths down to the directory git ran
	// in, and §3.4.6 records a unit's paths.
	"--no-relative",
	// The algorithm, the context, and the inter-hunk context decide where
	// hunks begin, end, and merge, and therefore what §3.4.4 clusters.
	"--diff-algorithm=myers",
	"--unified=3",
	"--inter-hunk-context=0",
	// Rename detection at git's own threshold, so a moved file reads as a
	// rename rather than as a whole file the author is asked to review
	// having written none of it. It also pins `diff.renames=copies` back
	// down to renames alone.
	"--find-renames=50%",
	// The prefixes the `a/` and `b/` headers carry, which
	// `diff.noprefix` and `diff.mnemonicPrefix` would otherwise rewrite.
	"--src-prefix=a/",
	"--dst-prefix=b/",
	// Submodules as the plain pointer change they are.
	"--ignore-submodules=none",
	"--submodule=short",
}

// Diff is a pull request's diff against its merge base, per §3.4.1.
type Diff struct {
	// MergeBase is the commit the diff was taken against. It is reported
	// as well as used, because §3.7.1 has cr print the merge base it
	// oriented on.
	MergeBase string
	// Patch is the unified diff from MergeBase to the head revision.
	Patch string
}

// DiffAgainstMergeBase returns the diff of head against the merge base of base
// and head — §3.4.1's "against the PR merge base at the current head".
//
// The merge base is resolved first and then diffed against explicitly, which
// is what `git diff base...head` means with three dots. Doing it in two steps
// is what lets §3.7.1 report the merge base cr actually used: the value
// printed and the value diffed are one commit by construction.
//
// Two dots would be a different thing entirely, and a quiet one. `git diff
// base..head` diffs the two endpoints, so every commit that reached the base
// branch after the pull request branched turns up in the output as the
// author's work — reversed, as deletions of code they never touched. The
// review would then assert things about a colleague's change under someone
// else's name, which is the most expensive way this tool can be wrong.
//
// Both revisions are read, never written: the diff is taken between two
// commits, so it does not consult, refresh, or disturb the user's worktree or
// index (invariant 2).
func DiffAgainstMergeBase(dir, base, head string) (Diff, error) {
	mergeBase, err := MergeBase(dir, base, head)
	if err != nil {
		return Diff{}, err
	}
	args := slices.Clone(diffArgs)
	// --end-of-options keeps a revision that begins with a dash from
	// being read as a flag; the trailing -- says no pathspec follows.
	args = append(args, "--end-of-options", mergeBase, head, "--")
	patch, err := run(dir, args...)
	if err != nil {
		return Diff{}, err
	}
	return Diff{MergeBase: mergeBase, Patch: patch}, nil
}

// MergeBase returns the best common ancestor of base and head.
//
// Two histories with no common ancestor have no merge base, and git reports
// that as a non-zero exit like any other failure, which §3.1.3 turns into
// exit code 3 carrying the command's stderr.
func MergeBase(dir, base, head string) (string, error) {
	out, err := run(dir, "merge-base", "--end-of-options", base, head)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
