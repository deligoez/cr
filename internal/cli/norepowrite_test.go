package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// Invariant 2, which §2.2 states as normative text: all state lives under
// `~/.cr/`, cr MUST NOT write inside the repository under review, and it MUST
// NOT modify that repository's tracked files, index, HEAD, stash, or any
// branch. §5.1.1 carves out the single exception — the worktree registration
// under `.git/worktrees/` that `cr sandbox create` leaves behind.
//
// nowrite_test.go in this package guards a differently-named sentence: §2.1.2's
// prohibition on *network* writes and the confirmation gate §8.5 puts in front
// of its one exception. Nothing there reads a path, so nothing there covers
// this.
//
// # Where a write could come from
//
// cr reaches the repository under review two ways and no third. It can address
// a path itself, which needs one of the os functions that change something on
// disk; or it can drive git, behind which every asset §2.2 names — the index,
// HEAD, the stash, every branch — sits. internal/git is the only door to git,
// which TestCrReachesTheNetworkThroughOneRunnerAndNoOtherWay already holds
// shut, so the two static guards here fence the two routes: which files may
// call an os write, and which git subcommands may be run. The behavioural guard
// then drives the built binary against a real repository and reads the result
// off disk, which is the only one of the three that can see a write nobody
// predicted.

// filesystemWrites are the os functions that change something on disk, or hand
// back a handle that can.
//
// The constructors are listed with the direct writes because a handle is the
// same write one line later: *os.File's Write, WriteString, Truncate and Chmod
// are unreachable without one of Create, CreateTemp, OpenFile, NewFile or
// OpenRoot, and os.Open cannot produce one that writes. Fencing the five is
// therefore the whole of it, and it needs no type information — which matters,
// because a guard that had to resolve types would be a guard that stops working
// the first time a file fails to type-check.
var filesystemWrites = map[string]bool{
	// Direct writes.
	"WriteFile": true,
	"Mkdir":     true,
	"MkdirAll":  true,
	"MkdirTemp": true,
	"Remove":    true,
	"RemoveAll": true,
	"Rename":    true,
	"Link":      true,
	"Symlink":   true,
	"Truncate":  true,
	"Chmod":     true,
	"Chown":     true,
	"Lchown":    true,
	"Chtimes":   true,
	// Handles that can write.
	"Create":     true,
	"CreateTemp": true,
	"OpenFile":   true,
	"NewFile":    true,
	"OpenRoot":   true,
}

// statePackage is the one package allowed to call them. Its doc comment says
// why in a sentence: it derives every path cr writes, and every path it derives
// is resolved against one root, which
// TestEveryPathTheStateLayoutHandsOutIsUnderItsRoot checks is really so.
//
// Widening this list is how invariant 2 would be lost, and it is deliberately a
// thing a reviewer sees rather than something a call site does by existing.
var statePackage = filepath.Join("internal", "state") + string(filepath.Separator)

// eachSourceFile parses the Go files cr ships — its own source, tests excluded —
// and hands each one's repository-relative path and syntax tree to visit.
//
// Tests are out of the surface on purpose. They write temporary trees by design
// and are the wrong thing to police: what invariant 2 is about is the binary a
// colleague runs against their own checkout.
func eachSourceFile(t *testing.T, visit func(rel string, file *ast.File)) {
	t.Helper()
	root := moduleRoot(t)

	scanned := 0
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		require.NoError(t, err)

		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)

		scanned++
		visit(rel, parsed)
		return nil
	}))

	// A walk that found nothing would pass and would mean nothing.
	require.Greater(t, scanned, 10, "only %d files were scanned, so this guard proved nothing", scanned)
}

// osWrite reports the os function a call invokes and whether it writes.
//
// The package qualifier is matched as text, which is exactly as strong as the
// import guard beside it: `os` under an alias would read as some other package
// here and slip past, so the caller fails an aliased import outright rather
// than trusting a name.
func osWrite(call *ast.CallExpr) (string, bool) {
	fun, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := fun.X.(*ast.Ident)
	if !ok || pkg.Name != "os" {
		return "", false
	}
	return fun.Sel.Name, filesystemWrites[fun.Sel.Name]
}

// lowLevelPackages reach the filesystem underneath os, so importing one outside
// internal/state would put a write where the call scan cannot read it.
var lowLevelPackages = map[string]bool{
	"syscall":                  true,
	"golang.org/x/sys/unix":    true,
	"golang.org/x/sys/windows": true,
}

// processControlFile is the one file outside internal/state that may import
// `syscall`, and processControl is every name it may use out of it.
//
// §5.2.3 has cr kill a run that outlives `tests.timeout_seconds`, and what has
// to die is the process group: a runner that started `php` or `composer`
// leaves them holding the test database §5.6's lock serialises access to, and
// holding the pipe the run's output travels through. Neither the group a child
// is put in nor a signal to that group can be spelled without `syscall`.
//
// The exemption is by name rather than by file, so the guard keeps saying what
// it said before. Every identifier below controls a process and addresses no
// path, and anything else out of `syscall` — Open, Unlink, Mkdir, Rename —
// fails this test in the file that is exempt exactly as it would anywhere
// else. What is widened is not "this file may reach the filesystem" but "these
// three names are not the filesystem".
var processControlFile = filepath.Join("internal", "sandbox", "run.go")

var processControl = map[string]bool{
	"SysProcAttr": true,
	"Kill":        true,
	"SIGKILL":     true,
}

// syscallUse reports the syscall identifier an expression names, if it names
// one. The package qualifier is matched as text, which is as strong as the
// import guard beside it: an aliased import is refused outright above.
func syscallUse(n ast.Node) (string, bool) {
	sel, ok := n.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "syscall" {
		return "", false
	}
	return sel.Sel.Name, true
}

// Nothing outside internal/state writes to the filesystem at all.
//
// This is invariant 2's first half — cr never writes inside the repository
// under review — proved from the other end. Rather than trying to decide, at
// each call site, whether the path being written happens to sit inside the
// user's checkout, the guard fixes where a write may be spelled: one package,
// whose every path is rooted at `~/.cr`. A write into the repository under
// review then has nowhere to be written from.
//
// state.WriteNamedFile is the one write in that package whose path §2.2 does
// not derive, and it changes nothing this guard asserts. §6.5.1 spells
// `cr merge <files...> -o <out>`, so that output is the caller's file at the
// caller's path; what keeps the sentence above true is that the write is still
// spelled here, in the one package an audit of cr's writes has to read. A
// command reaching for os.WriteFile itself could pass only by exempting the
// file it sat in — and the exemption would outlive the one write that earned
// it, leaving that file free to write anywhere while this guard reported
// nothing.
//
// Two ways around a call scan are closed alongside it. An aliased import of os
// would make every write invisible here, and a package that reaches the
// filesystem beneath os would never mention os at all.
//
// The one exemption is named rather than general: §5.2.3's process-group kill
// needs `syscall`, and the file that performs it may use exactly the three
// process-control names processControl lists. Every other identifier out of
// that package fails here, in the exempt file as much as anywhere else.
func TestOnlyTheStatePackageWritesToTheFilesystem(t *testing.T) {
	var found []string
	eachSourceFile(t, func(rel string, file *ast.File) {
		if strings.HasPrefix(rel, statePackage) {
			return
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			require.NoError(t, err)
			if path == "os" && spec.Name != nil {
				found = append(found, rel+" imports os as "+spec.Name.Name)
			}
			exempt := path == "syscall" && rel == processControlFile
			if lowLevelPackages[path] && !exempt {
				found = append(found, rel+" imports "+path)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if name, uses := syscallUse(n); uses && !processControl[name] {
				found = append(found, rel+" uses syscall."+name)
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if name, writes := osWrite(call); writes {
				found = append(found, rel+" calls os."+name)
			}
			return true
		})
	})

	slices.Sort(found)
	assert.Empty(t, found,
		"invariant 2: internal/state derives every path cr writes, so no other package may write at all")
}

// Every path internal/state hands out lands under its own root.
//
// This is the other half of the sentence above. Confining the writes to one
// package is worth something only if that package cannot address anything but
// `~/.cr`, and §2.2's root is a field nothing outside internal/state can set:
// state.New and state.Default are the two constructors, and every path method
// joins onto it.
//
// The methods are enumerated by reflection rather than listed, so a path added
// later is checked by existing. The filter is the signature §2.2's table
// implies — strings and pull request numbers in, one path out — and a method
// that takes an argument this guard cannot supply fails rather than being
// passed over, because an unchecked path is the one this test exists to find.
//
// The boundary it does not claim: a caller who hands a path segment of their
// own — `l.PRFile(owner, repo, pr, "../../escape")` — walks out of the tree, and
// no reflection over the signature can see that. What is fenced is the root,
// which is where a write into the repository under review would have to come
// from.
func TestEveryPathTheStateLayoutHandsOutIsUnderItsRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state-root")
	layout := reflect.ValueOf(state.New(root))
	surface := layout.Type()

	checked := 0
	for method := range surface.Methods() {
		signature := method.Type
		if signature.NumOut() != 1 || signature.Out(0).Kind() != reflect.String {
			continue
		}

		// The receiver first, then one argument per parameter. The values
		// are arbitrary: what is asserted is where the path lands, and
		// every §2.2 path is a join onto the root whatever the segments say.
		args := make([]reflect.Value, 0, signature.NumIn())
		args = append(args, layout)
		for at := 1; at < signature.NumIn(); at++ {
			switch signature.In(at).Kind() {
			case reflect.String:
				args = append(args, reflect.ValueOf("segment"))
			case reflect.Int:
				args = append(args, reflect.ValueOf(7))
			default:
				require.Fail(t, "unsupported parameter",
					"%s takes a %s, which this guard cannot supply, so its path went unchecked",
					method.Name, signature.In(at))
			}
		}

		produced := method.Func.Call(args)[0].String()
		inside, err := filepath.Rel(root, produced)
		require.NoError(t, err)
		assert.False(t, inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)),
			"§2.2: %s resolves to %s, which is outside the state root", method.Name, produced)
		checked++
	}

	// The floor is the size of §2.2's table, so a filter that quietly
	// stopped matching fails here rather than passing over an empty set.
	require.Greater(t, checked, 20, "only %d paths were checked, so this guard proved nothing", checked)
}

// gitReads are the git subcommands internal/git is allowed to run, each with
// the section that asks for it.
//
// This is the second route into the repository under review, and the one §2.2's
// second sentence is about: the index, HEAD, the stash and every branch are
// files under `.git/`, and cr does not address them — git does, on cr's behalf.
// So the guard is over the verb rather than over the path, and it is an
// allowlist of what cr runs today rather than a denylist of what git can
// destroy, because the denylist is the one that goes stale in the unsafe
// direction.
//
// §5.1.1's exception is the map's one write, and it was added by hand: `cr
// sandbox create` runs `git worktree add`, so `worktree` sits here with the
// section that asks for it. Widening the map is a deliberate act with a
// reviewer, which is the point — and the behavioural guard below independently
// fixes how far that write may reach, so the map growing by one word cannot
// quietly grant more than the one path §2.2 names.
var gitReads = map[string]string{
	"diff":       "§3.4.1's diff of the head against the merge base",
	"merge-base": "§3.4.1's merge base",
	"ls-tree":    "§6.2.3's question of whether the head holds a cited path at all, and §4.3.1's listing of the head's source",
	"cat-file":   "§6.2.3's read of the cited line as the head holds it, and §4.3.1's read of a source file to index",
	"worktree":   "§5.1.1's sandbox worktree, the one write §2.2 permits",
	"rev-parse":  "§5.1.6's check that the sandbox HEAD is still the round's head",
	"ls-files":   "§5.1.6's question of whether a file under the leftover glob is tracked",
	"check-attr": "§3.4.7's question of whether the head declares a file generated",
	"remote":     "§11.1's repository detection, which reads `git remote -v` and nothing else",
}

// gitSourceFiles parses internal/git's own source, tests excluded.
func gitSourceFiles(t *testing.T) []*ast.File {
	t.Helper()
	dir := filepath.Join(moduleRoot(t), "internal", "git")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "internal/git is cr's only door to git; if it moved, move this guard with it")

	parsed := make([]*ast.File, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		parsed = append(parsed, file)
	}
	require.NotEmpty(t, parsed, "no source was read, so this guard proved nothing")
	return parsed
}

// packageValues collects the package-level variables of internal/git by name.
//
// They are collected across the whole package rather than per file, because
// the argument list a call spreads and the runner it is spread into need not
// live in the same file — diffArgs and run do not.
func packageValues(files []*ast.File) map[string]ast.Expr {
	values := make(map[string]ast.Expr)
	for _, file := range files {
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, spec := range general.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for at, name := range value.Names {
					if at < len(value.Values) {
						values[name.Name] = value.Values[at]
					}
				}
			}
		}
	}
	return values
}

// resolutionDepth bounds the walk back from a call site to a literal. Nothing
// in internal/git needs more than three steps, and the bound is what keeps a
// cycle from hanging the suite rather than failing it.
const resolutionDepth = 8

// firstElement reports the first string an expression's argument list begins
// with: the literal itself, a slice's first element, the variable it names, or
// the first element of what slices.Clone and append were given.
//
// Clone and append are followed because both keep their first argument's first
// element, which is where the subcommand sits. Anything else returns false, and
// the caller turns that into a failure rather than into silence.
func firstElement(values map[string]ast.Expr, within *ast.FuncDecl, expr ast.Expr, depth int) (string, bool) {
	if depth > resolutionDepth {
		return "", false
	}
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind != token.STRING {
			return "", false
		}
		name, err := strconv.Unquote(node.Value)
		return name, err == nil
	case *ast.CompositeLit:
		if len(node.Elts) == 0 {
			return "", false
		}
		return firstElement(values, within, node.Elts[0], depth+1)
	case *ast.Ident:
		return namedArgv(values, within, node.Name, depth+1)
	case *ast.CallExpr:
		if len(node.Args) == 0 || !keepsItsFirstElement(node.Fun) {
			return "", false
		}
		return firstElement(values, within, node.Args[0], depth+1)
	}
	return "", false
}

// keepsItsFirstElement reports whether a call hands back a slice beginning with
// its own first argument's first element.
func keepsItsFirstElement(fun ast.Expr) bool {
	if builtin, ok := fun.(*ast.Ident); ok {
		return builtin.Name == "append"
	}
	selector, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "slices" && selector.Sel.Name == "Clone"
}

// namedArgv resolves a variable back to the argument list it holds: a
// package-level one by name, or a local one through the first assignment made
// to it in the function the call sits in.
//
// The first assignment is the one read because it is the one that establishes
// what the list begins with; every later append adds to the tail, which is
// arguments rather than the subcommand.
func namedArgv(values map[string]ast.Expr, within *ast.FuncDecl, name string, depth int) (string, bool) {
	if origin, ok := values[name]; ok {
		return firstElement(values, within, origin, depth+1)
	}
	if within == nil || within.Body == nil {
		return "", false
	}
	var origin ast.Expr
	ast.Inspect(within.Body, func(n ast.Node) bool {
		if origin != nil {
			return false
		}
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for at, target := range assign.Lhs {
			ident, isIdent := target.(*ast.Ident)
			if isIdent && ident.Name == name && at < len(assign.Rhs) {
				origin = assign.Rhs[at]
				return false
			}
		}
		return true
	})
	if origin == nil {
		return "", false
	}
	return firstElement(values, within, origin, depth+1)
}

// unreadableSubcommand is what a call reports when the guard cannot say which
// subcommand it runs. It is never in gitReads, so the unanswerable case fails
// rather than passing: a reader who cannot tell what a git invocation does is
// exactly the reader §2.2 is written for.
const unreadableSubcommand = "a subcommand named at run time"

// gitSubcommand reports the git subcommand a call runs, and whether the call
// runs one at all.
func gitSubcommand(values map[string]ast.Expr, within *ast.FuncDecl, call *ast.CallExpr) (string, bool) {
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "run" {
		return "", false
	}
	// The directory first, then the argument list, whose head is the
	// subcommand.
	if len(call.Args) < 2 {
		return unreadableSubcommand, true
	}
	if name, resolved := firstElement(values, within, call.Args[1], 0); resolved {
		return name, true
	}
	return unreadableSubcommand, true
}

// Every git subcommand cr runs is a read.
//
// §2.2's second sentence names five things cr must not modify — tracked files,
// the index, HEAD, the stash, and any branch — and all five are reached by
// running git, never by cr writing a path. Fixing the verbs is therefore how
// that sentence is held: `git diff` and `git merge-base` cannot move any of the
// five, whatever arguments they are given, and no other subcommand is reachable
// from internal/git at all.
//
// It is a source guard rather than a run, so it covers what internal/git can do
// rather than what today's commands happen to ask of it. The behavioural guard
// below is the complement, and neither is worth much without the other: this
// one sees a write added to internal/git before any command wires it, and that
// one sees a write arriving by a route nobody thought to fence.
func TestEveryGitSubcommandCrRunsIsARead(t *testing.T) {
	files := gitSourceFiles(t)
	values := packageValues(files)

	invocations := 0
	var found []string
	for _, file := range files {
		for _, decl := range file.Decls {
			within, _ := decl.(*ast.FuncDecl)
			ast.Inspect(decl, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name, runs := gitSubcommand(values, within, call)
				if !runs {
					return true
				}
				invocations++
				if _, read := gitReads[name]; !read {
					found = append(found, "internal/git runs `git "+name+"`")
				}
				return true
			})
		}
	}

	require.Greater(t, invocations, 1, "only %d git invocations were read, so this guard proved nothing", invocations)
	slices.Sort(found)
	assert.Empty(t, slices.Compact(found),
		"§2.2: cr must not modify the repository's tracked files, index, HEAD, stash, or any branch")
}

// The pull request the fixture repository stands in for. The fixture checkout
// declares no remote, so §11.1's detection has nothing to read and the
// repository is named on the command line; its §2.3 state is prepared under
// CR_HOME, which is where every command that takes a `--repo` looks.
const (
	fixtureOwner    = "octocat"
	fixtureProject  = "hello"
	fixtureSlug     = fixtureOwner + "/" + fixtureProject
	fixturePR       = "7"
	fixturePRNumber = 7
	fixtureIssue    = "CR-7"
	// fixtureHead is the head its §2.3 state was recorded against. It is a
	// value rather than the fixture's own HEAD because nothing compares the
	// two yet, and a record is stamped with whatever meta.json holds.
	fixtureHead = "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c"
	// fixtureHeadBranch is the branch the briefed pull request is opened
	// from, so §3.4.1 has a merge base and a diff rather than two names
	// for one commit.
	fixtureHeadBranch = "pr-head"
)

// worktreeRegistration is §5.1.1's exception, spelled as a repository-relative
// path: the sole write §2.2 permits inside the repository under review.
const worktreeRegistration = ".git/worktrees"

// gitIn runs one git command inside dir.
//
// The environment is pinned rather than inherited, for the reason internal/git
// pins its own: a machine whose owner configures git is a machine where an
// unpinned read answers differently. GIT_OPTIONAL_LOCKS=0 is the one that
// matters most here — without it `git status` may refresh the index, and a
// fingerprint that disturbs the repository cannot measure whether cr did.
func gitIn(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"TMPDIR=" + os.TempDir(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"LC_ALL=C",
		"LANG=C",
		"GIT_AUTHOR_NAME=cr guard",
		"GIT_AUTHOR_EMAIL=guard@example.invalid",
		"GIT_COMMITTER_NAME=cr guard",
		"GIT_COMMITTER_EMAIL=guard@example.invalid",
	}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(stderr.String()), err
	}
	return stdout.String(), nil
}

// mustGit runs one git command inside dir and fails the test if it refuses.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitIn(t, dir, args...)
	require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	return out
}

// fixtureRepository builds the repository cr is run against.
//
// It carries one of everything §2.2's second sentence names, because a
// fingerprint of an empty checkout would compare nothing to nothing: a commit
// to be HEAD, a second branch, a stash entry, a tracked file edited, a file
// staged and not committed, an untracked file, and a file the repository
// ignores. Each of those is something a careless run could disturb, and each
// has to be present before the guard can say it was not.
func fixtureRepository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}

	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("app.go", "package app\n\nfunc Retry() {}\n")
	write(".gitignore", "built/\n")
	mustGit(t, dir, "add", "app.go", ".gitignore")
	mustGit(t, dir, "commit", "--quiet", "-m", "the commit under review")
	mustGit(t, dir, "branch", "spare")

	// A branch with a commit of its own, so the pull request cr is briefed
	// on has a diff to take against its merge base. Without it §3.4 would
	// cluster an empty patch, and the run would prove nothing about what
	// `cr brief` does while it reads the repository. It is created before
	// the working tree is dirtied, so the stash, the staged file, and the
	// edited file below are exactly what they were.
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("app.go", "package app\n\nfunc Retry() { backoff() }\n")
	mustGit(t, dir, "add", "app.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the change under review")
	mustGit(t, dir, "checkout", "--quiet", "main")

	write("app.go", "package app\n\nfunc Retry() { stashed() }\n")
	mustGit(t, dir, "stash", "push", "--quiet", "-m", "work in progress")

	write("app.go", "package app\n\nfunc Retry() { edited() }\n")
	write("staged.go", "package app\n")
	mustGit(t, dir, "add", "staged.go")
	write("scratch.txt", "notes\n")
	write("built/output.bin", "\x00\x01\n")

	// The fixture is checked rather than assumed. A guard run against a
	// checkout that turned out to hold no stash, no second branch and no
	// staged change would pass and would mean nothing.
	require.Contains(t, mustGit(t, dir, "stash", "list"), "work in progress")
	refs := mustGit(t, dir, "for-each-ref", "--format=%(refname)")
	for _, ref := range []string{"refs/heads/main", "refs/heads/spare", "refs/stash"} {
		require.Contains(t, refs, ref, "the fixture was meant to hold %s", ref)
	}
	status := mustGit(t, dir, "status", "--porcelain=v2", "--untracked-files=all", "--ignored=matching")
	for _, expected := range []string{"app.go", "staged.go", "scratch.txt", "built/"} {
		require.Contains(t, status, expected, "the fixture was meant to hold %s in its status", expected)
	}
	return dir
}

// repoState is everything §2.2 forbids cr to disturb, read back off disk.
type repoState struct {
	// status is `git status`, which is the comparison the acceptance
	// criterion names.
	status string
	// refs is what `git status` alone does not report: the commit HEAD sits
	// at, the ref HEAD follows, every branch and tag, and the stash. §2.2
	// names four of those, and a branch created off the current commit
	// leaves `git status` byte-identical.
	refs string
	// files is every byte in the repository, `.git/` included, so a write
	// that neither git status nor a ref would report is caught too. This is
	// the half that answers §2.2's first sentence rather than its second.
	files string
	// permitted is the same manifest for §5.1.1's `.git/worktrees/`, held
	// apart because it is the one write §2.2 allows.
	permitted []string
}

// fileManifest hashes every entry in the repository, and returns §5.1.1's
// permitted path separately from everything else.
//
// Content rather than modification time, because a rewrite with identical bytes
// is not a modification of anything a reader can observe, and a mtime that
// moved without the content changing would fail a guard that means nothing by
// it. Directories and symlinks are recorded too: an empty directory left behind
// is still a write inside the repository under review.
func fileManifest(t *testing.T, dir string) (manifest string, permitted []string) {
	t.Helper()
	var entries []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)

		var entry string
		switch {
		case d.IsDir():
			entry = "dir  " + rel
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry = "link " + rel + " -> " + target
		default:
			info, err := d.Info()
			if err != nil {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(body)
			entry = fmt.Sprintf("file %s %04o %x", rel, info.Mode().Perm(), sum)
		}

		if rel == worktreeRegistration || strings.HasPrefix(rel, worktreeRegistration+"/") {
			permitted = append(permitted, entry)
			return nil
		}
		entries = append(entries, entry)
		return nil
	}))

	slices.Sort(entries)
	slices.Sort(permitted)
	require.Greater(t, len(entries), 10, "the fixture holds almost nothing, so a manifest of it proves nothing")
	return strings.Join(entries, "\n"), permitted
}

// fingerprintOf reads the repository's whole observable state.
//
// The manifest is taken on both sides of the git half and required to match,
// which is what makes the reading trustworthy: a fingerprint that changed the
// repository could not tell whether cr had.
func fingerprintOf(t *testing.T, dir string) repoState {
	t.Helper()
	opening, _ := fileManifest(t, dir)

	taken := repoState{
		status: mustGit(t, dir, "status", "--porcelain=v2", "--untracked-files=all", "--ignored=matching"),
		refs: strings.Join([]string{
			"head " + mustGit(t, dir, "rev-parse", "HEAD"),
			"following " + mustGit(t, dir, "rev-parse", "--symbolic-full-name", "HEAD"),
			"refs\n" + mustGit(t, dir, "for-each-ref", "--format=%(refname) %(objectname)"),
			"stash\n" + mustGit(t, dir, "stash", "list"),
		}, "\n"),
	}

	closing, permitted := fileManifest(t, dir)
	require.Equal(t, opening, closing,
		"reading the fingerprint changed the repository, so it cannot measure whether cr did")
	taken.files, taken.permitted = closing, permitted
	return taken
}

// leafCommands names every command in the tree that does work, spelled the way
// it is typed.
//
// It is read off the tree rather than listed, so a command added later arrives
// here by existing. Cobra's own `help` and `completion` are left out: they are
// not cr's commands, §11 does not name them, and neither reads a repository.
func leafCommands(t *testing.T) []string {
	t.Helper()
	var names []string
	var walk func(prefix string, cmd *cobra.Command)
	walk = func(prefix string, cmd *cobra.Command) {
		name := strings.TrimSpace(prefix + " " + cmd.Name())
		children := cmd.Commands()
		if len(children) == 0 {
			names = append(names, name)
			return
		}
		for _, sub := range children {
			walk(name, sub)
		}
	}
	for _, sub := range newRootCmd().Commands() {
		if sub.Name() == "help" || sub.Name() == "completion" {
			continue
		}
		walk("", sub)
	}

	require.NotEmpty(t, names, "a guard over none of the tree's commands proves nothing")
	return names
}

// ghShim writes a `gh` that answers the two reads `cr brief` performs, and
// returns the directory holding it.
//
// It stands in for the real binary rather than for internal/gh, so everything
// between the command and the transport runs exactly as it does in production:
// the read boundary of §2.1.2 judges the invocation, the environment allowlist
// is applied, and the answer is parsed by the same code. What it removes is the
// network, a token, and a pull request that would have to exist.
//
// The two revisions are the fixture's own, because §3.4.1 diffs the head
// against the merge base and git cannot resolve a commit the checkout does not
// hold.
func ghShim(t *testing.T, dir, head, base string) string {
	t.Helper()
	answers := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(answers, name)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		return path
	}
	pr := write("pr.json", `{"data":{"repository":{"pullRequest":{`+
		`"number":`+fixturePR+`,"title":"`+fixtureIssue+` retry the upload",`+
		`"body":"Closes `+fixtureIssue+`.","headRefName":"`+fixtureHeadBranch+`",`+
		`"headRefOid":"`+head+`","baseRefName":"main","baseRefOid":"`+base+`"}}}}`)
	threads := write("threads.json", `{"data":{"repository":{"pullRequest":{`+
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}}}`)

	shim := filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(shim, []byte(
		"#!/bin/sh\ncase \"$*\" in\n  *reviewThreads*) exec cat "+threads+
			" ;;\n  *) exec cat "+pr+" ;;\nesac\n"), 0o700))
	return dir
}

// repoRuns is the argv each command in the tree is exercised with, keyed by the
// command as it is typed.
//
// cr cannot invent a command's arguments, so this half is a table. What keeps
// it from freezing at today's five is that it is checked against the tree
// rather than trusted: a command added later fails the guard until someone
// gives it a real invocation, and "representative" therefore keeps meaning
// every command that exists.
//
// Every one of them is required to succeed. A command refused at its arguments
// never reaches the code that could write, so a run that exits 2 would prove
// nothing about the command it named.
//
// `cr record` and `cr claims record` each read an NDJSON file the agent names
// on the command line, and the latter also reads the issue text §3.1.4 puts in
// a file, so the argv carries paths and the table is built rather than written
// out. All three files are prepared outside the repository under review: a
// fixture written inside it would be a write, and this guard cannot tell one
// the test made from one cr made.
//
// `cr claims record` is given `--intent-file` for a second reason. Without it
// §3.1's default `intent.cmd` would start `jira`, and this guard would then be
// measuring whether a tracker CLI nobody installed writes into the repository.
func repoRuns(merged, claims, issue, cells, pairs, mutation, perRole, mergeOut string) map[string][]string {
	return map[string][]string{
		"init":    {"init"},
		"config":  {"config", "--repo", fixtureSlug},
		"note":    {"note", fixtureIssue, "the retry is deliberate", "--source", "chat", "--pr", fixturePR},
		"answer":  {"answer", fixturePR, "f1", "the retry is deliberate", "--source", "thread", "--repo", fixtureSlug},
		"context": {"context", fixtureIssue},
		"record":  {"record", fixturePR, merged, "--repo", fixtureSlug},
		"claims record": {
			"claims", "record", fixturePR, claims,
			"--repo", fixtureSlug, "--intent-file", issue,
		},
		"cells record": {"cells", "record", fixturePR, cells, "--repo", fixtureSlug},
		// `cr claims set-aside` writes intent-gaps.ndjson and reads the
		// §3.6 store, both under the state root, and reaches the
		// repository not at all. It runs last: §4.1.7 derives the entry
		// it stamps at `cr map record`, and the note it rests on is the
		// one `cr note` recorded, so runOrder defers it past both.
		"claims set-aside": {
			"claims", "set-aside", fixturePR, fixtureIssue + "#c1",
			"--note", fixtureIssue + "#n1", "--repo", fixtureSlug,
		},
		// `cr rules suggest` reads the state root and nothing else:
		// §2.6.3.1 scans the comments posted from recorded rounds,
		// which live under `~/.cr/`, and §2.6.3.3 forbids it to write
		// a rule file — so it should reach the repository neither to
		// read nor to write, and this is where that is checked.
		"rules suggest": {"rules", "suggest", "--repo", fixtureSlug},
		// `cr rules list --dead` reads the corpus and the rule ledger under
		// the state root and stats §2.4.1's marker files in the checkout to
		// select the profile, as `cr brief` does; it writes nothing at all.
		"rules list": {"rules", "list", "--dead", "--repo", fixtureSlug},
		// `cr stats` reads one file under the state root — §7.3's
		// repository-wide triage.ndjson — and §7.3.7 forbids it to act
		// on what it finds, so it should reach the repository under
		// review neither to read nor to write.
		"stats": {"stats", "--repo", fixtureSlug},
		// `cr draft` writes two files — the round's draft.md and the
		// findings whose state §9.1 moved — and §2.2 puts both under
		// the state root. runOrder puts it after `cr record`, so it
		// queues and renders the record that run stored.
		"draft":      {"draft", fixturePR, "--repo", fixtureSlug},
		"map record": {"map", "record", fixturePR, pairs, "--repo", fixtureSlug},
		// `cr merge` reads the §4.6.2 fan-out output files, whose names
		// bind their records to one role, so its input is named
		// review-<role>.ndjson rather than merged.ndjson. Both it and
		// the `-o` path sit outside the repository under review, like
		// every other file here. It sorts after `cr brief`, so the unit
		// its record names is one the round has formed.
		"merge": {
			"merge", perRole, "-o", mergeOut,
			"--repo", fixtureSlug, "--pr", fixturePR,
		},
		// `cr post` reads the round's draft back and builds the one
		// review §8.3.1 posts, and reads the repository only through
		// §7.2's location row — which no marker here moves. runOrder
		// puts it after `cr draft`, so the draft it reads is one this
		// run wrote, holding the record `cr record` stored: a round
		// with nothing queued is refused before a payload is built,
		// and building one is the step likeliest to write something.
		"post": {"post", fixturePR, "--repo", fixtureSlug},
		// `cr review` reads the repository as `cr brief` does: §4.6.1
		// carries the unit's hunks, so it takes §3.4.1's diff at the
		// recorded head, and §4.3.1 reads the head's blobs for the
		// symbol index. It sorts after `cr brief`, so there is a round
		// whose units the diff has to rebuild.
		"review": {"review", fixturePR, "--repo", fixtureSlug},
		// `cr brief` is the one command that reads the repository, so
		// it is the one this guard was widened for: §3.4.1 takes a
		// diff and §2.4.1 stats marker files, both inside the checkout
		// cr must not write to. `--issue` and `--intent-file` keep the
		// run off the tracker for the reason above.
		"brief": {
			"brief", fixturePR,
			"--repo", fixtureSlug, "--issue", fixtureIssue, "--intent-file", issue,
		},
		// `cr sandbox create` is the one command that writes inside
		// the repository under review at all, and §5.1.1 permits it
		// exactly the registration under `.git/worktrees/`. It runs
		// after `cr brief` — the run order below is alphabetical, and
		// `sandbox` sorts after `brief` — which is what gives it a
		// recorded head to check the worktree out at.
		"sandbox create": {"sandbox", "create", fixturePR, "--repo", fixtureSlug},
		// `cr sandbox destroy` takes that registration back out, and
		// is what has to leave `.git/worktrees/` as it found it. It
		// sorts between the two, so `cr test` then meets a pull
		// request with no sandbox and §5.1.6 rebuilds one — which is
		// the state this guard most wants covered, since a removal
		// that left a stale registration behind would make the
		// rebuild fail on the path it had just freed.
		"sandbox destroy": {"sandbox", "destroy", fixturePR, "--repo", fixtureSlug},
		// `cr test` runs a command inside the sandbox, which is the
		// case this guard exists for from the other direction: the
		// suite has to run in the worktree under `~/.cr` and never in
		// the checkout cr was invoked from. It sorts after `sandbox
		// create`, so there is a sandbox to run in.
		"test": {"test", fixturePR, "--repo", fixtureSlug},
		// `cr probe run` is the first command in cr that edits a file
		// inside a checkout, so it is the one this guard was widened
		// for from the sharpest direction: §5.3.2 applies a mutation
		// and §5.3.3 reverts it, and the checkout it does that in must
		// be the sandbox under `~/.cr` rather than the repository the
		// command was run from. The patch is prepared outside the
		// repository under review, like every other input here.
		"probe run": {
			"probe", "run", fixturePR, "--repo", fixtureSlug,
			"--kind", "mutation", "--patch", mutation,
		},
		// `cr rules check` reads the repository for the diff §2.6.1.1
		// evaluates, exactly as `cr brief` does, and it sorts after `cr
		// brief` in the run order, so the round it reads has been opened.
		"rules check": {"rules", "check", fixturePR, "--repo", fixtureSlug},
		// `cr status` reads the repository twice — §4.3.1's symbol index
		// at the round's head and §3.4.1's diff — because §10.1.3 has it
		// report the lens halves that could not run, and reads nothing
		// else there. It sorts after `cr brief`, so the round it counts
		// has been opened.
		"status": {"status", fixturePR, "--repo", fixtureSlug},
		// `cr waivers list` reads §7.4.4's two files, both under the state
		// root, and with `--pr` reaches GitHub for §9.3.1's head and the
		// repository not at all. `--pr` is given so the run reads the pull
		// request's file and its round too, which is the wider of the two
		// invocations. It sorts after `cr brief`, so there is a round.
		"waivers list": {"waivers", "list", "--repo", fixtureSlug, "--pr", fixturePR},
		// `cr waivers remove` rewrites one of §7.4.4's files under the
		// state root. The prepared waiver is the pull request's, so the run
		// reads the round and writes §2.3's waivers.ndjson — the wider of
		// the two scopes — and it sorts after `cr waivers list`, which
		// therefore meets the waiver still there.
		"waivers remove": {"waivers", "remove", "wp1", "--repo", fixtureSlug, "--pr", fixturePR},
	}
}

// runOrder is the sequence the runs are performed in: alphabetical, but with
// `cr sandbox create` hoisted to just behind `cr brief`.
//
// The two hoists are each forced by a section. `cr brief` writes the head every
// later command reads (§3.7), and a worktree cannot be checked out at a head no
// round has recorded. `cr sandbox create` refuses to build over a sandbox that
// is already there (§5.1.1), while §5.1.6 has `cr probe run` and `cr test`
// build one when they find none — so the one command that has to meet a pull
// request with no sandbox runs before either of them.
//
// Everything after is alphabetical, which still leaves `sandbox destroy`
// between the probe and `cr test`: the rebuild path this guard most wants
// covered, since a removal that left a stale registration behind would make the
// rebuild fail on the path it had just freed.
func runOrder(t *testing.T, runs map[string][]string) []string {
	t.Helper()
	hoisted := []string{"brief", "sandbox create"}
	// `cr claims set-aside` stamps an entry §4.1.7 derives at `cr map
	// record` and rests it on a note `cr note` records, and alphabetical
	// order puts it before both — so it goes last rather than earlier.
	// It is the mirror of a hoist and not a second kind of exception:
	// both say that one command's input is another command's output.
	// `cr draft` and `cr post` go after it for the same reason: the draft
	// queues the record `cr record` stores, and `cr post` builds its review
	// from that draft, since a round with nothing queued is refused before
	// any payload is built.
	deferred := []string{"claims set-aside", "draft", "post"}
	for _, name := range append(slices.Clone(hoisted), deferred...) {
		require.Contains(t, runs, name, "the ordered %s has no invocation to run", name)
	}
	order := slices.Clone(hoisted)
	for _, name := range slices.Sorted(maps.Keys(runs)) {
		if !slices.Contains(hoisted, name) && !slices.Contains(deferred, name) {
			order = append(order, name)
		}
	}
	return append(order, deferred...)
}

// No command in the tree writes inside the repository it is run in.
//
// This is the criterion with teeth, and the only one of the three guards that
// runs anything: the built binary, from inside a real repository, with the
// whole of §2.2's second sentence read off disk before and after. The two
// source guards above say what cr can be seen to do; this one says what it did.
//
// The honest limit is the command surface. v0.1 is five commands in, none of
// them yet drives git, and every one of them keeps its state under CR_HOME — so
// what this run proves today is narrower than what it will prove once `cr brief`
// and `cr review` read the repository. That is why the list of runs is derived
// from the tree instead of written out: the guard widens as the surface does,
// without anybody remembering it.
func TestNoCommandTouchesTheRepositoryUnderReview(t *testing.T) {
	fixture := fixtureRepository(t)
	binary := crBinary(t)
	home := t.TempDir()
	root := filepath.Join(home, ".cr")

	// `cr answer` answers a record of a pull request cr has been briefed
	// on, and that briefing is §2.3 state under CR_HOME rather than
	// anything in the repository. It is prepared here so the command runs
	// to completion instead of refusing at its first read.
	prepared := state.New(root)
	require.NoError(t, prepared.Init())
	// §2.4.5's profiles, and the per-repository `profile` §2.4.1 makes the
	// override. `cr brief` runs before `cr cells record` here and settles
	// meta.json's active roles from the resolved profile, and the fixture
	// carries no marker file of its own — so without a profile every axis
	// would be off, no role would be active, and §4.5.6 would refuse every
	// cell before the command reached anything this guard measures.
	for id, body := range profile.Builtins() {
		require.NoError(t, prepared.EnsureProfile(id, body))
	}
	// `cr test` needs a profile that names a test command: §2.4 makes an
	// absent `tests.cmd` a disabled test axis and §5.2.1 then has nothing to
	// run. The shipped `generic` declares none, so this run's copy of it
	// gains one that exits 0 and writes nothing — what is measured here is
	// where the command ran, not what it did.
	require.NoError(t, os.WriteFile(prepared.Profile("generic"), []byte(
		`{"id":"generic","match":{"files":[],"globs":["**/*"]},`+
			`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},`+
			`"tests":{"cmd":["true"],"globs":["*_test.txt"]}}`), 0o600))
	require.NoError(t, prepared.EnsureRepo(fixtureOwner, fixtureProject))
	require.NoError(t, os.WriteFile(
		prepared.RepoConfig(fixtureOwner, fixtureProject), []byte(`{"profile":"generic"}`), 0o600))
	require.NoError(t, prepared.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 1, Head: fixtureHead,
	}))
	// `cr record` checks every record's unit against the units of the round
	// (§6.1.3), so the round has one. It is written as bytes rather than
	// through a unit type because §3.4.6's record does not exist yet, and
	// the id is the whole of what this command reads.
	require.NoError(t, held.Write(state.FileUnits,
		[]byte(`{"id":"u1","head":"`+fixtureHead+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	// `cr waivers remove` deletes a waiver that exists, so one is there to
	// delete: a `not-here` over a file no other run anchors in, so no merge
	// or draft here drops anything against it.
	_, err = finding.Waive(prepared, fixtureOwner, fixtureProject, &finding.Waiver{
		WaiverKey: finding.WaiverKey{
			Path: "unrelated.go", Side: "RIGHT", Class: "unchecked-error", ContentHash: "fedcba9876543210",
		},
		Disposition: finding.DispositionNotHere,
	}, finding.WaiverProvenance{Round: 1, PR: fixturePRNumber, Head: fixtureHead})
	require.NoError(t, err)

	// The file `cr record` is pointed at, outside the repository under
	// review for the reason repoRuns gives. Its anchor is app.go's changed
	// line 3, inside the one unit `cr brief` forms before `cr record` runs:
	// `cr record` binds a record's anchor to the unit it names.
	merged := filepath.Join(home, "merged.ndjson")
	require.NoError(t, os.WriteFile(merged, []byte(`{"id":"f1","kind":"finding",`+
		`"role":"correctness","class":"unchecked-error","severity":"high","unit":"u1",`+
		`"anchor":{"path":"app.go","side":"RIGHT","start_line":3,"line":3,"content_hash":"0123456789abcdef"},`+
		`"summary":"Is the returned error dropped?",`+
		`"evidence":"The call's second result is assigned to the blank identifier."}`+"\n"), 0o600))

	// The two files `cr claims record` reads, outside the repository under
	// review for the same reason. The claim's span is a substring of the
	// issue text, so §3.3.1's occurrence check has something to find.
	issue := filepath.Join(home, "issue.txt")
	require.NoError(t, os.WriteFile(issue,
		[]byte("The retry must back off exponentially.\n"), 0o600))
	// The file `cr cells record` is pointed at, outside the repository for
	// the same reason. It names the round's one unit and its one active
	// role, so §4.5.6 accepts it, and it says `finding` because `cr record`
	// in the run below stores that role's record on that unit, which a
	// `pass` there would contradict in either order.
	cells := filepath.Join(home, "cells.ndjson")
	require.NoError(t, os.WriteFile(cells,
		[]byte(`{"unit":"u1","role":"correctness","result":"finding"}`+"\n"), 0o600))

	// The file `cr map record` is pointed at. It is empty rather than a
	// pair, because §4.1.6 checks every claim id against the round's
	// claims and `cr claims record` sorts after `cr map record` in the run
	// order below — an empty mapping is a real §4.1.1 answer, since every
	// unit may be mapped to zero claims.
	pairs := filepath.Join(home, "mapping.ndjson")
	require.NoError(t, os.WriteFile(pairs, nil, 0o600))

	// The unified diff `cr probe run` applies, outside the repository under
	// review for the same reason. It breaks the function the pull request's
	// head branch introduced, which is the shape §5.3.1 describes: a
	// mutation of production code the suite is expected to notice.
	mutation := filepath.Join(home, "mutation.diff")
	require.NoError(t, os.WriteFile(mutation, []byte(
		"--- a/app.go\n+++ b/app.go\n@@ -1,3 +1,3 @@\n package app\n \n"+
			"-func Retry() { backoff() }\n+func Retry() {}\n"), 0o600))

	// The file `cr merge` is pointed at, outside the repository under
	// review for the same reason, and named for the role whose §4.6.2
	// output file it stands in for — finding.DecodePerRole refuses an
	// input whose name binds its records to no role.
	perRole := filepath.Join(home, "review-correctness.ndjson")
	require.NoError(t, os.WriteFile(perRole, []byte(`{"id":"f1","kind":"finding",`+
		`"role":"correctness","class":"unchecked-error","severity":"high","unit":"u1",`+
		`"anchor":{"path":"app.go","side":"RIGHT","start_line":3,"line":3,"content_hash":"0123456789abcdef"},`+
		`"summary":"The returned error is dropped.",`+
		`"evidence":"The call's second result is assigned to the blank identifier."}`+"\n"), 0o600))

	claims := filepath.Join(home, "claims.ndjson")
	require.NoError(t, os.WriteFile(claims, []byte(`{"id":"`+fixtureIssue+`#c1",`+
		`"text":"The retry backs off exponentially.","source":"acceptance",`+
		`"span":"back off exponentially"}`+"\n"), 0o600))

	// `cr brief` reads GitHub, so a `gh` that answers without a network
	// call goes first on the PATH. It answers with the fixture's own two
	// revisions, which is what lets §3.4.1 resolve a merge base at all.
	shims := ghShim(t, t.TempDir(),
		strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch)),
		strings.TrimSpace(mustGit(t, fixture, "rev-parse", "main")))
	env := []string{
		"PATH=" + shims + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + home,
		"TMPDIR=" + t.TempDir(),
		"CR_HOME=" + root,
	}
	run := func(args ...string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(binary, args...)
		// The repository under review is the directory cr is run from,
		// which is what would make a relative path inside cr land in it.
		cmd.Dir = fixture
		cmd.Env = env
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		require.NoErrorf(t, cmd.Run(), "cr %s: %s",
			strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}

	runs := repoRuns(merged, claims, issue, cells, pairs, mutation,
		perRole, filepath.Join(home, "merge-out.ndjson"))
	commands := leafCommands(t)
	require.ElementsMatch(t, commands, slices.Collect(maps.Keys(runs)),
		"every command in the tree is run against the fixture, so a new one needs an invocation here")

	before := fingerprintOf(t, fixture)

	run("--version")
	run("--help")
	for _, name := range runOrder(t, runs) {
		run(append(strings.Fields(name), "--help")...)
		run(runs[name]...)
	}

	after := fingerprintOf(t, fixture)
	assert.Equal(t, before.status, after.status,
		"§2.2: git status must be byte-identical across a run of every command")
	assert.Equal(t, before.refs, after.refs,
		"§2.2: cr must not modify the repository's HEAD, stash, or any branch")
	assert.Equal(t, before.files, after.files,
		"§2.2: cr must not write inside the repository under review")

	// §5.1.1's exception, now exercised. `.git/worktrees/` is held out of
	// the manifest above, so the registration `cr sandbox create` leaves
	// behind passes the three comparisons while anything wider still fails
	// them. What is asserted here is the exception itself, and it is
	// asserted from both ends: nothing was in it before the run, so every
	// entry after the run is cr's; and every one of those entries sits
	// under the single registration directory the sandbox's own name gives
	// it. A second registration, a file dropped beside them, or a write
	// anywhere else under `.git/worktrees/` fails here.
	if !slices.ContainsFunc(commands, func(name string) bool { return strings.HasPrefix(name, "sandbox") }) {
		assert.Empty(t, after.permitted,
			"§5.1.1 gives the exception to the sandbox, and no sandbox command exists")
		return
	}
	assert.Empty(t, before.permitted,
		"the fixture must start with no registration, or what is in it afterwards is not cr's")
	require.NotEmpty(t, after.permitted,
		"§5.1.1's exception went unused, so this guard measured nothing about the one write §2.2 permits")

	// The registration git names after the sandbox directory, which is the
	// directory internal/state derives. Reading the name from there rather
	// than spelling it keeps the two from drifting apart.
	registration := worktreeRegistration + "/" + state.DirSandbox
	for _, entry := range after.permitted {
		// A manifest entry is a kind and a repository-relative path,
		// whitespace-separated; the path is the second field of each
		// of fileManifest's three shapes.
		written := strings.Fields(entry)[1]
		assert.True(t,
			written == worktreeRegistration ||
				written == registration ||
				strings.HasPrefix(written, registration+"/"),
			"§2.2 permits the worktree registration and nothing else, and cr wrote %s", written)
	}
}

// The fingerprint sees every change §2.2 names, including the ones `git status`
// alone would report as nothing.
//
// The guard above is an assertion that two readings match, and such an
// assertion is worth exactly what the reading is worth: a fingerprint blind to
// what it is watching for would pass forever. So each of §2.2's five is
// disturbed here on a fixture of its own and the fingerprint is required to
// notice.
//
// Two of them are the reason the reading is more than the criterion's literal
// words. `git status` reports the working tree against HEAD, so it says the
// same thing before and after a branch is created off the current commit, and
// the same thing before and after HEAD is pointed at an identical commit — a
// force-push, a rebase onto the same tree, or a checkout cr had no business
// doing would all pass a comparison of `git status` alone. Those two cases are
// asserted from both sides: status unchanged, fingerprint changed.
func TestTheFingerprintSeesWhatGitStatusAloneWouldMiss(t *testing.T) {
	for name, disturbance := range map[string]struct {
		change    func(t *testing.T, dir string)
		seenByGit bool
		assertion string
	}{
		"a tracked file edited": {
			change: func(t *testing.T, dir string) {
				t.Helper()
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "app.go"), []byte("package app\n"), 0o600))
			},
			seenByGit: true,
			assertion: "§2.2: a tracked file",
		},
		"the index moved": {
			change: func(t *testing.T, dir string) {
				t.Helper()
				mustGit(t, dir, "add", "scratch.txt")
			},
			seenByGit: true,
			assertion: "§2.2: the index",
		},
		"the stash pushed to": {
			change: func(t *testing.T, dir string) {
				t.Helper()
				mustGit(t, dir, "stash", "push", "--quiet", "-m", "a second entry", "--", "app.go")
			},
			seenByGit: true,
			assertion: "§2.2: the stash",
		},
		"a branch created off the current commit": {
			change: func(t *testing.T, dir string) {
				t.Helper()
				mustGit(t, dir, "branch", "another")
			},
			seenByGit: false,
			assertion: "§2.2: any branch",
		},
		"HEAD pointed at an identical commit": {
			change: func(t *testing.T, dir string) {
				t.Helper()
				mustGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/spare")
			},
			seenByGit: false,
			assertion: "§2.2: HEAD",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := fixtureRepository(t)
			before := fingerprintOf(t, dir)
			disturbance.change(t, dir)
			after := fingerprintOf(t, dir)

			assert.NotEqual(t, before, after,
				"%s went unnoticed, so %s is not really guarded", name, disturbance.assertion)
			if !disturbance.seenByGit {
				assert.Equal(t, before.status, after.status,
					"%s is invisible to git status, which is why the fingerprint reads more than it", name)
			}
		})
	}
}
