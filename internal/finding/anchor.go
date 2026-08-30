package finding

import (
	"fmt"
	"strings"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/text"
)

// contextWindow is §9.2's bound on an anchor's recorded context: up to three
// lines on each side of the anchored range.
//
// §9.2.3 says what it is for and, in doing so, why the bound is enforced here
// rather than left to whoever reads it. v0.1 never migrates an anchor, so
// nothing in this version reads the window back at all; it is recorded only so
// that a v0.2 migration has it for the rounds v0.1 produced. A window written
// wider than the bound is therefore a fault no v0.1 command could ever notice.
const contextWindow = 3

// AnchorContentHash is §9.2's content hash: the normalised hash per §1.4 of the
// lines from `start_line` to `line` inclusive, taken as one text.
//
// The lines are joined with a single LF and handed to text.NormalisedHash whole,
// and that is the whole of the rule. There is deliberately no branch on how many
// of them there are. §9.2 writes the requirement as "a single-line anchor and a
// multi-line anchor hash by the same rule", which is a warning about a specific
// bug: a length test that hashes one line directly and several joined is
// equivalent on the day it is written and stops being equivalent the moment
// either side of it is touched, while both halves still look right. One line
// joins to itself, so the general case already is the special one, and
// TestTheContentHashBranchesOnNothing keeps it that way.
//
// The pre-image is named once, here, because three sections hash the same lines
// and their values have to be comparable: §9.2 for the anchor, §7.4.1 for the
// waiver key, and §6.1's citation content hash, which is this function over a
// slice of one line.
//
// The caller supplies the anchored lines themselves, read from the tree §6.1.2
// resolves the anchor's side against. Nothing here answers an empty slice,
// because ValidateAnchor refuses an anchor that spans no line, so a range always
// has a first line to read.
//
// The error is §1.4 step 1's, returned rather than swallowed for the reason
// text.NormalisedHash gives: undecodable input fails with exit code 1, and a
// fallback would turn that refusal into a value indistinguishable from a real
// one.
func AnchorContentHash(lines []string) (string, error) {
	return text.NormalisedHash(strings.Join(lines, "\n"))
}

// ValidateAnchor holds one record's anchor to §9.2's field set.
//
// It reports through RejectedRecordError, the shape §6.1.3 gives every record
// rejection — exit code 1, naming the line and the field — so an anchor fault
// reaches the user as the record fault it is, and needs no exit mapping of its
// own. The field is always `anchor`: `path`, `side`, `start_line` and `line` are
// keys of one object, and naming the object is what lets the user find it on the
// line the error points at.
//
// The ordering rule is round 12's finding unordered-range-fields. §9.2 requires
// an anchor to carry both line numbers and says nothing about their order, so
// `start_line: 40, line: 12` satisfies every word of the section while naming no
// range at all — and the fields are read as a range downstream, by §6.2.2's
// probe-target containment and §8.3.3's payload ordering, neither of which
// re-checks it.
//
// Two shapes of that fault are refused, and they are not the same shape:
//
//   - Inverted. §9.2's range is inclusive, so its length is line-start_line+1
//     and start_line == line is a legal one-line anchor rather than an empty one.
//     That leaves ordering as the only thing to check: any start_line greater
//     than line runs backwards. There is no separate empty case to find here,
//     because the length reaches zero only at line == start_line-1, which is
//     already inverted.
//   - Empty. An empty range is instead the one that names no line of any file.
//     §9.2 requires an anchor to carry start_line, so GitHub's own API — where
//     start_line is optional and absent means a single-line comment — does not
//     carry over: an anchor that omits it decodes to zero, and zero is not a
//     line. Both trees number their lines from 1, so a start_line below 1 spans
//     nothing whatever the other number says.
//
// The anchor is taken by pointer because a record's anchor is large enough that
// copying it is waste, not because anything here writes to it: every branch
// below reads and reports.
func ValidateAnchor(file string, line int, anchor *Anchor) error {
	reject := func(problem string) error {
		return &RejectedRecordError{File: file, Line: line, Field: "anchor", Problem: problem}
	}
	if anchor.Path == "" {
		return reject("names no path, so this is an item with no code location and never becomes a record (§6.1.2, §4.1.3)")
	}
	// §9.2's vocabulary is closed on the way in, where the value arrives
	// from outside: a side is written on an agent's NDJSON line, and
	// git.Side is a defined string that any spelling reaches. The set
	// itself is git.ParseSide's, so this reading of it and internal/gh's
	// cannot come apart.
	if _, known := git.ParseSide(string(anchor.Side)); !known {
		return reject(fmt.Sprintf(
			"names side %q; §9.2's values are %s, which anchors a line in the head, and %s, which anchors a removed line",
			anchor.Side, git.Right, git.Left,
		))
	}
	if anchor.StartLine < 1 {
		return reject(fmt.Sprintf(
			"starts at line %d, which no file has; §9.2 requires a start_line and both trees number their lines from 1",
			anchor.StartLine,
		))
	}
	if anchor.Line < anchor.StartLine {
		return reject(fmt.Sprintf(
			"runs from line %d back to line %d; §9.2's range is inclusive, so it runs forwards and a one-line anchor writes the same number twice",
			anchor.StartLine, anchor.Line,
		))
	}
	if len(anchor.ContextBefore) > contextWindow || len(anchor.ContextAfter) > contextWindow {
		return reject(fmt.Sprintf(
			"carries %d lines of context before the range and %d after; §9.2 records up to %d on each side",
			len(anchor.ContextBefore), len(anchor.ContextAfter), contextWindow,
		))
	}
	return nil
}

// The two trees §6.1.2 resolves an anchor against, as the user is told them.
const (
	headTree      = "the head under review"
	mergeBaseTree = "the merge base"
)

// Trees are those two revisions, each as a reader of one path.
//
// §9.2.1 gives the two sides different subject matter — RIGHT anchors a line in
// the head, LEFT a removed line — and §6.1.2 turns that difference into two
// lookups: a RIGHT anchor resolves against the head, a LEFT one against the
// merge base. A removed line is not in the head to be found, and a line the
// change added is not in the merge base, so one tree cannot answer for both
// without calling half the anchors cr will ever write unresolvable.
//
// They are two readers rather than one reader taking a revision, so that which
// commit is the head and which the merge base stays where §3.4.1 decides it,
// with internal/git, and is never something a record's own fields could move.
type Trees struct {
	// Head reads a path as the head under review holds it. A RIGHT anchor
	// resolves here.
	Head HeadFile
	// MergeBase reads it as the merge base holds it. A LEFT anchor
	// resolves here.
	MergeBase HeadFile
}

// tree returns the reader §6.1.2 binds to one side, together with the name the
// user is told when the lookup fails, and whether the side names a tree at all.
func (t Trees) tree(side git.Side) (read HeadFile, named string, known bool) {
	if side == git.Right {
		return t.Head, headTree, true
	}
	if side == git.Left {
		return t.MergeBase, mergeBaseTree, true
	}
	return nil, "", false
}

// ResolveAnchor resolves one record's anchor against the tree its side names,
// per §6.1.2: the head for RIGHT, the merge base for LEFT.
//
// It reports through RejectedRecordError, the shape §6.1.3 gives every record
// rejection, naming the field `anchor` for the reason ValidateAnchor gives: the
// anchor is one object, and naming it is what lets the user find it on the line
// the error points at. §11.2 codes it 1 — the file was read and parsed, and
// what is wrong is the record's own content — while a git that refuses is
// returned unchanged and stays exit code 3, exactly as ResolveCitations leaves
// it.
//
// Only the last line of the range is measured against the file. That the range
// runs forwards from line 1 or higher is §9.2's shape rule and ValidateAnchor's
// to keep, so the whole range lies in the file as soon as its last line does,
// and re-deciding the shape here would be a second reading of the section that
// could come to a different answer.
//
// §9.2.1 also has a LEFT anchor used only for records about deletions, and that
// half is not enforced here. Whether a record is about a deletion is a
// judgement about what it says, which §2.1 leaves to the agent; what cr can
// establish is the location, and this is it. A line the change added is not in
// the merge base, so a LEFT anchor reaching for one either falls outside the
// file or names other code entirely — and §6.2.2 keeps such a record out of the
// probed grade in any case, because a probe target is always RIGHT.
func ResolveAnchor(trees Trees, file string, line int, anchor *Anchor) error {
	reject := func(problem string) error {
		return &RejectedRecordError{File: file, Line: line, Field: "anchor", Problem: problem}
	}
	read, named, known := trees.tree(anchor.Side)
	if !known {
		return reject(fmt.Sprintf(
			"names side %q, which is neither %s nor %s and so names no tree to resolve against",
			anchor.Side, git.Right, git.Left,
		))
	}
	lines, exists, err := read(anchor.Path)
	if err != nil {
		return err
	}
	if !exists {
		return reject(fmt.Sprintf("names %q, which %s does not hold as a file", anchor.Path, named))
	}
	if anchor.Line > len(lines) {
		return reject(fmt.Sprintf(
			"runs to line %d of %q, which holds %d lines at %s",
			anchor.Line, anchor.Path, len(lines), named,
		))
	}
	return nil
}
