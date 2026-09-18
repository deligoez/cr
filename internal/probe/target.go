package probe

import (
	"fmt"
	"strconv"
	"strings"

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

// HeadFile reads a path as the head under review holds it: the file's lines,
// and whether the head holds it as a file at all.
//
// It has the shape finding.HeadFile has and is deliberately not that type:
// internal/finding imports this package, so the dependency cannot run the other
// way. What the two share is the reader that satisfies both, git.FileAtRevision
// with the repository and the head already bound — so the tree §6.2.3 resolves
// a citation against and the tree a gap probe's target is resolved against are
// one tree by construction.
type HeadFile func(path string) (lines []string, exists bool, err error)

// InvalidTargetError reports a `--target` that is not a location the head under
// review holds.
//
// It is separate from TargetError because the two faults are corrected
// differently. A mutation probe's target is derived, so TargetError says the
// patch admits none and the agent has to send a different experiment; a gap
// probe's target is typed, so this says the flag names nowhere and the agent
// has to retype it. Both are the agent's data inside input cr read without
// trouble, so internal/cli/exit.go codes both 1.
type InvalidTargetError struct {
	// Target is the value the flag carried, quoted back so the agent can
	// see what cr read rather than what it meant.
	Target string
	// Problem says why the head does not hold it, in the words §6.2.3's
	// citation rejections use.
	Problem string
}

func (e *InvalidTargetError) Error() string {
	return fmt.Sprintf(
		"--target %q %s; §5.5 takes a gap probe's target from this flag and has it "+
			"validated as §6.2.3 validates a citation",
		e.Target, e.Problem)
}

// ParseTarget reads §5.5's `path:line` into its two halves.
//
// The separator is the last colon rather than the first, because a path may
// hold one and a line number may not. The number is required to be spelled the
// way cr spells one — no sign, no leading zero, no surrounding space — for the
// reason parseID accepts only the canonical id: a value cr would not have
// written is not a value cr can claim to have understood, and `app.go:+3` and
// `app.go:03` both invite the reader to believe cr checked something it did not.
//
// Line 1 is the floor, as it is for a citation: §6.2.3 rejects an entry whose
// line is out of range, and a file's lines are numbered from one.
func ParseTarget(target string) (path string, line int, err error) {
	reject := func(problem string) (string, int, error) {
		return "", 0, &InvalidTargetError{Target: target, Problem: problem}
	}
	at := strings.LastIndex(target, ":")
	if at < 0 {
		return reject("is not a path:line")
	}
	path, number := target[:at], target[at+1:]
	if path == "" {
		return reject("names no path")
	}
	line, err = strconv.Atoi(number)
	if err != nil || number != strconv.Itoa(line) || line < 1 {
		return reject(fmt.Sprintf("names line %q, which is not a line number", number))
	}
	return path, line, nil
}

// CheckTarget validates a gap probe's supplied `--target` against the head
// under review, as §6.2.3 validates a citation.
//
// The two refusals are §6.2.3's two: a path the head does not hold as a file,
// and a line the file does not have. Nothing else is asked, and §6.2.3 says why
// in as many words — validation establishes that a location **exists**, never
// that it supports anything. What a gap probe's target is for is §6.2.2, which
// requires a `probed` record's probe target to fall inside that record's anchor
// range; a target naming a line the head does not hold could satisfy no anchor
// and would be a `path:line` cr had put in a record and never opened.
//
// No hash is taken. §6.2.3 has cr store a citation's `content_hash` for a v0.2
// drift check, and §5.5's table gives a probe record no such row: storing one
// here would be a field the section does not describe, which checkRecordFields
// refuses.
func CheckTarget(read HeadFile, target string) error {
	path, line, err := ParseTarget(target)
	if err != nil {
		return err
	}
	lines, exists, err := read(path)
	if err != nil {
		return err
	}
	if !exists {
		return &InvalidTargetError{Target: target, Problem: fmt.Sprintf(
			"names %q, which the head under review does not hold as a file", path)}
	}
	if line > len(lines) {
		return &InvalidTargetError{Target: target, Problem: fmt.Sprintf(
			"names line %d of %q, which holds %d lines at the head under review",
			line, path, len(lines))}
	}
	return nil
}
