// Package reinvention attaches §4.3.1's candidate pre-existing symbols: for
// every function, method, or class the diff adds, the symbols the head already
// declared elsewhere.
//
// It decides nothing. §4.3.2's own sentence is explicit — semantic equivalence
// is the agent's judgement, not cr's — and §4.3.4 makes every reinvention item
// a question for the same reason: cr cannot know whether the existing symbol
// was looked at and rejected. So this locates candidates and stops, which is
// P5 and invariant 1 at one more call site.
//
// The other half of what it produces is the honesty one. §4.3.1 needs a symbol
// index built from the profile's `symbols.lang`, and when none can be built it
// requires the reinvention half of the convention axis to be marked unavailable
// per §4.5 rather than skipped silently — so unavailable.go's entry travels
// beside the attachments rather than in a report a caller has to remember to
// ask for.
package reinvention

import (
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/symbol"
)

// Attachment is what §4.3.1 attaches for one symbol the diff added.
type Attachment struct {
	// Added is the function, method, or class the diff declared.
	Added symbol.Decl `json:"added"`
	// Candidates are the pre-existing symbols that qualified under
	// §4.3.2, in that item's order and no longer than
	// `reinvention.max_candidates`. It is empty rather than nil when none
	// qualified, per §12.
	//
	// They carry no similarity and no verdict. §4.3.2 ends by saying
	// semantic equivalence is the agent's judgement and not cr's, and a
	// score travelling beside a symbol would read as a confidence cr does
	// not have — which §4.3.4 then posts to a colleague as a question.
	Candidates []symbol.Decl `json:"candidates"`
}

// Attachments is §4.3.1's whole answer for one round: what was attached, and
// the §4.5.4 entry for the half that did not run.
//
// The two travel together because §4.3.1 makes them one obligation — attach
// candidates, or mark the half unavailable — and a caller that could take the
// first without the second is a caller that can skip it silently, which is the
// sentence's own wording for the failure. testadequacy.Attachment carries
// §4.4's symbol half the same way, so §4.6.1 reads the two halves of the same
// problem with one shape.
//
// Both slices are empty rather than nil, per §12.
type Attachments struct {
	// Attached holds one entry per symbol the diff added.
	Attached []Attachment `json:"attached"`
	// Unavailable holds the §4.5.4 entry for the reinvention half when it
	// did not run, and nothing when it did.
	Unavailable []Unavailable `json:"unavailable"`
	// unindexed are the files MarkUnindexed named, whose units the
	// attachment does not speak for.
	unindexed map[string]bool
}

// MarkUnindexed adds Unindexed's entry for files to the attachments and
// remembers the files, so Covers answers false for their units.
//
// It is a step after Attach rather than an argument to it because the files
// come from reading the head, which Attach does not do; the entry and the
// memory are one call so a caller cannot report the files and still let their
// units read as covered.
func (a *Attachments) MarkUnindexed(p *profile.Profile, files []string) {
	if len(files) == 0 {
		return
	}
	a.Unavailable = append(a.Unavailable, Unindexed(p, files)...)
	a.unindexed = make(map[string]bool, len(files))
	for _, file := range files {
		a.unindexed[file] = true
	}
}

// Covers reports whether the attachments speak for a unit of the file at path:
// false when the half did not run at all, and false for a file MarkUnindexed
// named. Only where it is true is an empty attachment the diff declaring
// nothing.
func (a *Attachments) Covers(path string) bool {
	if a.unindexed != nil {
		return !a.unindexed[path]
	}
	return len(a.Unavailable) == 0
}

// Attach reads §4.3.1 over one round's profile, index, and diff.
//
// # What "added by the diff" means, and why the index answers it
//
// A declaration is added by the diff when it is written on a changed line, and
// the index already records the line every declaration is written on. So the
// added symbols are found by intersecting the index with the diff rather than
// by parsing the hunk text a second time — which matters beyond tidiness: a
// declaration recognised one way and not the other would be attached as an
// added symbol whose own entry stayed in the candidate pool, and cr would ask
// the author whether they had reinvented the function they were looking at.
//
// Only RIGHT-side changed lines are read. §3.4.1 numbers a hunk that adds no
// line on the LEFT, in merge-base coordinates, where the same number names a
// different line; the index is built over the head, so a LEFT number tested
// against it would subtract whatever happens to sit at that line now. A hunk
// that adds nothing declares nothing at the head, which is the same answer
// reached honestly.
//
// # Why the subtraction is over the whole index and not per symbol
//
// §4.3.1 subtracts "every symbol the diff itself declares on a changed line",
// not every symbol this one added symbol declares. Two helpers added in one
// pull request are each other's diff-declared symbols, so neither is offered as
// the other's pre-existing candidate — cr has no pre-existing anything to point
// at there, and asking whether a new function reinvents its equally new
// neighbour is a question about a decision the author made deliberately, in the
// same change, minutes ago.
//
// # What reaches the attachment
//
// The pool is what §4.3.1 subtracts; what is attached is §4.3.2's ranking of
// it, narrowed to the candidates whose parameter count matches and whose
// comparison-name similarity clears the threshold, ordered, and cut at
// `reinvention.max_candidates`. The two items are one attachment because a
// caller holding the unranked pool would be holding every symbol in the
// repository, and handing that to §4.6.1's prompt is not a narrower version of
// the right answer — it is a question about every function the author did not
// write.
//
// # A head with no index
//
// It attaches nothing and reports the §4.5.4 entry instead. An empty attachment
// list on its own reads to the author as "cr looked for reinvention and found
// none" — an assertion cr never made — and §4.3.1 forbids exactly that silence.
// The reason travels with the entry, so what the reader is told is a lens that
// did not run and what would make it run.
func Attach(p *profile.Profile, index *symbol.Index, hunks []git.Hunk, r Ranking) Attachments {
	if entry, marked := unavailability(p, index); marked {
		return Attachments{Attached: []Attachment{}, Unavailable: []Unavailable{entry}}
	}

	declared := declaredLines(hunks)
	added := make([]symbol.Decl, 0, len(index.Decls))
	pool := make([]symbol.Decl, 0, len(index.Decls))
	for _, decl := range index.Decls {
		if declared[location{path: decl.Path, line: decl.Line}] {
			added = append(added, decl)
			continue
		}
		pool = append(pool, decl)
	}

	attached := make([]Attachment, 0, len(added))
	for _, decl := range added {
		attached = append(attached, Attachment{Added: decl, Candidates: rank(decl, pool, r)})
	}
	return Attachments{Attached: attached, Unavailable: []Unavailable{}}
}

// location is one head-side place a declaration can sit.
type location struct {
	path string
	line int
}

// declaredLines is the set of head-side lines the diff changed, which is where
// a symbol the diff declares can be written.
func declaredLines(hunks []git.Hunk) map[location]bool {
	lines := make(map[location]bool, len(hunks))
	for at := range hunks {
		for _, changed := range hunks[at].Changed {
			if changed.Side != git.Right {
				continue
			}
			lines[location{path: hunks[at].Path, line: changed.Line}] = true
		}
	}
	return lines
}
