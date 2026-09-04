package reinvention

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/symbol"
)

// indexable is a profile cr can build a symbol index for, so a case about the
// attachment is not also a case about §4.3.1's unavailability path.
func indexable() *profile.Profile {
	return &profile.Profile{ID: "fixture", Symbols: profile.Symbols{Lang: "go"}}
}

// head is the index one round is attached against.
func head(decls ...symbol.Decl) *symbol.Index {
	return &symbol.Index{Lang: "go", Decls: decls}
}

// decl is one declaration of the head, spelled short enough that a table stays
// readable.
func decl(path string, line int, name string) symbol.Decl {
	return symbol.Decl{Path: path, Line: line, Name: name, Kind: symbol.Function, Params: 1}
}

// addedAt is a hunk whose changed lines are the head-side lines given, which is
// how §3.4.1 numbers a hunk that adds anything.
func addedAt(path string, lines ...int) git.Hunk {
	changed := make([]git.ChangedLine, 0, len(lines))
	for _, line := range lines {
		changed = append(changed, git.ChangedLine{Side: git.Right, Line: line})
	}
	return git.Hunk{Path: path, Side: git.Right, Changed: changed}
}

// names reads the candidate names out of an attachment, so an assertion says
// which symbols were offered rather than repeating four fields of each.
func names(decls []symbol.Decl) []string {
	out := make([]string, 0, len(decls))
	for _, d := range decls {
		out = append(out, d.Name)
	}
	return out
}

// §4.3.1: for every function, method, or class added by the diff, the
// candidates are the head symbol index minus every symbol the diff itself
// declares on a changed line.
func TestEveryAddedSymbolIsAttachedTheRestOfTheHeadIndex(t *testing.T) {
	index := head(
		decl("app/existing.go", 10, "FormatMoney"),
		decl("app/existing.go", 20, "ParseMoney"),
		decl("app/new.go", 3, "MoneyFormat"),
	)

	attachments := Attach(indexable(), index, []git.Hunk{addedAt("app/new.go", 3)})

	require.Len(t, attachments.Attached, 1, "one declaration sits on a changed line")
	assert.Empty(t, attachments.Unavailable, "the lens ran, so §4.5.4 has nothing to report")
	assert.Equal(t, "MoneyFormat", attachments.Attached[0].Added.Name)
	assert.Equal(t, []string{"FormatMoney", "ParseMoney"}, names(attachments.Attached[0].Candidates))
}

// The subtraction is what this attachment exists for: a symbol the diff
// declares is never its own candidate. Without it cr would ask the author
// whether the function they are looking at reinvents itself — and §4.3.4 makes
// that a question posted to a colleague, which is the exact cost the trust
// economy is written against.
func TestASymbolTheDiffDeclaresIsNeverItsOwnCandidate(t *testing.T) {
	index := head(
		decl("app/existing.go", 10, "FormatMoney"),
		decl("app/new.go", 3, "MoneyFormat"),
		decl("app/new.go", 9, "MoneyParse"),
	)

	attachments := Attach(indexable(), index, []git.Hunk{addedAt("app/new.go", 3, 9)})

	require.Len(t, attachments.Attached, 2)
	for _, attachment := range attachments.Attached {
		assert.NotContains(t, names(attachment.Candidates), attachment.Added.Name,
			"%s is declared by this diff, so it is not a pre-existing candidate for itself",
			attachment.Added.Name)
		// And not for its neighbour either: §4.3.1 subtracts every
		// symbol the diff declares, not just the one being attached.
		// Two helpers added in one change are a decision the author
		// made deliberately, minutes ago, in the same diff.
		assert.Equal(t, []string{"FormatMoney"}, names(attachment.Candidates))
	}
}

// A LEFT-side changed line is numbered in the merge base, where the same number
// names a different line. Testing one against a head-built index would subtract
// whatever happens to sit at that line now — a pre-existing symbol silently
// dropped from every candidate pool, with nothing saying it was.
func TestALeftSideChangedLineSubtractsNothing(t *testing.T) {
	index := head(decl("app/existing.go", 10, "FormatMoney"), decl("app/new.go", 3, "MoneyFormat"))
	deleted := git.Hunk{
		Path:    "app/existing.go",
		Side:    git.Left,
		Changed: []git.ChangedLine{{Side: git.Left, Line: 10}},
	}

	attachments := Attach(indexable(), index, []git.Hunk{addedAt("app/new.go", 3), deleted})

	require.Len(t, attachments.Attached, 1, "a hunk that adds nothing declares nothing at the head")
	assert.Equal(t, []string{"FormatMoney"}, names(attachments.Attached[0].Candidates))
}

// A changed line that declares nothing adds no attachment. A pull request can
// change a hundred lines of a function body without declaring one symbol, and
// §4.3.1 attaches candidates to added declarations, not to added lines.
func TestAChangedLineThatDeclaresNothingAttachesNothing(t *testing.T) {
	index := head(decl("app/existing.go", 10, "FormatMoney"))

	assert.Empty(t, Attach(indexable(), index, []git.Hunk{addedAt("app/existing.go", 11, 12, 13)}).Attached)
	assert.Empty(t, Attach(indexable(), index, nil).Attached, "a round with no diff adds no symbol")
}

// A head that declares nothing else still attaches, with an empty candidate
// list that serialises as `[]`. The attachment is the record that cr looked;
// dropping it would leave the added symbol out of §4.6.1's prompt entirely.
func TestAnAddedSymbolWithNoCandidatesStillAttaches(t *testing.T) {
	attachments := Attach(indexable(),
		head(decl("app/new.go", 3, "MoneyFormat")), []git.Hunk{addedAt("app/new.go", 3)})

	require.Len(t, attachments.Attached, 1)
	assert.Empty(t, attachments.Attached[0].Candidates)
	encoded, err := json.Marshal(attachments.Attached[0])
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"candidates":[]`)
}

// Each attachment owns its candidate list, so §4.3.2's ranking can narrow or
// sort one in place without moving another's. Sharing one backing array would
// make the second added symbol's candidates depend on what was done to the
// first's, which is the kind of defect that shows up as a wrong question weeks
// later.
func TestEachAttachmentOwnsItsCandidateList(t *testing.T) {
	index := head(
		decl("app/existing.go", 10, "FormatMoney"),
		decl("app/existing.go", 20, "ParseMoney"),
		decl("app/new.go", 3, "MoneyFormat"),
		decl("app/new.go", 9, "MoneyParse"),
	)

	attachments := Attach(indexable(), index, []git.Hunk{addedAt("app/new.go", 3, 9)})

	require.Len(t, attachments.Attached, 2)
	attachments.Attached[0].Candidates[0].Name = "overwritten"
	assert.Equal(t, "FormatMoney", attachments.Attached[1].Candidates[0].Name)
}

// Both collections serialise as `[]` and never as `null`, per §12.
func TestAnEmptyAttachmentSerialisesAsAnEmptyArray(t *testing.T) {
	attachments := Attach(indexable(), head(), nil)

	encoded, err := json.Marshal(attachments)
	require.NoError(t, err)
	assert.JSONEq(t, `{"attached":[],"unavailable":[]}`, string(encoded))
}
