package run

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CheckPaths holds every `--path` of §5.2.1 to what the section requires of
// one: relative, clean, and resolving inside the sandbox. The first value that
// is not is refused, in the order the section lists the three conditions, so
// the fault a reader is shown is the earliest one in their own command line.
//
// Whether the path exists is deliberately not asked, and §5.2.1 says so in as
// many words. A runner is entitled to take a directory a setup command has yet
// to create, a suite name that is not a file at all, or a path the profile's
// `tests.paths_arg` turns into something else; cr would be guessing at the
// runner's vocabulary, and guessing wrong would refuse a run that works.
//
// The check is lexical, and that is what makes it total. A relative path whose
// cleaned spelling does not begin with `..` cannot leave the directory it is
// resolved against, whatever that directory holds, so the answer is right
// before §5.1.6 has built a sandbox to resolve it against — which is where a
// refusal has to land, since §11.2 codes a malformed invocation 2 and a run
// refused for its command line must have executed nothing.
func CheckPaths(paths []string) error {
	for _, path := range paths {
		if err := checkPath(path); err != nil {
			return err
		}
	}
	return nil
}

// checkPath is CheckPaths for one value.
func checkPath(path string) error {
	switch cleaned := filepath.Clean(path); {
	case filepath.IsAbs(path):
		return fmt.Errorf(
			"--path %q is absolute: §5.2.1 narrows the run to a path inside the sandbox, "+
				"written relative to its root", path)
	case cleaned != path:
		return fmt.Errorf(
			"--path %q is not clean: §5.2.1 passes it to the runner as written, and its clean "+
				"spelling is %q", path, cleaned)
	case cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)):
		return fmt.Errorf(
			"--path %q leaves the sandbox: §5.2.1 narrows the run to a path that resolves "+
				"inside it", path)
	}
	return nil
}
