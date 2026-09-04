package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §4.3.1's index is built over one head, so the listing has to be the head's
// files: every one of them, from every directory, named from the root of the
// tree whatever directory git was run in.
func TestARevisionsBlobsAreListedRecursivelyFromTheRootOfTheTree(t *testing.T) {
	dir := committed(t, map[string]string{
		"app/Models/Order.php": "<?php\nclass Order {}\n",
		"app/helpers.php":      "<?php\nfunction money() {}\n",
		"README.md":            "# fixture\n",
	})

	blobs, err := BlobsAtRevision(dir, "HEAD")

	require.NoError(t, err)
	paths := make([]string, 0, len(blobs))
	for _, blob := range blobs {
		assert.NotEmpty(t, blob.Object, "%s was listed without the object holding it", blob.Path)
		paths = append(paths, blob.Path)
	}
	assert.ElementsMatch(t, []string{"README.md", "app/Models/Order.php", "app/helpers.php"}, paths,
		"a listing that stopped at the top level would index one file of a Laravel tree")
}

// The listing reads the revision and never the worktree, for the reason
// FileAtRevision gives: §2.1.1 makes a run reproducible given the same head,
// and an unsaved edit is not the head. A file added on disk and never committed
// would otherwise put symbols in the index that no reviewer of this head can
// open.
func TestBlobsAreReadFromTheCommitAndNotFromTheWorktree(t *testing.T) {
	dir := committed(t, map[string]string{"app.go": "package app\n\nfunc Retry() {}\n"})
	writeFixtureFile(t, dir, "app.go", "package app\n\nfunc Renamed() {}\n")
	writeFixtureFile(t, dir, "uncommitted.go", "package app\n\nfunc Ghost() {}\n")

	blobs, err := BlobsAtRevision(dir, "HEAD")

	require.NoError(t, err)
	require.Len(t, blobs, 1, "uncommitted.go is not part of this head")
	require.Equal(t, "app.go", blobs[0].Path)

	lines, err := BlobLines(dir, blobs[0].Object)
	require.NoError(t, err)
	assert.Equal(t, []string{"package app", "", "func Retry() {}"}, lines)
}

// A revision that does not resolve is git refusing, which §3.1.3 makes an exit
// code 3 rather than an empty listing that reads as a head holding no files.
func TestARevisionThatDoesNotResolveIsAnError(t *testing.T) {
	dir := committed(t, map[string]string{"app.go": "package app\n"})

	_, err := BlobsAtRevision(dir, "no-such-revision")

	require.Error(t, err)
	var refused *CommandError
	assert.ErrorAs(t, err, &refused)

	_, err = BlobLines(dir, "0000000000000000000000000000000000000000")
	require.Error(t, err)
}

// A record the listing shape does not fit is dropped rather than misread. A
// tree and a submodule are the two entries `ls-tree -r` can still produce that
// hold no lines, and a Blob naming one would be handed to a scanner as a file.
func TestOnlyBlobRecordsBecomeBlobs(t *testing.T) {
	listing := "040000 tree aaa\tapp\x00" +
		"100644 blob bbb\tapp/Order.php\x00" +
		"160000 commit ccc\tvendor/pinned\x00" +
		"malformed\x00" +
		"100644 blob\tmissing-the-object\x00"

	assert.Equal(t, []Blob{{Path: "app/Order.php", Object: "bbb"}}, parseBlobs(listing))
}

// An empty listing is no blobs rather than one empty record, which is the
// trailing-NUL reading splitNUL already fixes for the other listings.
func TestAnEmptyListingHoldsNoBlobs(t *testing.T) {
	assert.Empty(t, parseBlobs(""))
}
