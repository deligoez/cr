package git

import (
	"fmt"
	"strings"
)

// PatchedFile is one file of a unified diff, with every hunk kept whole.
//
// It is what ParseHunks deliberately does not produce. That parser answers
// §3.4.1's question — which lines a diff git wrote counts as changed — and
// throws the rest of each hunk away, because a unit is built out of changed
// lines and nothing else. Applying a diff is the other question: the context
// lines are what say the patch matches the file in front of it, and the
// removed lines are what say which bytes go. §5.3.2 has cr apply an
// agent-supplied patch to a sandbox file, so the whole of each hunk has to
// survive parsing.
//
// The two share the header primitives below them rather than the scan, since a
// scan that produced both would have to be read as answering neither.
type PatchedFile struct {
	// Path is the head-side path, without the b/ prefix, falling back to
	// the pre-image path for a file the patch deletes.
	Path string
	// BasePath is the pre-image path without the a/ prefix, and empty
	// when the patch creates the file.
	BasePath string
	// HeadPath is the post-image path, and empty when the patch deletes
	// the file.
	HeadPath string
	// Hunks are the file's hunks, in the order the patch writes them.
	Hunks []PatchHunk
}

// PatchHunk is one @@ block with its body intact.
type PatchHunk struct {
	// BaseStart is the hunk's first line in the pre-image, and the line
	// an insertion follows when BaseLines is zero.
	BaseStart int
	// BaseLines is how many pre-image lines the hunk covers.
	BaseLines int
	// HeadStart is the hunk's first line in the post-image.
	HeadStart int
	// HeadLines is how many post-image lines the hunk covers.
	HeadLines int
	// Body holds the hunk's lines exactly as the patch wrote them,
	// leading marker included, so applying can verify what it replaces.
	Body []string
}

// MalformedPatchError reports a patch that is not a unified diff cr can apply:
// a line no hunk or file header can be, or a patch holding no hunk at all.
//
// ParsePatch reads a string and never learns where it came from, so it leaves
// File empty; the caller that read the patch out of a file fills it in, as
// state.RepeatedKeyError's caller fills in its file. The cli layer maps it onto
// exit code 1: the file was found and read, and what is wrong is the data in it.
type MalformedPatchError struct {
	// File is the file the patch was read from, and empty until a caller
	// that knows it says.
	File string
	// Line is the one-based line of the patch refused, and zero for a
	// refusal about the patch as a whole.
	Line int
	// Problem says what is wrong there.
	Problem string
	// Step is §12.4's next actionable step when the refusal has one of its
	// own, and empty for the refusals the general step answers.
	Step string
}

// malformedPatchStep is the step for a patch that is not a unified diff at all:
// the shape to write, and the flag that makes `git diff` write it.
const malformedPatchStep = "correct the `--patch` file at the line the message names so it is a unified diff " +
	"with --- and +++ headers and @@ hunks; if it came from `git diff`, re-run it with --no-ext-diff"

// Hint is §12.4's next actionable step for the refusal: its own when it has one,
// and otherwise the shape of a unified diff.
func (e *MalformedPatchError) Hint() string {
	if e.Step != "" {
		return e.Step
	}
	return malformedPatchStep
}

func (e *MalformedPatchError) Error() string {
	name := e.File
	if name == "" {
		name = "patch"
	}
	if e.Line == 0 {
		return name + ": " + e.Problem
	}
	return fmt.Sprintf("%s line %d: %s", name, e.Line, e.Problem)
}

// ParsePatch reads a unified diff as something to apply.
//
// It is strict about the shape and forgiving about nothing. A patch cr is
// about to run against a checkout is an experiment whose result becomes an
// assertion to a colleague under §6.2, so a diff that is nearly a diff is
// refused rather than guessed at: §5.3.4's first rung already gives a patch
// that does not apply cleanly its answer, and a parser that repaired one would
// take that rung's decision away from it.
//
// It is cleared by refactoring, never by raising the limit and never by loosening what
// the parser refuses.
//
//nolint:gocognit,funlen // measured 2026-08-31 at cognitive 34 over 51 statements
func ParsePatch(patch string) ([]PatchedFile, error) {
	files := make([]PatchedFile, 0)
	var (
		basePath, headPath string
		open               bool
		current            PatchHunk
		baseLeft, headLeft int
		// stray is the first line outside every hunk that is no file
		// header, which strayed reports once the whole patch is read.
		stray *MalformedPatchError
	)

	// finish ends the open hunk and files it under the file it belongs to.
	finish := func() {
		last := len(files) - 1
		files[last].Hunks = append(files[last].Hunks, current)
		open = false
	}

	for n, line := range strings.Split(strings.TrimSuffix(patch, "\n"), "\n") {
		// A hunk whose counts are spent stays open for one more line,
		// because `\ No newline at end of file` follows the line it
		// annotates and is often that last line's neighbour. Closing on
		// the count alone would drop the marker into the file-header
		// scan, and Apply would then put back a final newline the patch
		// says the file does not end with.
		if open && baseLeft <= 0 && headLeft <= 0 && !endOfFileMarker(line) {
			finish()
		}
		if open {
			if line == "" {
				return nil, &MalformedPatchError{Line: n + 1,
					Problem: "a hunk holds no empty line, because a blank context line keeps its leading space"}
			}
			switch line[0] {
			case ' ':
				baseLeft, headLeft = baseLeft-1, headLeft-1
			case '+':
				headLeft--
			case '-':
				baseLeft--
			case '\\':
				// The marker annotates the line above it and
				// belongs to neither version, so it consumes
				// nothing.
			default:
				return nil, &MalformedPatchError{Line: n + 1, Problem: fmt.Sprintf("%q is not a hunk line", line)}
			}
			current.Body = append(current.Body, line)
			continue
		}

		switch {
		case strings.HasPrefix(line, "--- "):
			basePath = headerPath(line[4:], "a/")
		case strings.HasPrefix(line, "+++ "):
			headPath = headerPath(line[4:], "b/")
			named := headPath
			if named == "" {
				named = basePath
			}
			if named == "" {
				return nil, &MalformedPatchError{Line: n + 1,
					Problem: "the file header names no path on either side"}
			}
			files = append(files, PatchedFile{
				Path: named, BasePath: basePath, HeadPath: headPath,
				Hunks: make([]PatchHunk, 0),
			})
		case strings.HasPrefix(line, "@@ "):
			if len(files) == 0 {
				return nil, &MalformedPatchError{Line: n + 1,
					Problem: "a hunk arrives before any --- and +++ file header"}
			}
			fields := hunkHeader.FindStringSubmatch(line)
			if fields == nil {
				return nil, &MalformedPatchError{Line: n + 1, Problem: fmt.Sprintf("%q is not a hunk header", line)}
			}
			current = PatchHunk{
				BaseStart: headerNumber(fields[1]),
				BaseLines: headerCount(fields[2]),
				HeadStart: headerNumber(fields[3]),
				HeadLines: headerCount(fields[4]),
				Body:      make([]string, 0),
			}
			if current.BaseLines == 0 && current.HeadLines == 0 {
				return nil, &MalformedPatchError{Line: n + 1,
					Problem: fmt.Sprintf("%q covers no line on either side", line)}
			}
			baseLeft, headLeft = current.BaseLines, current.HeadLines
			open = true
		case strings.HasPrefix(line, "diff --git "), strings.HasPrefix(line, "index "):
			// git's own headers, walked past: the --- and +++ lines
			// name the file again, and neither line asks cr to do
			// anything those two and the hunks do not.
		default:
			stray = firstStray(stray, n+1, line)
		}
	}
	if open {
		if baseLeft > 0 || headLeft > 0 {
			return nil, &MalformedPatchError{Problem: "the patch ends inside a hunk"}
		}
		// The patch ended on the hunk's last line, which is the
		// ordinary shape: a diff of one hunk stops there.
		finish()
	}
	return strayed(files, stray)
}

// firstStray keeps the first line outside every hunk that is no file header,
// with why cr would not execute it.
func firstStray(kept *MalformedPatchError, line int, text string) *MalformedPatchError {
	if kept != nil {
		return kept
	}
	problem, step := unexecuted(text)
	return &MalformedPatchError{Line: line, Problem: problem, Step: step}
}

// strayed refuses a patch holding a stray line, unless it named no file at all.
//
// A patch with no file header is not a diff with one line too many; it is the
// report a configured diff.external writes in a diff's place, and the caller's
// no-hunk refusal names the flag that fixes it.
func strayed(files []PatchedFile, stray *MalformedPatchError) ([]PatchedFile, error) {
	if stray != nil && len(files) > 0 {
		return nil, stray
	}
	return files, nil
}

// endOfFileMarker reports the `\ No newline at end of file` line, which is the
// one hunk line that follows a hunk's last counted line rather than being one.
func endOfFileMarker(line string) bool {
	return line != "" && line[0] == '\\'
}

// extendedHeaders are the header lines git writes for a rename, a copy, a mode
// change, a created or deleted file, and the similarity a rename or copy is
// measured by.
var extendedHeaders = []string{
	"old mode ", "new mode ", "new file mode ", "deleted file mode ",
	"similarity index ", "dissimilarity index ",
	"rename from ", "rename to ", "copy from ", "copy to ",
}

// unexecuted says why ParsePatch refuses a line outside every hunk that is no
// file header, and the step that removes what it refused.
//
// A probe applies hunks to files the sandbox already holds and does nothing
// else, so a rename, a copy or a mode change is a step cr never takes. Walking
// past such a line would still store it in §5.5's `input`, which `cr draft`
// shows a colleague as the experiment that ran; refusing it keeps the stored
// patch and the executed one the same patch, as §5.3.2's evidence chain needs.
//
// The patch around such a line is a diff, so the step names what was refused
// rather than the shape of a unified diff and the flag for an external differ.
func unexecuted(line string) (problem, step string) {
	for _, header := range extendedHeaders {
		if strings.HasPrefix(line, header) {
			return fmt.Sprintf("%q is a rename, copy or mode header, which cr does not execute: "+
					"a probe only applies hunks to files the sandbox holds, so the probe record's input "+
					"would show a step that never ran", line),
				"remove the rename, copy or mode header on the line the message names from the `--patch` " +
					"file, and write the mutation as @@ hunks against files the sandbox already holds"
		}
	}
	return fmt.Sprintf("%q is neither a file header nor a hunk line, so cr would not execute it", line),
		"remove the line the message names from the `--patch` file: outside its @@ hunks a probe's patch " +
			"holds only diff --git, index, --- and +++ file header lines"
}

// ApplyError reports a hunk that does not match the file it addresses, which is
// §5.3.4's first rung: the patch did not apply cleanly.
//
// It names the file, the line, and both texts, because the reader has to decide
// whether the patch is stale or the sandbox is — and those are opposite fixes.
type ApplyError struct {
	// Path is the file the hunk addresses.
	Path string
	// Line is the pre-image line number that did not match, and zero
	// when the hunk reaches past the end of the file.
	Line int
	// Want is the line the patch expected to find there.
	Want string
	// Found is the line that is actually there, and empty when the file
	// has no such line.
	Found string
	// Problem says which of the two shapes this is.
	Problem string
}

func (e *ApplyError) Error() string {
	if e.Line == 0 {
		return fmt.Sprintf("%s: %s", e.Path, e.Problem)
	}
	return fmt.Sprintf("%s line %d: %s: the patch expected %q and the file holds %q",
		e.Path, e.Line, e.Problem, e.Want, e.Found)
}

// Apply returns what content becomes once this file's hunks are applied to it.
//
// Every hunk is applied at the line its header names, and every context and
// removed line is required to be there exactly. There is no offset search and
// no fuzz, which git's own `apply` offers and cr does not want: a hunk that
// matched three lines away matched something the agent did not write the patch
// against, and §5.3.2 has the evidence chain run "on the patch cr executed".
// A mutation applied somewhere other than where it was aimed is the expensive
// shape of wrong — the experiment succeeds, the record names a target, and the
// two are about different code.
func (f *PatchedFile) Apply(content string) (string, error) {
	lines := splitLines(content)
	out := make([]string, 0, len(lines))
	// cursor is the next pre-image line to copy, counted from zero.
	cursor := 0
	// trailing says whether the applied content ends in a newline. It is
	// the pre-image's answer unless a hunk reached the end of the file,
	// which is the only place the answer can change.
	trailing := strings.HasSuffix(content, "\n")

	for i := range f.Hunks {
		hunk := &f.Hunks[i]
		// A hunk covering no pre-image line is an insertion after the
		// line its header names, so the copy runs to that line rather
		// than to the one before it.
		upto := hunk.BaseStart - 1
		if hunk.BaseLines == 0 {
			upto = hunk.BaseStart
		}
		if upto < cursor || upto > len(lines) {
			return "", &ApplyError{
				Path:    f.Path,
				Problem: fmt.Sprintf("the hunk at %d addresses a line the file does not hold", hunk.BaseStart),
			}
		}
		out = append(out, lines[cursor:upto]...)
		cursor = upto

		for _, body := range hunk.Body {
			marker, text := body[0], body[1:]
			switch marker {
			case ' ', '-':
				if cursor >= len(lines) {
					return "", &ApplyError{
						Path:    f.Path,
						Problem: fmt.Sprintf("the hunk at %d reaches past the end of the file", hunk.BaseStart),
					}
				}
				if lines[cursor] != text {
					return "", &ApplyError{
						Path: f.Path, Line: cursor + 1, Want: text, Found: lines[cursor],
						Problem: "the hunk does not match the file",
					}
				}
				if marker == ' ' {
					out = append(out, text)
				}
				cursor++
			case '+':
				out = append(out, text)
			}
		}
		// The final newline is the last hunk's to change, and only a
		// hunk that consumed the file to its end can have changed it.
		if cursor == len(lines) {
			trailing = !endsWithoutNewline(hunk.Body)
		}
	}
	out = append(out, lines[cursor:]...)
	if len(out) == 0 {
		return "", nil
	}
	applied := strings.Join(out, "\n")
	if trailing {
		applied += "\n"
	}
	return applied, nil
}

// endsWithoutNewline reports whether a hunk leaves the post-image without a
// final newline.
//
// git writes `\ No newline at end of file` directly beneath the line it
// annotates. A marker under a removed line describes the pre-image alone and
// says nothing about what the patch produces; one under an added or context
// line describes the post-image, and is the answer when it is the last thing
// the hunk has to say.
func endsWithoutNewline(body []string) bool {
	last := len(body) - 1
	if last < 1 || body[last][0] != '\\' {
		return false
	}
	return body[last-1][0] != '-'
}
