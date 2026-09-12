package state

import (
	"fmt"
	"os"
)

// WriteNamedFile writes a file the caller named on the command line.
//
// It is the one write in cr whose path §2.2's table does not derive. §6.5.1
// spells `cr merge <files...> -o <out>`, so the output is the caller's file at
// the caller's path, the way the inputs beside it are the caller's files — and
// there is no root of cr's to join it onto.
//
// It lives in this package all the same, and that is the whole point of putting
// it here rather than writing it at the call site.
// TestOnlyTheStatePackageWritesToTheFilesystem fixes where a write may be
// spelled at all, so that reading every write cr performs is reading one
// package. A command reaching for os.WriteFile itself could pass that guard
// only by exempting the file it sat in, and such an exemption outlives the one
// write that earned it: the file would be free to write anywhere, forever,
// while the guard went on reporting nothing.
//
// What it deliberately does not do is judge the path. §2.2 forbids cr to write
// inside the repository under review, and an `-o` aimed there is the caller
// aiming it: §11's own table spells the flag, and a refusal here would have to
// resolve the checkout on every merge in order to tell the caller something
// they had just typed. The guard that measures §2.2 over a real repository
// names a path outside it, which is where a merged file belongs.
func WriteNamedFile(path string, body []byte) error {
	if err := os.WriteFile(path, body, filePerm); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
}
