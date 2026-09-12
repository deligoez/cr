package unit

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/git"
)

// §3.4.2 excludes a file a glob matches and counts it; §3.4.7 lists a binary
// or generated file without clustering it. A glob wins over both kinds, so an
// excluded binary file is counted and not listed, and a file with no changed
// line is neither listed nor clustered.
func TestSortFilesExcludesListsAndClustersEachFileOnce(t *testing.T) {
	changed := []git.ChangedFile{
		{Path: "lib.go", Changed: true},
		{Path: "vendor/dep.go", Changed: true},
		{Path: "vendor/blob.bin", Binary: true},
		{Path: "logo.png", Binary: true},
		{Path: "api.pb.go", Changed: true},
		{Path: "moved.txt"},
	}
	generated := map[string]bool{"api.pb.go": true, "logo.png": true}

	files := sortFiles(changed, []string{"vendor/**"}, generated)

	assert.Equal(t, 2, files.Excluded)
	assert.Equal(t, []Listed{
		{Path: "logo.png", Kind: KindBinary},
		{Path: "api.pb.go", Kind: KindGenerated},
	}, files.Listed)
	assert.Equal(t, []string{"vendor/dep.go", "vendor/blob.bin"}, files.ExcludedPaths())
	assert.Equal(t, []string{"lib.go"}, files.ClusteredPaths())

	kept := files.Clusterable([]git.Hunk{{Path: "lib.go"}, {Path: "vendor/dep.go"}, {Path: "api.pb.go"}})
	assert.Equal(t, []git.Hunk{{Path: "lib.go"}}, kept,
		"only the hunks of a file neither excluded nor listed reach §3.4.4")
}

// No glob excludes nothing, and a diff with no file sorts to an empty listing
// rather than a null one.
func TestSortFilesOverNoGlobAndNoFile(t *testing.T) {
	files := sortFiles(nil, nil, map[string]bool{})

	assert.Zero(t, files.Excluded)
	assert.NotNil(t, files.Listed)
	assert.Empty(t, files.Listed)
}
