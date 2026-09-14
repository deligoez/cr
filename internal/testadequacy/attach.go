// Package testadequacy carries §4.4's evidence: the test files a pull request
// changed, the symbols they reference, and the classification the agent makes
// out of them.
//
// The division of §2.1.3 runs straight through this package. Attach locates the
// candidate evidence and supplies it; Coverage records the decision; nothing
// between the two computes one, and coverage.go makes that a property of the
// types rather than a rule someone has to keep.
package testadequacy

import (
	"fmt"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/symbol"
	"github.com/deligoez/cr/internal/unit"
)

// SymbolLens names the half of the test axis §4.4.1 builds out of a symbol
// index, spelled the way profile.ReinventionLens spells §4.3.1's.
//
// It is deliberately not an axis id. The file half of §4.4.1 runs without any
// index at all and the agent classifies from it, so reporting the whole test
// axis as not run would claim more went unreviewed than did.
const SymbolLens = axis.Test + "/symbols"

// References is the source of §4.4.1's "and the symbols they reference".
//
// unit.SymbolIndex cannot serve it. Indexed and Enclosing answer where a symbol
// is declared, and a test file referring to a helper declares nothing; §4.3.1
// builds an index of declarations over the head, which is a different question
// from what one file names. So this is its own interface, and HeadReferences
// answers it out of that index by reading each test file at the head; with no
// index there is nothing to answer from, which is why §4.4 needs the
// unavailability path below.
type References interface {
	// Referenced names the symbols the test file at path refers to, in the
	// order they first appear, and reports false when cr could not read
	// that file's references.
	//
	// The bool is what keeps a file cr could not read apart from a file that
	// references nothing. Collapsing the two would make an unread file
	// indistinguishable from an empty one, and §4.5.4 exists because that
	// silence reads to the author as a lens that looked.
	Referenced(path string) ([]string, bool)
}

// Unavailable is one entry of §4.5.4's report: a lens that did not run, and the
// reason it could not.
//
// §4.4 states no unavailability path of its own, the way §4.3.1 does for the
// reinvention half of the convention axis. §4.5.4 nevertheless requires every
// lens that did not run to appear in the coverage report with its reason, and
// the symbol half of §4.4.1 is such a lens whenever no index can answer for a
// test file. This is how it gets there, rather than by failing the run or by
// handing back an empty symbol list cr never established.
type Unavailable struct {
	// Lens is the lens that did not run.
	Lens string `json:"lens"`
	// Reason is why it could not.
	Reason string `json:"reason"`
}

// Disclosure satisfies finding.HonestyDisclosure, so the entry reaches the
// reader through the one channel §11.1 exempts from `--quiet` rather than
// through a message a flag can silence. The text is derived from the two fields,
// so what is printed and what a caller reads as data cannot drift apart.
func (u Unavailable) Disclosure() string {
	return "lens " + u.Lens + " unavailable, per §4.5.4: " + u.Reason
}

// Attachment is what §4.4.1 attaches: the test files the pull request changed,
// the symbols they reference, and the lenses that could not be filled.
//
// It carries no unit id, and that is the point. Attaching a subset of the test
// files to a unit would be cr deciding which tests bear on which code — the
// coverage judgement §2.1.3 reserves for the agent, arrived at one step early
// and wearing the shape of an attachment. cr supplies the same evidence to every
// unit and records what the agent makes of it.
type Attachment struct {
	// Paths are the test files this round's diff names, deduplicated and in
	// the order the diff names them.
	Paths []string `json:"paths"`
	// Symbols are the symbols those files reference, deduplicated and in
	// first-appearance order.
	Symbols []string `json:"symbols"`
	// Unavailable holds the §4.5.4 entries for the halves that did not run.
	// It is empty, and never nil, when both did.
	Unavailable []Unavailable `json:"unavailable"`
}

// Attach builds one round's §4.4.1 attachment out of the diff's hunks.
//
// refs is nil when no symbol index could be built for the round, which
// HeadReferences reports for a profile whose index did not arrive. A nil index
// is answered here rather than tolerated: it produces the §4.5.4 entry
// SymbolLens names, so a round with no index reports a half that did not run
// instead of a half that found nothing.
func Attach(p *profile.Profile, refs References, hunks []git.Hunk) Attachment {
	paths := testPaths(p, hunks)
	symbols, unavailable := referenced(p, refs, paths)
	return Attachment{Paths: paths, Symbols: symbols, Unavailable: unavailable}
}

// Unindexed is the §4.5.4 entry for the symbol half over the changed files
// symbol.Unindexed found outside the head index, given the paths of the round's
// units, and nothing when it found none or no unit is a test file.
//
// The symbols a test file references are read against the head index, so a
// symbol declared in a file the index does not hold is never among them, and
// an attachment reading "none" there is cr saying the tests reference nothing
// it looked for. A round whose units hold no test file has nothing for this
// half to read, so the files cost it nothing and no entry is owed.
func Unindexed(p *profile.Profile, paths, files []string) []Unavailable {
	out := make([]Unavailable, 0, 1)
	if len(files) == 0 || !slices.ContainsFunc(paths, p.IsTestFile) {
		return out
	}
	return append(out, Unavailable{Lens: SymbolLens, Reason: fmt.Sprintf(
		"profile %q builds §4.3.1's symbol index over its match.globs, which cover none of %s, "+
			"and those files declare symbols at the head, so a symbol the test files reference from them "+
			"is not attached; add a glob covering them to the profile's match.globs",
		p.ID, strings.Join(files, ", "),
	)})
}

// PerUnit hands the round's attachment to every unit, which is the whole of
// §4.4.1's "to every unit" and what §4.6.1 carries into each prompt.
//
// Every unit gets the same attachment because cr has no basis for any other
// distribution: narrowing one unit's test files to the ones that bear on it is
// the classification §2.1.3 reserves for the agent. So the count is the only
// thing the units decide here, and "every" is produced rather than remembered.
func PerUnit(clusters []unit.Cluster, a Attachment) []Attachment {
	per := make([]Attachment, 0, len(clusters))
	for range clusters {
		per = append(per, a)
	}
	return per
}

// testPaths returns the test files the round's hunks name, deduplicated and in
// the order the diff names them.
//
// A hunk of a file the change deletes carries its merge-base path, so a test
// file the pull request removed is attached alongside the ones it changed and
// added. §4.4.1 says "changed or added", and a deletion is neither a new file
// nor an edited one — but it is the most coverage-relevant thing a diff can do
// to a suite, and withholding it would leave the agent classifying a unit as
// covered by a test that is no longer there. Supplying evidence costs the agent
// a judgement it is already making; withholding it costs the author a wrong
// assertion.
func testPaths(p *profile.Profile, hunks []git.Hunk) []string {
	paths := make([]string, 0, len(hunks))
	seen := make(map[string]struct{}, len(hunks))
	for i := range hunks {
		file := hunks[i].Path
		if file == "" || !p.IsTestFile(file) {
			continue
		}
		if _, repeated := seen[file]; repeated {
			continue
		}
		seen[file] = struct{}{}
		paths = append(paths, file)
	}
	return paths
}

// referenced reads §4.4.1's second half, and reports the lens unavailable
// rather than returning a list cr never established.
//
// A file the index cannot answer for does not fail the round and does not
// vanish: the symbols cr did read are still attached, and the files it could not
// read are named in the §4.5.4 entry. Partial evidence with the gap stated is
// worth more than either half of the alternative.
func referenced(p *profile.Profile, refs References, paths []string) ([]string, []Unavailable) {
	symbols, unavailable := make([]string, 0), make([]Unavailable, 0, 1)
	if reason := indexReason(p, refs); reason != "" {
		return symbols, append(unavailable, Unavailable{Lens: SymbolLens, Reason: reason})
	}
	seen := make(map[string]struct{}, len(paths))
	unread := make([]string, 0, len(paths))
	for _, file := range paths {
		named, indexed := refs.Referenced(file)
		if !indexed {
			unread = append(unread, file)
			continue
		}
		for _, symbol := range named {
			if _, repeated := seen[symbol]; repeated {
				continue
			}
			seen[symbol] = struct{}{}
			symbols = append(symbols, symbol)
		}
	}
	if len(unread) > 0 {
		unavailable = append(unavailable, Unavailable{
			Lens: SymbolLens,
			Reason: "cr could not read " + strings.Join(unread, ", ") +
				" at the head, so the symbols they reference are not attached",
		})
	}
	return symbols, unavailable
}

// indexReason names why no index can answer for any test file at all, and is
// empty when one can.
//
// The profile clauses are the pair unit.Detectable weighs, read the loud way.
// That function answers the same pair with one bit and no error because §3.4.3
// lets its fallthrough be silent — adjacency still produces units, and nothing
// the reader would have been told about goes missing. §4.4.1 has no second
// branch to fall to: with no index there are no referenced symbols, and a silent
// empty list reads as "cr looked and found none", an assertion cr never made.
//
// The four reasons are the four states that leave refs nil, in the order that
// decides between them, and each names what would make the half run, as
// reinvention's own reasons do for §4.3.1. A repository no profile matched has
// no profile to add a language to, so naming an empty profile id there would
// send the reader to edit a file that does not exist; the fix is choosing a
// profile, as profile.MissingProfile says.
func indexReason(p *profile.Profile, refs References) string {
	switch {
	case p.ID == "":
		return "no profile matched this repository, so §4.3.1's symbol index cannot be built; " +
			"set `profile` in the per-repository config to name the profile this repository is"
	case p.Symbols.Lang == "":
		return fmt.Sprintf(
			"profile %q declares no symbols.lang, so §4.3.1's symbol index cannot be built; "+
				"set symbols.lang in the profile to name this repository's language",
			p.ID,
		)
	case !symbol.Supported(p.Symbols.Lang):
		return fmt.Sprintf(
			"profile %q declares symbols.lang %q, which cr has no symbol scanner for; "+
				"set symbols.lang to a language cr can index",
			p.ID, p.Symbols.Lang,
		)
	case refs == nil:
		return fmt.Sprintf("cr built no symbol index for symbols.lang %q", p.Symbols.Lang)
	}
	return ""
}
