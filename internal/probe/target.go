package probe

import (
	"fmt"
	"strconv"

	"github.com/deligoez/cr/internal/git"
)

// TargetError reports a mutation patch §5.3.2's derivation cannot place a
// target in.
//
// It is a refusal rather than a fallback, and round 12's
// undefined-derivation-branch finding is why. §5.5 makes `target` a required
// row for a mutation probe, and §6.2.2 has a `probed` record's evidence point
// at it — so a target cr guessed is a line cr would go on to name to a
// colleague. There is no honest guess available: the alternative to refusing is
// to point at a line the patch does not touch.
type TargetError struct {
	// Path is the file the derivation was working in, and empty when the
	// patch named none.
	Path string
	// Problem says why no target follows from the patch.
	Problem string
}

func (e *TargetError) Error() string {
	if e.Path == "" {
		return "no target follows from this patch: " + e.Problem +
			"; §5.3.2 derives a mutation probe's target from the patch itself"
	}
	return fmt.Sprintf(
		"no target follows from this patch at %s: %s; §5.3.2 derives a mutation probe's target from the patch itself",
		e.Path, e.Problem)
}

// Target is §5.5's `target` for a mutation probe: the `path:line` §5.3.2
// derives from the patch.
//
// # Which hunk is "the first"
//
// §5.3.2 says "the first hunk" and a patch is a set of hunks in whatever order
// its author wrote them, so round 12's probe-target-underdetermined and
// probe-target-steering findings are two halves of one problem: a derivation
// that read file order would be single-valued only for the diffs git happens to
// emit, and an agent that wanted the target somewhere else could get it there by
// reordering its own patch. The order is therefore fixed here — ascending
// pre-image path, then ascending pre-image start — so every spelling of the
// same change derives the same target, and rewriting the patch's layout buys
// nothing.
//
// # Which line inside it
//
// The first removed or replaced line, which in a unified diff are one thing: an
// edited line is a removal and an addition standing together, and the removal is
// what carries the pre-image number. A hunk that removes nothing falls to
// §5.3.2's second clause, "its first pre-image line", which is the hunk's own
// start — the first context line it covers.
//
// A zero-context add-only hunk has no such line. `@@ -41,0 +42 @@` covers no
// pre-image line at all: 41 is the line the insertion follows, not a line the
// hunk holds, and naming it would point the evidence at code the patch does not
// touch. That is the branch §5.3.2 leaves undefined and the one this refuses.
//
// # Why a pre-image line is a head line
//
// §5.5 makes a probe's target `side: RIGHT`, and §5.3.2 derives it from the
// pre-image. The two agree because of what the patch applies to: §5.3.1's
// mutation is a diff against a sandbox file, and §5.1.6 admits the sandbox only
// when its HEAD is the round's head. The pre-image is therefore the head's own
// text, and its line numbers are head coordinates.
func Target(files []git.PatchedFile) (string, error) {
	var chosen *git.PatchHunk
	path := ""
	for i := range files {
		file := &files[i]
		if len(file.Hunks) == 0 {
			// A file header with no hunk changes no line — a mode
			// change, or a rename git wrote on its own — so there
			// is nothing in it to aim at.
			continue
		}
		if file.BasePath == "" {
			return "", &TargetError{
				Path:    file.Path,
				Problem: "the patch creates the file, so it has no pre-image line to name",
			}
		}
		for j := range file.Hunks {
			hunk := &file.Hunks[j]
			if chosen == nil || file.BasePath < path ||
				(file.BasePath == path && hunk.BaseStart < chosen.BaseStart) {
				chosen, path = hunk, file.BasePath
			}
		}
	}
	if chosen == nil {
		return "", &TargetError{Problem: "it holds no hunk"}
	}

	// The pre-image line each body line sits on: context and removed lines
	// consume one, added lines consume none.
	line := chosen.BaseStart
	for _, body := range chosen.Body {
		switch body[0] {
		case '-':
			return path + ":" + strconv.Itoa(line), nil
		case ' ':
			line++
		}
	}
	if chosen.BaseLines == 0 {
		return "", &TargetError{
			Path: path,
			Problem: fmt.Sprintf(
				"its first hunk at %d only adds lines and covers no pre-image line, "+
					"so §5.3.2's target would name a line the patch does not touch",
				chosen.BaseStart),
		}
	}
	return path + ":" + strconv.Itoa(chosen.BaseStart), nil
}
