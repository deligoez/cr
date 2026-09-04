package rule

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runGit runs one git command in dir and returns its output, failing the test
// on any error. It is the test's own runner rather than internal/git's, which
// is deliberately not exported: what these cases need is a repository to
// measure, and cr's runner exists to read one under §2.1.1's pinned knobs.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=cr", "GIT_AUTHOR_EMAIL=cr@example.com",
		"GIT_COMMITTER_NAME=cr", "GIT_COMMITTER_EMAIL=cr@example.com")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return string(out)
}

// changedRepository is a real repository whose head commit adds the line a
// fix-carrying rule matches, and returns its root together with the diff of
// that change.
//
// It is a repository rather than a fixture string because that is the claim
// under test. §2.6.2.3 says cr applies a fix to no file, and a diff held in a
// constant has no file to apply one to — the assertion would hold against an
// implementation that rewrote every source it touched.
func changedRepository(t *testing.T) (root, patch string) {
	t.Helper()
	root = t.TempDir()
	source := filepath.Join(root, "app", "Models", "Order.php")
	require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o750))

	require.NoError(t, os.WriteFile(source, []byte("<?php\n$tail = 1;\n"), 0o600))
	runGit(t, root, "-c", "init.defaultBranch=main", "init", "--quiet")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "--quiet", "-m", "the commit under review")

	require.NoError(t, os.WriteFile(source,
		[]byte("<?php\n$added = DB::raw('new');\n$tail = 1;\n"), 0o600))
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "--quiet", "-m", "the change under review")

	return root, runGit(t, root, "diff", "--no-ext-diff", "--no-color", "HEAD~1", "HEAD")
}

// worktreeOf reads every tracked and untracked file of a checkout as bytes,
// keyed by its repository-relative path.
//
// `.git` is left out on purpose, and the omission is not a weakening. Running
// `git status` writes to the index cache itself, so a snapshot including it
// would measure git's own bookkeeping rather than cr's behaviour — and the
// invariant §2.2 states is about the user's worktree, index, and branch, which
// `git status --porcelain` reads back beside this.
func worktreeOf(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, relErr := filepath.Rel(root, path)
		require.NoError(t, relErr)
		if entry.IsDir() {
			if relative == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		files[filepath.ToSlash(relative)] = string(data)
		return nil
	}))
	return files
}

// §2.6.2.3: cr applies a fix to no file. A fix only ever produces suggestion
// text for the author to accept.
//
// The measurement is the whole path against a real checkout: a rule carrying a
// fix, the real diff of a real commit, the hit that diff produces, the
// suggestion generated from it, and the record it lands on. Then the working
// tree is read back twice over — file by file, and through `git status
// --porcelain`, which also answers for the index and for a file cr might have
// added rather than changed.
//
// It is the second invariant of the project as much as §2.6.2.3: cr never
// writes inside the repository under review, and a fix is the one feature whose
// output is literally source code, so it is the feature most likely to be
// implemented as an edit by someone who has not read the section.
func TestAFixCarryingRuleLeavesTheWorkingTreeUnchanged(t *testing.T) {
	root, patch := changedRepository(t)
	before := worktreeOf(t, root)
	require.Empty(t, runGit(t, root, "status", "--porcelain"),
		"the fixture must start clean for the assertion below to mean anything")

	matcher := fixMatcher(t, fixBlock(nil))
	hits := Evaluate([]Matcher{matcher}, hunksOf(t, patch))
	require.Len(t, hits, 1)

	record := aRuleRecord()
	record.Anchor.StartLine, record.Anchor.Line = hits[0].Line, hits[0].Line
	require.True(t, matcher.Suggest(&record, &hits[0], hunksOf(t, patch)),
		"the fix must have produced a suggestion for the assertion below to mean anything")
	require.Equal(t, `$added = DB::selectRaw('new');`, record.Suggestion)

	assert.Equal(t, before, worktreeOf(t, root), "§2.6.2.3 applies the fix to no file")
	assert.Empty(t, runGit(t, root, "status", "--porcelain"),
		"nothing was added, changed, or staged in the repository under review")
}

// The suggestion is text and the line it was computed from is untouched, which
// is the same claim one step in from the checkout. A fix that rewrote its input
// in place would still leave the tree clean when the input came from a diff
// held in memory.
func TestAGeneratedSuggestionDoesNotRewriteTheLineItReads(t *testing.T) {
	matcher := fixMatcher(t, fixBlock(nil))
	hunks := hunksOf(t, mixedDiff)
	hit := oneHit(t, &matcher)
	record := aRuleRecord()

	require.True(t, matcher.Suggest(&record, &hit, hunks))

	assert.Equal(t, `$added = DB::raw('new');`, hit.Text, "the hit still carries what the diff held")
	assert.Equal(t, []git.ChangedLine{
		{Side: git.Right, Line: 11, Text: "$added = DB::raw('new');"},
	}, hunks[0].Changed, "and so does the hunk it came from")
}

// fileCalls are the calls to the file-system packages this package's own source
// may make, as `package.Function`. Both read.
//
// It is an allowlist and not a list of forbidden writes, for the reason
// internal/git inherits an environment rather than filtering one: a denylist
// answers only for the calls whoever wrote it thought of, and `os` grows. A
// write added here fails this test by not being on the list, whatever it is
// called and whichever release of Go introduced it.
var fileCalls = []string{"os.ReadDir", "os.ReadFile"}

// filePackages are the standard-library packages that can reach a file, named
// so a call through any of them is measured rather than only a call through os.
var filePackages = []string{"os", "io", "exec", "ioutil", "syscall"}

// §2.6.2.3 read off the source rather than off a run: a package that names no
// function able to write a file cannot write one, whatever a later edit
// believes it is doing.
//
// The run above says what cr did with the rule this test wrote; this says what
// cr can be seen to be able to do at all, and it fails on the edit that
// introduces a write rather than on the round a suggestion silently becomes a
// patch. internal/cli's TestNoCommandTouchesTheRepositoryUnderReview makes the
// same guarantee at the command surface; this one makes it where the fix lives.
func TestTheRulePackageNamesNoCallThatCouldApplyAFix(t *testing.T) {
	sources, err := os.ReadDir(".")
	require.NoError(t, err)

	named := make([]string, 0, len(fileCalls))
	for _, source := range sources {
		name := source.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		require.NoError(t, parseErr)
		named = append(named, fileCallsIn(parsed)...)
	}
	slices.Sort(named)

	assert.Equal(t, fileCalls, slices.Compact(named),
		"§2.6.2.3 produces suggestion text; a rule never applies anything to a file")
}

// fileCallsIn returns every `package.Function` this file names from one of
// filePackages, whether or not the result is called.
func fileCallsIn(parsed *ast.File) []string {
	named := make([]string, 0)
	ast.Inspect(parsed, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := selector.X.(*ast.Ident)
		if ok && slices.Contains(filePackages, pkg.Name) {
			named = append(named, pkg.Name+"."+selector.Sel.Name)
		}
		return true
	})
	return named
}

// A record only ever carries text. §6.1's `suggestion` row is a string, and the
// path to a file is not on the record at all — the anchor names one, and
// nothing in this package resolves it against a tree.
func TestASuggestionIsTextAndNamesNoFileToWrite(t *testing.T) {
	matcher := fixMatcher(t, fixBlock(nil))
	hit := oneHit(t, &matcher)
	record := aRuleRecord()

	require.True(t, matcher.Suggest(&record, &hit, hunksOf(t, mixedDiff)))

	assert.IsType(t, "", record.Suggestion)
	assert.NotContains(t, record.Suggestion, record.Anchor.Path,
		"the suggestion is the replacement line and carries no destination")
	assert.Equal(t, finding.Anchor{
		Path: "app/Models/Order.php", Side: git.Right, StartLine: 11, Line: 11,
	}, record.Anchor, "and the anchor it would be posted against is unchanged")
}
