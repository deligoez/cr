package cli

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
)

// execProgram reports the program an exec.Command or exec.CommandContext call
// starts, and whether the call starts one at all.
//
// A program that is not a plain string literal is reported as unfixed rather
// than passed over. The reader of a runner cannot say which command such a
// call runs, so neither can this guard, and the unanswerable case is the one
// that has to fail. An aliased import of os/exec fails the same way, from the
// other direction: the call is then invisible here and the runner it sits in
// goes missing from the result.
func execProgram(call *ast.CallExpr) (string, bool) {
	fun, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkg, ok := fun.X.(*ast.Ident)
	if !ok || pkg.Name != "exec" {
		return "", false
	}
	// The program is the first argument of Command and the second of
	// CommandContext, which takes the context ahead of it.
	var at int
	switch fun.Sel.Name {
	case "Command":
		at = 0
	case "CommandContext":
		at = 1
	default:
		return "", false
	}
	if len(call.Args) <= at {
		return "", false
	}
	literal, ok := call.Args[at].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "a program named at run time", true
	}
	name, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "a program named at run time", true
	}
	return name, true
}

// §14.1 gives cr three runtime dependencies, and two of them are programs cr
// starts itself.
//
// TestCrReachesTheNetworkThroughOneRunnerAndNoOtherWay already fixes where a
// process may be started: os/exec is imported by the two files in runners and
// by nothing else. This fixes what those two start, which is a different
// claim that the first does not imply — an import scan reads
// exec.Command(helper, ...) exactly as it reads exec.Command("git", ...), so
// a fourth dependency introduced inside a runner passes it untouched.
//
// Requiring a literal costs the runners nothing, because each exists to drive
// exactly one command, and the result below says so: two runners, two
// invocations, one program each.
func TestTheRunnersStartNothingButGitAndGh(t *testing.T) {
	root := moduleRoot(t)

	var found []string
	for rel := range runners {
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, rel), nil, 0)
		require.NoError(t, err)

		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if program, isExec := execProgram(call); isExec {
				found = append(found, rel+" starts "+program)
			}
			return true
		})
	}

	slices.Sort(found)
	assert.Equal(t, []string{
		filepath.Join("internal", "gh", "run.go") + " starts gh",
		filepath.Join("internal", "git", "run.go") + " starts git",
	}, found, "§14.1: git and gh are the only programs cr starts, one to each runner")
}

// crBinary builds cr and returns the path to the binary it produced.
//
// §14.1's first claim is that there is one binary: one build command, one
// output file, and the run below is then a run of that file and of nothing
// standing beside it. A build that fails fails the test rather than skipping
// it, because a binary that was never produced says nothing about what a
// binary depends on.
func crBinary(t *testing.T) string {
	t.Helper()
	gotool, err := exec.LookPath("go")
	require.NoError(t, err, "the go toolchain is how this guard builds the binary it runs")

	binary := filepath.Join(t.TempDir(), "cr")
	build := exec.Command(gotool, "build", "-o", binary, "./cmd/cr")
	build.Dir = moduleRoot(t)
	out, err := build.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", out)

	info, err := os.Stat(binary)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular(), "§14.1: cr is one file, not a tree of them")
	return binary
}

// strippedPath assembles the PATH the binary is run on: one directory holding
// a symlink to git, a symlink to gh, and nothing else whatsoever.
//
// A missing git or gh fails rather than skips. They are §14.1's declared
// dependencies, so a machine without them cannot run cr at all, and a guard
// that quietly passed there would be reporting on a claim it never checked.
func strippedPath(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "path")
	require.NoError(t, os.Mkdir(dir, 0o700))

	for _, name := range []string{"git", "gh"} {
		resolved, err := exec.LookPath(name)
		require.NoErrorf(t, err, "§14.1 makes %s a runtime dependency of cr", name)
		require.NoError(t, os.Symlink(resolved, filepath.Join(dir, name)))
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	require.Equal(t, []string{"gh", "git"}, names,
		"the run proves nothing unless the PATH it runs on really holds nothing else")
	return dir
}

// cr runs with git and gh on PATH and nothing else, which is §14.1 asserted as
// a run rather than as a reading of the source.
//
// It is the other half of TestTheRunnersStartNothingButGitAndGh, and neither
// half restates the other. That one reads what the source names and cannot see
// a program reached by an absolute path, which needs no PATH entry at all;
// this one runs the binary and cannot see a dependency the exercised commands
// never take. §14.1 is worth what the two are worth together.
//
// The tracker is the third dependency and is deliberately absent from the
// PATH. §3.1.1 makes it argv the user configures, §3.1.4 lets --intent-file
// replace it outright, and no command in the tree today reads the intent
// source at all, so a run that demanded it on PATH would assert something
// §3.1 does not say. cr config is the proof rather than the omission: it
// prints §3.1.2's default tracker argv while that program is unreachable,
// which is what makes the default a default and not a dependency.
func TestTheBinaryRunsWithNothingButGitAndGhOnPath(t *testing.T) {
	binary := crBinary(t)
	path := strippedPath(t)
	home := t.TempDir()

	defaults, err := config.Resolve(config.Sources{})
	require.NoError(t, err)
	tracker := defaults.Strings("intent.cmd")
	require.NotEmpty(t, tracker, "§3.1.2 gives intent.cmd a default, and it names the program")
	require.NoFileExists(t, filepath.Join(path, tracker[0]),
		"§3.1's tracker is configured by the user, not installed alongside cr")

	// The environment is the allowlist internal/git and internal/gh already
	// settled on — PATH, HOME, TMPDIR — with CR_HOME added so §2.2's tree
	// lands in the temporary home rather than in the user's own.
	root := filepath.Join(home, ".cr")
	env := []string{
		"PATH=" + path,
		"HOME=" + home,
		"TMPDIR=" + t.TempDir(),
		"CR_HOME=" + root,
	}
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(binary, args...)
		cmd.Env = env
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		require.NoErrorf(t, cmd.Run(), "cr %s: %s",
			strings.Join(args, " "), strings.TrimSpace(stderr.String()))
		return stdout.String()
	}

	assert.Contains(t, run(t, "--version"), "cr version")
	assert.Contains(t, run(t, "--help"), "Usage:")

	// Every registered command answers --help, so a command added later is
	// exercised here by existing rather than by someone remembering it.
	registered := newRootCmd().Commands()
	require.NotEmpty(t, registered, "a guard over none of the tree's commands proves nothing")
	for _, sub := range registered {
		assert.Contains(t, run(t, sub.Name(), "--help"), sub.Name())
	}

	// init and config are the commands that do their work with no argument
	// today, and they are the two worth running in full: init writes the
	// whole §2.2 tree, and config resolves every layer of §2.7.
	assert.Contains(t, run(t, "init"), root)
	assert.DirExists(t, filepath.Join(root, "profiles"))

	resolved := run(t, "config")
	assert.Contains(t, resolved, "intent.cmd")
	assert.Contains(t, resolved, tracker[0])
}
