package git

import (
	"fmt"
	"slices"
	"strings"
)

// ChangedFile is one file of §3.4.1's diff, as the file list rather than the
// patch reports it.
//
// It exists because the patch cannot report every file. ParseHunks reads hunks,
// and a binary file has none: git writes `Binary files … differ` where the hunks
// would be, so a reader of the patch alone drops a binary file silently rather
// than listing it as §3.4.7 requires.
type ChangedFile struct {
	// Path is the head-side path, the same one Hunk.Path carries, falling
	// back to the merge-base path for a file the change deletes.
	Path string
	// Binary reports that git diffed the file as binary, so no hunk of it
	// exists to be clustered.
	Binary bool
	// Changed reports that the file has at least one added or removed line,
	// which is exactly the case in which the patch holds a hunk for it. A
	// pure rename or a mode change has none.
	Changed bool
}

// ChangedFiles returns every file the diff from mergeBase to head touches, in
// the order git lists them.
//
// It carries diffArgs, so rename detection and every other knob that decides
// which files a diff reports is the one DiffAgainstMergeBase ran with, and the
// paths here are the paths the hunks of that diff carry — all but `--unified`,
// which implies the patch and would have git write it after the list. The
// context width decides where a hunk ends and never whether a file has one.
// The merge base is taken as given rather than resolved again, so the caller
// pairs this list with the patch of one diff and not with the patch of a
// second.
func ChangedFiles(dir, mergeBase, head string) ([]ChangedFile, error) {
	args := slices.Clone(diffArgs)
	args = slices.DeleteFunc(args, func(arg string) bool {
		return strings.HasPrefix(arg, "--unified")
	})
	args = append(args, "--numstat", "-z", "--end-of-options", mergeBase, head, "--")
	listing, err := run(dir, args...)
	if err != nil {
		return nil, err
	}
	return parseNumstat(splitNUL(listing))
}

// parseNumstat reads `--numstat -z` records.
//
// A record is `added<TAB>removed<TAB>path`, and a rename writes an empty path
// followed by two records of its own, the merge-base path and then the head
// path. A binary file writes `-` for both counts.
func parseNumstat(records []string) ([]ChangedFile, error) {
	files := make([]ChangedFile, 0, len(records))
	for i := 0; i < len(records); i++ {
		added, removed, path, ok := numstatFields(records[i])
		if !ok {
			return nil, fmt.Errorf("numstat record %q is not added, removed and path", records[i])
		}
		if path == "" {
			if i+2 >= len(records) {
				return nil, fmt.Errorf("numstat rename record %q names no head path", records[i])
			}
			path, i = records[i+2], i+2
		}
		files = append(files, ChangedFile{
			Path:    path,
			Binary:  added == "-",
			Changed: added != "-" && (added != "0" || removed != "0"),
		})
	}
	return files, nil
}

// numstatFields cuts one record at its first two tabs. A path may itself hold a
// tab, so the cut stops there.
func numstatFields(record string) (added, removed, path string, ok bool) {
	added, rest, ok := strings.Cut(record, "\t")
	if !ok {
		return "", "", "", false
	}
	removed, path, ok = strings.Cut(rest, "\t")
	return added, removed, path, ok
}

// Generated returns which of paths the repository at head declares generated:
// the ones `git check-attr` reports `linguist-generated` set or true for, read
// from the attributes of the head commit's tree.
//
// `--source` reads the tree rather than the worktree, so the answer is the one
// head declares and not the one the checkout happens to hold; measured on git
// 2.55.0, a worktree `.gitattributes` the head tree lacks leaves a path
// `unspecified`. Two attribute sources outside the tree are still consulted,
// because run pins neither: the per-clone `$GIT_DIR/info/attributes`, which git
// offers no switch to exclude, and `core.attributesFile`, which the diff read
// consults too.
func Generated(dir, head string, paths []string) (map[string]bool, error) {
	generated := make(map[string]bool, len(paths))
	if len(paths) == 0 {
		return generated, nil
	}
	args := []string{"check-attr", "-z", "--source=" + head, "linguist-generated", "--"}
	args = append(args, paths...)
	listing, err := run(dir, args...)
	if err != nil {
		return nil, err
	}
	records := splitNUL(listing)
	if len(records)%3 != 0 {
		return nil, fmt.Errorf("check-attr wrote %d records, which is not a path, attribute and value per file", len(records))
	}
	for i := 0; i < len(records); i += 3 {
		if value := records[i+2]; value == "set" || value == "true" {
			generated[records[i]] = true
		}
	}
	return generated, nil
}
