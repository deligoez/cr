package git

import (
	"slices"
	"strings"
)

// lsTreeArgs are the flags every tree lookup carries, pinned for the reason
// diffArgs are pinned: §2.1.1 has the same state, the same head, and the same
// inputs give the same result, and where git happened to be run is none of the
// three.
var lsTreeArgs = []string{
	"ls-tree",
	// Paths from the root of the tree rather than from the directory git
	// ran in, so a citation's path means the same file wherever cr was
	// invoked from.
	"--full-tree",
	// NUL-terminated records with raw paths, so a path holding a quote, a
	// backslash or a newline arrives spelled the way the repository spells
	// it instead of in git's quoted form.
	"-z",
}

// FileAtRevision returns the lines path holds at rev, and whether rev holds it
// as a file at all.
//
// # It reads the revision, never the worktree
//
// The two differ whenever the checkout is dirty, and §2.1.1 settles which one
// this is: a command is reproducible given the same state directory, the same
// head SHA, and the same inputs. A worktree is none of those — the same command
// against the same head would answer differently before and after an unsaved
// edit — while a revision is exactly the head SHA, so two runs against one head
// read one text. §6.2.3's "against the current head" is that head, and §6.2.1
// evaluates containment in head coordinates, which are the revision's line
// numbers and not an editor's.
//
// Reading the object database rather than the checkout is also what keeps
// invariant 2 honest in the other direction: `cat-file` hands back the blob as
// it is stored, so `core.autocrlf`, a smudge filter, or a `.gitattributes`
// `text` setting cannot change the bytes cr hashes, and nothing here consults
// or refreshes the index.
//
// # A path that is not held is not an error
//
// Absence is reported rather than returned as a failure, because the two
// outcomes have different exit codes. §6.2.3 rejects a citation whose path does
// not exist with exit code 1 — the record's content is wrong — while a git that
// refuses is §3.1.3's exit code 3. A bad revision therefore stays an error, and
// a path the revision does not hold does not.
func FileAtRevision(dir, rev, path string) (lines []string, exists bool, err error) {
	if !namesATreeEntry(path) {
		return nil, false, nil
	}
	args := slices.Clone(lsTreeArgs)
	// --end-of-options keeps a revision that begins with a dash from being
	// read as a flag; the -- separates the revision from the path.
	args = append(args, "--end-of-options", rev, "--", path)
	listing, err := run(dir, args...)
	if err != nil {
		return nil, false, err
	}
	blob, held := blobAt(listing, path)
	if !held {
		return nil, false, nil
	}

	// The object is named by the id git just reported rather than by
	// `rev:path` a second time, so the blob that is read is the blob that
	// was found and no second path resolution can land elsewhere.
	content, err := run(dir, "cat-file", "blob", blob)
	if err != nil {
		return nil, false, err
	}
	return splitLines(content), true, nil
}

// namesATreeEntry reports whether path could name an entry of a git tree at all.
//
// A tree stores relative slash-separated paths, and no entry of one is empty,
// absolute, or carries a `.` or `..` segment. Such a path is answered here
// rather than handed to git because git refuses a path outside the repository
// with a fatal error, and a citation's path is written by an agent: §6.2.3
// makes an unopenable citation a validation failure, so `../../etc/passwd` must
// read as a path the head does not hold and not as an external command that
// blew up.
func namesATreeEntry(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// blobAt reports the object one `ls-tree -z` listing holds at path.
//
// A record is `<mode> SP <type> SP <object> TAB <path>`, and the listing is
// required to be exactly one record naming exactly the path that was asked for.
// Both halves are load-bearing. `ls-tree -- dir/` lists that directory's
// children rather than nothing, so its first record is a file nobody asked
// about; and a tree or a submodule sitting at the path is not something a
// citation can name a line in, which is the same answer as no entry at all.
func blobAt(listing, path string) (string, bool) {
	records := strings.Split(strings.TrimSuffix(listing, "\x00"), "\x00")
	if len(records) != 1 {
		return "", false
	}
	meta, named, separated := strings.Cut(records[0], "\t")
	if !separated || named != path {
		return "", false
	}
	fields := strings.Fields(meta)
	if len(fields) != 3 || fields[1] != "blob" {
		return "", false
	}
	return fields[2], true
}

// splitLines cuts a file's content into its lines, numbered from one.
//
// The final newline is a terminator and not a separator, so a file ending in
// one does not gain an empty last line, and a file holding nothing has no lines
// rather than one empty one. It is the split ParseHunks already applies to a
// patch, so a line number means the same thing on both sides of §6.2.1's
// containment test.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}
