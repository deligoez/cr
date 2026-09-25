package git

import "strings"

// IgnoredEnvFiles returns the gitignored `.env*` files at the root of the
// checkout at root, in the order git lists them, followed by the gitignored
// files matching `storage/*.key` (§5.1.2).
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
//
// The keys are listed by a second read without `--directory`, because a
// `storage/` ignored whole would otherwise come back as the directory and be
// dropped. Measured 2026-09-25 with git 2.55.0: with `--directory`, a
// repository ignoring `storage/` listed `storage/` for `:(glob)storage/*.key`;
// without it, `storage/oauth-private.key`, and a `storage/app/deep.key` was not
// listed either way. Laravel Passport keeps its keys there, and on
// tarfin-labs/backend#6328 19 baseline tests failed `Invalid key supplied`
// in a sandbox that lacked them.
func IgnoredEnvFiles(root string) ([]string, error) {
	listing, err := run(root, "ls-files", "-z", "--others", "--ignored", "--exclude-standard",
		"--directory", "--no-empty-directory", "--", ":(glob).env*")
	if err != nil {
		return nil, err
	}
	keys, err := run(root, "ls-files", "-z", "--others", "--ignored", "--exclude-standard",
		"--", ":(glob)storage/*.key")
	if err != nil {
		return nil, err
	}
	files := make([]string, 0)
	for _, name := range append(splitNUL(listing), splitNUL(keys)...) {
		if strings.HasSuffix(name, "/") {
			continue
		}
		files = append(files, name)
	}
	return files, nil
}
