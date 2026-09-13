package git

import "strings"

// Toplevel is the absolute path of the top-level directory of the working tree
// dir lies in, which is the repository root whichever of its directories dir is.
//
// §5.6.1 names the probe lock after the absolute path of the repository under
// review, and cr is run from wherever its user stands: a path taken from the
// working directory would name one lock at the root and another in every
// subdirectory, so two runs against the same test database would not wait on
// each other. git answers with symbolic links resolved, so two spellings of one
// checkout are one root as well.
//
// Only the one newline git ends its answer with is removed. A directory name may
// itself end in a space, and trimming whitespace would name a different path.
func Toplevel(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(out, "\n"), nil
}
