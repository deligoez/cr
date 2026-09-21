// Package migrate moves a record's anchor to the current head, per §9.4.
//
// It is a package of its own rather than a method on finding.Anchor because the
// decision needs files, and internal/finding holds no reader: an anchor knows
// what it recorded, and only a caller holding the head's trees can say where
// that now is.
//
// The whole design is one rule, §9.4.6: a migration that cannot be made unique
// is not a migration. Everything here is arranged so that declining is the
// default and placing is what has to be earned — every path that cannot settle
// the question returns the same decline, and none of them guesses.
package migrate

import (
	"slices"
	"strconv"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/text"
)

// The two keys of §9.4, named for the Outcome that reports them.
const (
	// KeyContent is §9.4.3's: the content hash alone.
	KeyContent = "content"
	// KeyContext is §9.4.4's: the content hash plus the recorded window.
	KeyContext = "context"
)

// Outcome says what §9.4 did with one record's anchor, and is what §9.4.7
// reports.
type Outcome struct {
	// Record is the id of the record the anchor belongs to.
	Record string `json:"record"`
	// From is the anchor as it was stored, `path:line`.
	From string `json:"from"`
	// To is where it moved, `path:line`, and is empty on a decline.
	To string `json:"to,omitempty"`
	// Placed reports whether the anchor moved.
	//
	// A declined record keeps the anchor it had, which §9.4.4 requires:
	// the recorded anchor is the only place the record was ever known to
	// be right about, and overwriting it with a worse guess would lose
	// that without gaining anything.
	Placed bool `json:"placed"`
	// Key names which of §9.4's two keys decided, and on a decline the key
	// that was tried last.
	Key string `json:"key"`
	// Candidates is how many the deciding key left. It is what separates a
	// decline meaning "nowhere" from one meaning "too many places", which
	// are different problems for whoever reads the report: the first is a
	// concern whose code is gone, the second a concern whose code is now
	// in several places.
	Candidates int `json:"candidates"`
}

// File is one file of the head as migration reads it: its path and its lines in
// order, each without a line terminator.
type File struct {
	Path  string
	Lines []string
}

// candidate is one run of lines whose content hash matched, before §9.4.4 has
// had a chance to narrow the set. The index is into the ordered files.
type candidate struct {
	file       int
	start, end int
}

// Anchor migrates one anchor onto files, which are the current head's.
//
// §9.4.3 fixes the search order — the file the anchor names, then every other
// file the diff touched — and the order changes no decision, since a unique
// candidate is unique wherever it sits. It exists so a reader of the report
// sees the anchor's own file named first when both could have carried it.
//
// An anchor whose recorded span is empty or inverted migrates nowhere. §9.2
// admits no such shape, so meeting one here means a stored record is
// malformed, and the honest answer is the decline every unplaceable record gets
// rather than a panic on a negative length.
func Anchor(record string, anchor *finding.Anchor, files []File) Outcome {
	ordered := order(anchor.Path, files)
	outcome := Outcome{
		Record: record,
		From:   anchor.Path + ":" + strconv.Itoa(anchor.Line),
		Key:    KeyContent,
	}

	span := anchor.Line - anchor.StartLine + 1
	if span < 1 {
		return outcome
	}

	found := gather(anchor.ContentHash, span, ordered)
	outcome.Candidates = len(found)
	switch {
	case len(found) == 1:
		return placed(&outcome, ordered, found[0])
	case len(found) == 0:
		return outcome
	}

	// §9.4.4: more than one place carries the same code, so the recorded
	// window is what has to tell them apart.
	outcome.Key = KeyContext
	survivors := narrow(anchor, found, ordered)
	outcome.Candidates = len(survivors)
	if len(survivors) == 1 {
		return placed(&outcome, ordered, survivors[0])
	}
	return outcome
}

// gather collects every run of span lines whose content hash is the anchor's,
// which is §9.4.3's candidate set.
//
// A hash that will not compute counts as no match rather than as an error.
// finding.AnchorContentHash refuses undecodable input per §1.4, and a file in
// the head that cannot be normalised is one this anchor cannot honestly be
// placed in — so the decline that follows is the answer §9.4.6 wants for every
// other case cr cannot settle, reached the same way.
func gather(hash string, span int, ordered []File) []candidate {
	found := make([]candidate, 0)
	for index, file := range ordered {
		for start := 0; start+span <= len(file.Lines); start++ {
			at, err := finding.AnchorContentHash(file.Lines[start : start+span])
			if err != nil || at != hash {
				continue
			}
			found = append(found, candidate{file: index, start: start, end: start + span - 1})
		}
	}
	return found
}

// narrow keeps the candidates whose neighbouring lines are the ones the record
// recorded, which is §9.4.4's wider key.
func narrow(anchor *finding.Anchor, found []candidate, ordered []File) []candidate {
	survivors := make([]candidate, 0, len(found))
	for _, one := range found {
		lines := ordered[one.file].Lines
		if matches(anchor.ContextBefore, above(lines, one.start, len(anchor.ContextBefore))) &&
			matches(anchor.ContextAfter, below(lines, one.end, len(anchor.ContextAfter))) {
			survivors = append(survivors, one)
		}
	}
	return survivors
}

// above returns up to want lines ending just before start. It returns a short
// slice at the file's edge, which matches refuses.
func above(lines []string, start, want int) []string {
	return lines[max(start-want, 0):start]
}

// below returns up to want lines starting just after end.
func below(lines []string, end, want int) []string {
	return lines[end+1 : min(end+1+want, len(lines))]
}

// matches compares a recorded window against a found one under §1.4's
// normalisation, line for line and at the same length.
//
// The length condition is the one worth stating, because the permissive
// alternative is tempting and wrong. §9.2.3 records *up to* three lines and
// keeps what existed, so a record holding fewer already says its anchor sat
// near the file's edge — and a candidate at a file boundary has fewer lines to
// offer. Counting the missing side as agreement would make every such
// candidate match every record: measured here before the rule was tightened, a
// candidate on line 1 with nothing above it matched a record that had recorded
// a line above, and §9.4.4 placed the anchor on the wrong one of two copies.
// An absent side is not a matching side.
//
// A normalisation that fails compares unequal, for gather's reason: a line cr
// cannot normalise is one it cannot honestly match.
func matches(recorded, found []string) bool {
	return slices.EqualFunc(recorded, found, func(a, b string) bool {
		left, leftErr := text.Normalise(a)
		right, rightErr := text.Normalise(b)
		return leftErr == nil && rightErr == nil && left == right
	})
}

// order returns files with the anchor's own path first, per §9.4.3.
func order(path string, files []File) []File {
	ordered := make([]File, 0, len(files))
	for _, file := range files {
		if file.Path == path {
			ordered = append(ordered, file)
		}
	}
	for _, file := range files {
		if file.Path != path {
			ordered = append(ordered, file)
		}
	}
	return ordered
}

// placed records where a unique candidate put the anchor. The line reported is
// the run's last, which is what §9.2 calls the anchor's `line`.
func placed(outcome *Outcome, ordered []File, one candidate) Outcome {
	outcome.Placed = true
	outcome.To = ordered[one.file].Path + ":" + strconv.Itoa(one.end+1)
	outcome.Candidates = 1
	return *outcome
}
