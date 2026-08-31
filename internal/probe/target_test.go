package probe

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
)

// §5.3.2's derivation, and round 12's probe-target-underdetermined,
// probe-target-steering and undefined-derivation-branch findings with it: the
// target has to be single-valued for every unified diff, fixed by the patch's
// content rather than by the order its author wrote it in.
//
// The multi-file and multi-hunk cases are written out of order deliberately.
// An agent that wanted the evidence to point somewhere other than where the
// mutation lands could otherwise get it there by moving a hunk up its own
// patch, and the assertion is that it cannot: the same change spelled two ways
// derives one target.
func TestTheMutationTargetIsDerivedFromThePatchAlone(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch string
		want  string
	}{
		{
			name: "a replaced line gives its pre-image number",
			patch: "--- a/app.go\n+++ b/app.go\n@@ -1,3 +1,3 @@\n package app\n \n" +
				"-func Retry() { backoff() }\n+func Retry() {}\n",
			want: "app.go:3",
		},
		{
			name:  "a removed line deeper in a hunk counts the context above it",
			patch: "--- a/app.go\n+++ b/app.go\n@@ -10,5 +10,4 @@\n a\n b\n-c\n d\n e\n",
			want:  "app.go:12",
		},
		{
			name:  "a -U0 hunk names the line it removes",
			patch: "--- a/app.go\n+++ b/app.go\n@@ -41 +41 @@\n-old\n+new\n",
			want:  "app.go:41",
		},
		{
			name: "the lowest pre-image start wins when the hunks descend",
			patch: "--- a/app.go\n+++ b/app.go\n@@ -40 +40 @@\n-late\n+LATE\n" +
				"@@ -8 +8 @@\n-early\n+EARLY\n",
			want: "app.go:8",
		},
		{
			name: "and when they ascend, which is the order git writes",
			patch: "--- a/app.go\n+++ b/app.go\n@@ -8 +8 @@\n-early\n+EARLY\n" +
				"@@ -40 +40 @@\n-late\n+LATE\n",
			want: "app.go:8",
		},
		{
			name: "the lowest path wins when the files descend",
			patch: "--- a/z.go\n+++ b/z.go\n@@ -2 +2 @@\n-z\n+Z\n" +
				"--- a/a.go\n+++ b/a.go\n@@ -90 +90 @@\n-a\n+A\n",
			want: "a.go:90",
		},
		{
			name: "and when they ascend, which is the order git writes",
			patch: "--- a/a.go\n+++ b/a.go\n@@ -90 +90 @@\n-a\n+A\n" +
				"--- a/z.go\n+++ b/z.go\n@@ -2 +2 @@\n-z\n+Z\n",
			want: "a.go:90",
		},
		{
			name:  "an add-only hunk with context names its first pre-image line",
			patch: "--- a/app.go\n+++ b/app.go\n@@ -12,2 +12,3 @@\n one\n+two\n three\n",
			want:  "app.go:12",
		},
		{
			name: "a file with no hunk is passed over",
			patch: "--- a/mode.sh\n+++ b/mode.sh\n" +
				"--- a/app.go\n+++ b/app.go\n@@ -5 +5 @@\n-x\n+y\n",
			want: "app.go:5",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files, err := git.ParsePatch(tc.patch)
			require.NoError(t, err)

			target, err := Target(files)
			require.NoError(t, err)
			assert.Equal(t, tc.want, target,
				"§5.3.2 derives the target from the patch itself")
		})
	}
}

// The branch §5.3.2 leaves undefined, refused rather than guessed at.
//
// A zero-context add-only hunk covers no pre-image line: `@@ -41,0 +42 @@`
// inserts after line 41 and holds no line of its own, so §5.3.2's "its first
// pre-image line" names nothing. Pointing at 41 anyway would put the target on
// code the patch does not touch — and §6.2.2 has a `probed` record's evidence
// point at the target, so that line would be asserted to a colleague as the
// place the experiment was performed.
func TestAPatchNoTargetFollowsFromIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch string
		want  string
		// path is the file the refusal names, and empty for the one
		// refusal that is about the patch rather than about a file in
		// it. A reader who is told a path can open it; one who is told
		// a path that means nothing is worse off than one told none.
		path string
	}{
		{
			name:  "a zero-context add-only first hunk",
			patch: "--- a/app.go\n+++ b/app.go\n@@ -41,0 +42 @@\n+added\n",
			want:  "covers no pre-image line",
			path:  "app.go",
		},
		{
			name:  "a patch that creates the file",
			patch: "--- /dev/null\n+++ b/app.go\n@@ -0,0 +1 @@\n+added\n",
			want:  "no pre-image line to name",
			path:  "app.go",
		},
		{
			name:  "a patch holding no hunk at all",
			patch: "--- a/mode.sh\n+++ b/mode.sh\n",
			want:  "holds no hunk",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files, err := git.ParsePatch(tc.patch)
			require.NoError(t, err)

			target, err := Target(files)
			var refused *TargetError
			require.ErrorAs(t, err, &refused)
			assert.Empty(t, target)
			assert.Contains(t, refused.Error(), tc.want)
			assert.Contains(t, refused.Error(), "§5.3.2")
			assert.Equal(t, tc.path, refused.Path)
			if tc.path == "" {
				assert.NotContains(t, refused.Error(), " at ",
					"a refusal about the whole patch names no file")
				return
			}
			assert.Contains(t, refused.Error(), " at "+tc.path,
				"a refusal about one file names it, so the reader can open it")
		})
	}
}

// headHolding answers as the head under review would for one file.
func headHolding(path string, lines ...string) HeadFile {
	return func(asked string) ([]string, bool, error) {
		if asked != path {
			return nil, false, nil
		}
		return lines, true, nil
	}
}

// §5.5's `path:line`, read the way cr writes one.
//
// The rejected spellings are the ones that would let a reader believe cr
// checked something it did not: a line number cr would never have written, and
// a value carrying no path at all. `pkg/app.go:12:3` is the case the last colon
// answers — an editor's `path:line:column`, whose line is not what cr means by
// one.
func TestAGapProbeTargetIsAPathAndALineNumber(t *testing.T) {
	for name, tc := range map[string]struct {
		target string
		path   string
		line   int
	}{
		"a path and a line":           {target: "app.go:3", path: "app.go", line: 3},
		"a path with directories":     {target: "src/pkg/app.go:12", path: "src/pkg/app.go", line: 12},
		"the first line is a line":    {target: "app.go:1", path: "app.go", line: 1},
		"the last colon separates":    {target: "app.go:12:3", path: "app.go:12", line: 3},
		"a colon in a path is a path": {target: "a:b/app.go:7", path: "a:b/app.go", line: 7},
		"no colon at all":             {target: "app.go"},
		"no path":                     {target: ":3"},
		"no line":                     {target: "app.go:"},
		"a line that is not a number": {target: "app.go:three"},
		"line zero is not a line":     {target: "app.go:0"},
		"a negative line":             {target: "app.go:-2"},
		"a signed line":               {target: "app.go:+3"},
		"a padded line":               {target: "app.go:03"},
		"a spaced line":               {target: "app.go: 3"},
		"a line beyond an int":        {target: "app.go:99999999999999999999"},
	} {
		t.Run(name, func(t *testing.T) {
			path, line, err := ParseTarget(tc.target)
			if tc.path == "" {
				var refused *InvalidTargetError
				require.ErrorAs(t, err, &refused)
				assert.Equal(t, tc.target, refused.Target)
				assert.Contains(t, refused.Error(), "§6.2.3",
					"§5.5 sends the reader to the rule the target is validated by")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.path, path)
			assert.Equal(t, tc.line, line)
		})
	}
}

// §5.5 has a gap probe's target "supplied and validated as §6.2.3 validates a
// citation", and §6.2.3 has exactly two refusals: a path the head does not hold
// as a file, and a line the file does not have.
//
// Both boundaries are asserted from both sides, because they are the whole of
// what the check establishes. §6.2.3 is explicit that validation proves a
// location **exists** and never that it supports anything, so a target inside
// the file is accepted whatever is written on that line.
func TestAGapProbeTargetIsResolvedAgainstTheHead(t *testing.T) {
	head := headHolding("app.go", "package app", "", "func Retry() {}")

	require.NoError(t, CheckTarget(head, "app.go:3"),
		"the last line of the file is inside it")
	require.NoError(t, CheckTarget(head, "app.go:1"),
		"the first line of the file is inside it")

	var missing *InvalidTargetError
	require.ErrorAs(t, CheckTarget(head, "gone.go:1"), &missing)
	assert.Contains(t, missing.Error(), "does not hold as a file")

	var past *InvalidTargetError
	require.ErrorAs(t, CheckTarget(head, "app.go:4"), &past)
	assert.Contains(t, past.Error(), "holds 3 lines")

	// A shape the head is never asked about: the reader is not called at
	// all, so a malformed target is refused without a git invocation.
	unreadable := HeadFile(func(string) ([]string, bool, error) {
		t.Fatal("§5.5: a target that is not a path:line names nothing to resolve")
		return nil, false, nil
	})
	var malformed *InvalidTargetError
	require.ErrorAs(t, CheckTarget(unreadable, "app.go"), &malformed)
}

// A head cr cannot read is not a target the agent can correct, so the reader's
// own failure is returned unchanged rather than being turned into a rejection.
// The two carry different exit codes — §3.1.3's 3 for the external command and
// §6.2.3's 1 for the agent's data — and reporting a broken git as a mistyped
// flag would send the agent to retype a correct one.
func TestAnUnreadableHeadIsNotARejectedTarget(t *testing.T) {
	broken := errors.New("fatal: not a git repository")
	err := CheckTarget(func(string) ([]string, bool, error) {
		return nil, false, broken
	}, "app.go:3")
	require.ErrorIs(t, err, broken)
	var refused *InvalidTargetError
	assert.NotErrorAs(t, err, &refused)
}
