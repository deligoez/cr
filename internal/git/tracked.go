package git

import (
	"slices"
	"strings"
)

// Tracked is one worktree's tracked-file state: how its working tree stands
// against its own HEAD, and which tracked paths that answer is about.
//
// §5.1.6 measures cleanliness against a recorded baseline rather than against
// HEAD, because §5.1.3's setup commands legitimately modify tracked files —
// `composer install` rewrites a lock file, a build step regenerates a
// checked-in artefact. So what a baseline has to capture is the deviation
// itself, exactly enough that a later deviation can be told apart from it.
type Tracked struct {
	// Patch is the deviation of the working tree from HEAD, as a unified
	// diff. It is compared byte for byte: two reads of the same
	// tracked-file state produce the same bytes, and any tracked file
	// that gained, lost, or changed one changes them.
	Patch string
	// Paths are the tracked paths the patch touches. They carry nothing
	// the patch does not already hold and exist so a report can name the
	// files rather than announce that something, somewhere, differs.
	Paths []string
}

// TrackedState returns the tracked-file state of the worktree at dir.
//
// `--binary` is what the patch read carries and §3.4.1's diff must not. Without
// it git writes `Binary files a/x and b/x differ` for a non-text file, which
// says that the file changed and never says to what — so a probe that replaced
// one binary with another would leave a patch identical to the baseline's and
// pass the cleanliness check. §3.4.1's diff is read by an agent and wants the
// summary line; a comparison wants the bytes.
//
// It is two reads rather than one because the paths are asked of git instead of
// parsed back out of the patch. A `diff --git a/x b/y` header quotes a path
// holding a space, a quote, or a newline, and a parser that got that wrong
// would misname the file in the one report a reader acts on; `--name-only -z`
// hands the same paths over NUL-separated and unquoted.
//
// Nothing here writes. `git diff` against HEAD reads the working tree and the
// index and refreshes neither — run pins GIT_OPTIONAL_LOCKS=0, so the
// opportunistic index write git would otherwise make does not happen — and the
// sandbox is a worktree of the repository under review, where §2.2 permits cr
// no write but the registration §5.1.1 already made.
//
// The two argument lists are spelled out rather than built by a helper because
// the subcommand has to stay readable from the source: TestEveryGitSubcommand-
// CrRunsIsARead resolves a run's first argument back to a literal, and a list
// arriving from a function call is a subcommand named at run time.
func TrackedState(dir string) (Tracked, error) {
	patchArgs := slices.Clone(diffArgs)
	// --end-of-options and the trailing -- are the two guards
	// DiffAgainstMergeBase carries: nothing here comes from a user, but a
	// revision read as a flag and a pathspec read as a revision are
	// mistakes worth being unable to make.
	patchArgs = append(patchArgs, "--binary", "--end-of-options", "HEAD", "--")
	patch, err := run(dir, patchArgs...)
	if err != nil {
		return Tracked{}, err
	}

	pathArgs := slices.Clone(diffArgs)
	pathArgs = append(pathArgs, "--name-only", "-z", "--end-of-options", "HEAD", "--")
	listing, err := run(dir, pathArgs...)
	if err != nil {
		return Tracked{}, err
	}
	return Tracked{Patch: patch, Paths: splitNUL(listing)}, nil
}

// Head is the revision the worktree at dir is checked out at.
//
// §5.1.6 compares it against the round's head before every probe or test run,
// because a sandbox at some other revision is a sandbox whose results belong to
// code no finding of this round was written against. `--end-of-options` is left
// off deliberately: rev-parse echoes an argument it does not recognise back on
// standard output, so the guard other reads carry would become a second line of
// output here. The revision is the literal HEAD and comes from no caller.
func Head(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// TrackedAmong returns which of paths the worktree at dir tracks.
//
// §5.1.6's leftover-artefact scan is evaluated over untracked files only, and
// this is the half that decides which those are. Round 12's
// artefact-glob-not-scoped-to-untracked finding is why it is asked at all: a
// `tests.probe_path_template` whose glob also matches a tracked test file would
// otherwise make every sandbox permanently unclean, and §5.1.6's mandated
// recreation unbounded.
//
// The paths are handed over as `:(literal)` pathspecs, so a name holding a `*`
// or a `[` is matched as itself rather than as a pattern of git's own.
func TrackedAmong(dir string, paths []string) (map[string]bool, error) {
	if len(paths) == 0 {
		return map[string]bool{}, nil
	}
	args := []string{"ls-files", "-z", "--full-name", "--"}
	for _, path := range paths {
		args = append(args, ":(literal)"+path)
	}
	listing, err := run(dir, args...)
	if err != nil {
		return nil, err
	}
	tracked := make(map[string]bool, len(paths))
	for _, path := range splitNUL(listing) {
		tracked[path] = true
	}
	return tracked, nil
}

// splitNUL cuts a NUL-terminated git listing into its records. The final NUL is
// a terminator rather than a separator, so an empty listing is no records and
// not one empty one.
func splitNUL(listing string) []string {
	trimmed := strings.TrimSuffix(listing, "\x00")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "\x00")
}
