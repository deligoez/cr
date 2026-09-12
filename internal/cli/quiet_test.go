package cli

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// §11.1's seven, each run in the situation that produces it with `--quiet`
// given, and read off a terminal: the text shape is the one the flag acts on,
// since §12.1's document is the same with and without it.
//
// Each case builds its own state, so a disclosure is asserted over a run that
// had it to make rather than over a fixture shared with a case that did not.
func TestEveryHonestyDisclosureSurvivesQuiet(t *testing.T) {
	t.Run("every lens of 4.5.4 that did not run", func(t *testing.T) {
		completeStatusHome(t)
		shown := throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--quiet")
		for _, lens := range []string{
			"axis test disabled, per §4.5.2",
			"lens convention/reinvention unavailable, per §4.3.1",
			"lens test/symbols unavailable, per §4.5.4",
			"role test-adequacy skipped, per §4.6.4",
		} {
			assert.Contains(t, shown, lens)
		}
	})

	t.Run("the probe cap of 5.6.4", func(t *testing.T) {
		probeFixture(t, "echo 'Tests:  4 passed'\n")
		probing := []string{"probe", "run", fixturePR, "--repo", fixtureSlug,
			"--kind", "mutation", "--patch", writePatch(t, fixtureDiff)}
		for i := 1; i < 10; i++ {
			require.NoError(t, runCLI(t, probing...), "probe %d", i)
		}
		shown := throughATerminal(t, append(probing, "--quiet")...)
		assert.Contains(t, shown, "10 of 10 probes run this round")
	})

	t.Run("the forcing counts of 6.3.2", func(t *testing.T) {
		draftedHome(t, anArguedRecord("f1", "unchecked-error"))
		shown := throughATerminal(t, "draft", draftPR, "--repo", draftSlug, "--quiet")
		assert.Contains(t, shown, "§6.3: 1 records forced to question")
	})

	t.Run("the waiver and duplicate counts of 10.1.6", func(t *testing.T) {
		layout := recordedHome(t)
		waived := finding.Waiver{
			WaiverKey: finding.WaiverKey{
				Path: "internal/api/handler.go", Side: "RIGHT",
				Class: "unchecked-error", ContentHash: "0123456789abcdef",
			},
			Disposition: finding.DispositionNotHere,
		}
		_, err := finding.Waive(layout, recordOwner, recordRepo, &waived,
			finding.WaiverProvenance{Round: recordRound, PR: recordPRNum, Head: recordHead})
		require.NoError(t, err)
		file := writeFanOut(t, t.TempDir(), "correctness",
			aRoleRecord("f1", "correctness", "unchecked-error", "u1"),
			aRoleRecord("f2", "correctness", "missing-test", "u2"))

		printed, err := runMergeCLI(t, mergedOut(t), file)
		require.NoError(t, err)
		reported := decodeMergeResult(t, printed)
		require.NotEmpty(t, reported.Honesty)

		shown := throughATerminal(t, "merge", file, "-o", mergedOut(t),
			"--repo", recordSlug, "--pr", recordPR, "--quiet")
		for _, disclosed := range reported.Honesty {
			assert.Contains(t, shown, disclosed)
		}
		assert.Contains(t, shown, reported.Waived.Disclosure())
	})

	t.Run("the sandbox recreation notice of 5.1.6", func(t *testing.T) {
		_, _, sandboxPath, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
		shown := throughATerminal(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
			"--kind", "mutation", "--patch", writePatch(t, fixtureDiff), "--quiet")
		assert.Contains(t, shown, "§5.1.6")
		assert.Contains(t, shown, sandboxPath)
	})

	t.Run("the stale-round report of 9.3.2", func(t *testing.T) {
		_, recorded, moved := aRoundTheHeadOutran(t)
		shown := throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--quiet")
		assert.Contains(t, shown, "opened at head "+recorded)
		assert.Contains(t, shown, "current head is "+moved)
	})

	t.Run("the comment cap of 1.6.2", func(t *testing.T) {
		queued := make([]*finding.Finding, 0, 21)
		for range 21 {
			queued = append(queued, &finding.Finding{Kind: finding.KindFinding})
		}
		blocked := finding.CommentCapFor(queued, 20).Err()
		require.Error(t, blocked)
		_, terminal, err := pty.Open()
		require.NoError(t, err)
		t.Cleanup(func() { terminal.Close() })

		// The cap reaches the reader as `cr post`'s refusal, which Execute
		// hands to reportFailure. It takes the arguments, `--quiet`
		// among them, and has no flag it could consult.
		var stderr bytes.Buffer
		require.NoError(t, reportFailure(terminal, &stderr,
			[]string{"post", draftPR, "--repo", draftSlug, "--quiet"}, blocked))
		assert.Contains(t, stderr.String(), "21 comments")
		assert.Contains(t, stderr.String(), "post.max_comments 20")
	})
}

// §11.1's other half: `--quiet` does suppress something. A test run's live
// output is the informational message cr writes, and under the flag it stops
// reaching the terminal while the disclosure beside it does not.
func TestQuietSuppressesTheLiveRunOutputAndNotTheDisclosure(t *testing.T) {
	const noise = "informational-runner-noise"
	runner := "echo " + noise + "\necho 'Tests:  4 passed'\n"

	for name, quiet := range map[string]bool{"loud": false, "quiet": true} {
		t.Run(name, func(t *testing.T) {
			_, _, _, log := probeFixture(t, runner)
			args := []string{"probe", "run", fixturePR, "--repo", fixtureSlug,
				"--kind", "mutation", "--patch", writePatch(t, fixtureDiff)}
			if quiet {
				args = append(args, "--quiet")
			}

			shown := throughATerminal(t, args...)

			observed, err := os.ReadFile(log)
			require.NoError(t, err, "the runner never ran, so the noise could not have been printed")
			require.NotEmpty(t, observed)
			assert.Contains(t, shown, "§5.1.6", "the disclosure is printed either way")
			if quiet {
				assert.NotContains(t, shown, noise, "§11.1: --quiet suppresses the informational echo")
			} else {
				assert.Contains(t, shown, noise, "without --quiet the run is watched as it happens")
			}
		})
	}
}

// The exemption is a property of the writer, not a habit of its call sites,
// and this reads the source to hold it there.
//
// Three rules. `quiet` is read in output.go's settle and informational and
// nowhere else, so no rendering can be taught to consult it. A result's
// rendering — its Text, or any method of the same type taking the writer —
// never ranges over its own Honesty and never prints a Disclosure() itself:
// either reaches the reader only as an argument of the writer's disclose, whose
// body names no flag. `len(r.Honesty)` is allowed, being a count rather than a
// print.
func TestEveryDisclosureAResultPrintsGoesThroughTheWriter(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "internal", "cli")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	quietReads := make([]string, 0)
	bypasses := make([]string, 0)
	renderings := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		require.NoError(t, err)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			for _, read := range selectorsNamed(fn.Body, "quiet") {
				if name != "output.go" || (fn.Name.Name != "settle" && fn.Name.Name != "informational") {
					quietReads = append(quietReads, fset.Position(read.Pos()).String())
				}
			}
			if !rendersAResult(fn) {
				continue
			}
			renderings++
			bypasses = append(bypasses, disclosuresOutsideTheWriter(fset, fn.Body)...)
		}
	}

	require.Greater(t, renderings, 10, "the walk found too few renderings to prove anything")
	assert.Empty(t, quietReads,
		"§11.1: --quiet is consulted only where informational messages are written")
	assert.Empty(t, bypasses,
		"§11.1: a disclosure a rendering prints goes through writer.disclose, which reads no flag")
}

// rendersAResult reports whether fn is a result's rendering: a method whose
// parameters include the *writer.
func rendersAResult(fn *ast.FuncDecl) bool {
	if fn.Recv == nil {
		return false
	}
	for _, param := range fn.Type.Params.List {
		star, ok := param.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if ident, ok := star.X.(*ast.Ident); ok && ident.Name == "writer" {
			return true
		}
	}
	return false
}

// selectorsNamed is every selector expression under node whose selected name is
// name.
func selectorsNamed(node ast.Node, name string) []*ast.SelectorExpr {
	found := make([]*ast.SelectorExpr, 0)
	ast.Inspect(node, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
			found = append(found, sel)
		}
		return true
	})
	return found
}

// disclosuresOutsideTheWriter is every `.Honesty` or `.Disclosure` under body
// that is not inside a disclose call's arguments or a len call.
func disclosuresOutsideTheWriter(fset *token.FileSet, body *ast.BlockStmt) []string {
	sheltered := make(map[ast.Node]bool)
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		shelters := false
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "disclose" {
			shelters = true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "len" {
			shelters = true
		}
		if shelters {
			for _, arg := range call.Args {
				ast.Inspect(arg, func(inner ast.Node) bool {
					sheltered[inner] = true
					return true
				})
			}
		}
		return true
	})
	found := make([]string, 0)
	for _, name := range []string{"Honesty", "Disclosure"} {
		for _, sel := range selectorsNamed(body, name) {
			if !sheltered[sel] {
				found = append(found, fset.Position(sel.Pos()).String()+" "+name)
			}
		}
	}
	return found
}
