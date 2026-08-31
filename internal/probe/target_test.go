package probe

import (
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
			name: "the lowest pre-image start wins, whatever order the hunks are written in",
			patch: "--- a/app.go\n+++ b/app.go\n@@ -40 +40 @@\n-late\n+LATE\n" +
				"@@ -8 +8 @@\n-early\n+EARLY\n",
			want: "app.go:8",
		},
		{
			name: "the lowest path wins, whatever order the files are written in",
			patch: "--- a/z.go\n+++ b/z.go\n@@ -2 +2 @@\n-z\n+Z\n" +
				"--- a/a.go\n+++ b/a.go\n@@ -90 +90 @@\n-a\n+A\n",
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
	}{
		{
			name:  "a zero-context add-only first hunk",
			patch: "--- a/app.go\n+++ b/app.go\n@@ -41,0 +42 @@\n+added\n",
			want:  "covers no pre-image line",
		},
		{
			name:  "a patch that creates the file",
			patch: "--- /dev/null\n+++ b/app.go\n@@ -0,0 +1 @@\n+added\n",
			want:  "no pre-image line to name",
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
		})
	}
}
