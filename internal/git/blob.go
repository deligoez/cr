package git

import (
	"slices"
	"strings"
)

// Blob is one file a revision holds: the path it sits at, and the object id of
// the content stored there.
//
// The object id is carried rather than re-derived because §4.3.1's index reads
// every source file of one head, and naming `rev:path` a second time for each
// would resolve the same path twice — once to list it and once to read it —
// which is two chances for a listing and a read to disagree about what the head
// holds. It is the reason FileAtRevision already reads the blob by the id
// ls-tree just reported.
type Blob struct {
	// Path is the file's path from the root of the tree, slash-separated
	// and repository-relative, spelled the way the repository spells it.
	Path string
	// Object is the id of the blob stored at Path.
	Object string
}

// BlobsAtRevision lists every file rev holds, in the order git reports them.
//
// It reads the revision and never the worktree, for the reason FileAtRevision
// gives: §2.1.1 makes a command reproducible given the same head, and a dirty
// checkout is not the head. §4.3.1's symbol index is built over the head, so an
// unsaved edit must not add, remove, or move a symbol in it.
//
// Trees and submodules are left out. Neither holds lines a symbol can be
// declared on, and a listing that carried them would make every caller filter
// the same two cases back out.
func BlobsAtRevision(dir, rev string) ([]Blob, error) {
	args := slices.Clone(lsTreeArgs)
	// -r walks into subtrees, so the listing is the head's files rather
	// than its top-level entries. --end-of-options keeps a revision that
	// begins with a dash from being read as a flag.
	args = append(args, "-r", "--end-of-options", rev)
	listing, err := run(dir, args...)
	if err != nil {
		return nil, err
	}
	return parseBlobs(listing), nil
}

// BlobLines returns the lines the object holds, split the way FileAtRevision
// splits a file, so a line number means the same thing whichever read produced
// it.
func BlobLines(dir, object string) ([]string, error) {
	content, err := run(dir, "cat-file", "blob", object)
	if err != nil {
		return nil, err
	}
	return splitLines(content), nil
}

// parseBlobs reads an `ls-tree -r -z` listing into its blob entries.
//
// A record is `<mode> SP <type> SP <object> TAB <path>`, the shape blobAt reads
// for one path. A record this cannot read is dropped rather than returned as an
// error: the listing comes from git and not from a user, so a record that does
// not parse is a git that changed its output, and losing one file from a symbol
// index costs a candidate the agent might have been shown. §4.3.4 makes every
// reinvention item a question, so a missing candidate is a question not asked
// rather than a wrong assertion made.
func parseBlobs(listing string) []Blob {
	records := strings.Split(strings.TrimSuffix(listing, "\x00"), "\x00")
	blobs := make([]Blob, 0, len(records))
	for _, record := range records {
		meta, path, separated := strings.Cut(record, "\t")
		if !separated || path == "" {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 || fields[1] != "blob" {
			continue
		}
		blobs = append(blobs, Blob{Path: path, Object: fields[2]})
	}
	return blobs
}
