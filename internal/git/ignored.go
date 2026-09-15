package git

import "strings"

// IgnoredEnvFiles returns the gitignored `.env*` files at the root of the
// checkout at root, in the order git lists them.
//
// Only names are read, never content: `ls-files --others --ignored` lists what
// the working tree holds that no commit carries and an ignore rule covers, which
// is exactly the set a worktree added at a revision does not bring with it.
// `:(glob).env*` keeps the listing to the root, because a `*` under the glob
// magic does not cross a `/`, and `--directory` keeps git from descending into
// an ignored tree such as `vendor/` to find out. An ignored directory the
// pathspec still names comes back with a trailing `/` and is dropped, so what
// is returned are files a `sandbox.copy` entry could name.
//
// Measured 2026-09-15 with git 2.55.0 on a Laravel clone holding an ignored
// `vendor/` and `node_modules/` and a tracked `.env.example`: 6ms, naming
// `.env` alone. On a scratch repository with `sub/.env`, `vendor/x/.env` and an
// ignored `.envdir/`, it named `.env`, `.env.local` and `.env.testing`.
func IgnoredEnvFiles(root string) ([]string, error) {
	listing, err := run(root, "ls-files", "-z", "--others", "--ignored", "--exclude-standard",
		"--directory", "--no-empty-directory", "--", ":(glob).env*")
	if err != nil {
		return nil, err
	}
	files := make([]string, 0)
	for _, name := range splitNUL(listing) {
		if strings.HasSuffix(name, "/") {
			continue
		}
		files = append(files, name)
	}
	return files, nil
}
