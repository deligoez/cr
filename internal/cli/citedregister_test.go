package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// §6.2.4: record-time validation establishes that a citation **exists**, never
// that it **supports** the summary, and `cr` MUST NOT describe a `cited` record
// as verified, confirmed, or proven in any output.
//
// The clause is a register rule, and it is the one place §6.2's weakest
// assertable grade could be inflated into something it is not. All cr did was
// open the file the citation names and check the line is there; whether that
// line supports the summary is the judgement §2.1.3 gives the agent and §6.2.6
// gives the human, by rendering every citation verbatim. A sentence calling the
// record verified would spend the reviewer's standing on a claim nobody made.
//
// # Scope
//
// The fence is over cr's own output words, which in Go means its string
// literals. Comments are outside it and deliberately: this file's own doc
// comment names all three words in order to forbid them, `cli/record.go`
// explains why its `supported` map says neither "proves" nor "confirms", and a
// guard those tripped is a guard someone deletes.
//
// The agent's own prose is outside it from the other side. §8.1.7 renders
// `evidence` and a probe's `output_tail` verbatim, so an agent that wrote
// "verified" in its evidence sees that word in the comment — and that is the
// agent's sentence, attributed to the agent, which is exactly what verbatim
// rendering is for. §6.2.4 binds what cr says, not what cr quotes. The record
// fixtures in this package say all three words for that reason.
//
// Identifiers are outside it too, which is why the match is by whole word:
// `WaiverProvenance`, `render.provenance_region` and §3.6.6's "unverified
// hearsay" are not the words this forbids, and a substring match would convict
// every one of them.

// citedRegisterWords are §6.2.4's three words, matched whole so that
// "provenance" is not read as "proven" and "unverified" is not read as
// "verified". Case is ignored, because a sentence starting with one of them is
// the same sentence.
var citedRegisterWords = regexp.MustCompile(`(?i)\b(verified|confirmed|proven)\b`)

// No string literal cr ships may call anything verified, confirmed, or proven.
//
// The fence is wider than §6.2.4's sentence, and that is the honest shape for
// it. A literal is written long before it is known which record it will
// describe: cr composes one sentence for a whole class of records, so a check
// that only fired for a `cited` one would have to decide at authoring time
// which grade a format string can reach, which nothing can do. What §6.2.4
// costs by being enforced this way is a word cr has no other use for — cr
// validates, records and reports, and every one of those is a weaker verb than
// all three of these.
//
// Test files are excluded because they are inputs and assertions rather than
// output, and this file excludes itself, since the three words above would
// otherwise be read as the violation they exist to find.
func TestNoStringLiteralCrShipsCallsARecordVerified(t *testing.T) {
	root, self := moduleRoot(t), selfPath(t)

	// The matcher itself, before it is trusted over a thousand literals.
	// Whole-word matching is what lets this fence coexist with §7.4.8's
	// provenance and §3.6.6's unverified hearsay, both of which cr says
	// often and neither of which is §6.2.4's claim; a substring match would
	// convict every one of them and the fence would be turned off.
	for _, innocent := range []string{
		"the provenance region of §8.1.6", "unverified hearsay", "render.provenance_region",
	} {
		require.NotRegexp(t, citedRegisterWords, innocent,
			"the fence must not convict a word that merely contains one of §6.2.4's")
	}
	for _, convicted := range []string{
		"the citation is verified", "Confirmed by the run", "the gap is proven",
	} {
		require.Regexp(t, citedRegisterWords, convicted,
			"and it must convict §6.2.4's own three, wherever they sit in the sentence")
	}

	scanned := 0
	var found []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir():
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		case filepath.Ext(path) != ".go",
			strings.HasSuffix(path, "_test.go"),
			path == self:
			return nil
		}
		scanned++
		// Parsed rather than read as text, so the surface is the
		// literals alone: mode 0 leaves the comments out of the tree,
		// which is the exclusion this fence depends on.
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require.NoError(t, parseErr, "%s does not parse", path)
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			text, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr != nil {
				return true
			}
			if word := citedRegisterWords.FindString(text); word != "" {
				rel, relErr := filepath.Rel(root, path)
				require.NoError(t, relErr)
				found = append(found, fmt.Sprintf("%s says %q in %q", rel, word, text))
			}
			return true
		})
		return nil
	}))

	// A walk that found no files would pass and would mean nothing.
	require.Greater(t, scanned, 20, "only %d files were scanned, so this guard proved nothing", scanned)
	assert.Empty(t, found,
		"§6.2.4: cr must not describe a record as verified, confirmed, or proven; "+
			"cited means only that a human-checkable location was supplied")
}

// The same rule read off what a `cited` record actually renders to.
//
// The fence above is over the literals cr could print; this is over what one
// cited record does print, through every rendering v0.1 has for it. The two
// answer different questions — a sentence assembled from harmless fragments
// would pass the first — and only this one reads the finished text.
//
// The record's own evidence says all three words, and they are expected in the
// output that echoes the stored record back: §6.2.4 binds cr's description of
// the record, not the record's own prose, and `cr record` hands back what it
// stored so the caller can see the fields cr wrote. So the agent's sentence is
// removed from what is examined, and what is left is cr's.
func TestNoRenderingOfACitedRecordCallsItVerified(t *testing.T) {
	layout := gradedHome(t)

	cited := aGradedRecord("f1")
	cited["citations"] = []map[string]any{{"path": "app.go", "line": 1}}
	agentProse, ok := cited["evidence"].(string)
	require.True(t, ok, "the fixture's evidence is the agent's own sentence")
	file := writeRecordFile(t, "merged.ndjson", cited)

	printed, err := runRecord(t, "7", file, "--repo", fixtureSlug)
	require.NoError(t, err)
	stored, err := state.ReadRecords[finding.Finding](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	require.Equal(t, finding.GradeCited, stored[0].Grade,
		"the fixture has to actually be graded cited for this to test anything")

	rendered := map[string]string{
		"the json payload": printed,
		"the terminal line": throughATerminal(t,
			"record", "7", writeRecordFile(t, "second.ndjson", aGradedRecord("f2")),
			"--repo", fixtureSlug),
	}
	for _, lang := range render.Langs() {
		label, held := render.QuestionLabel(lang, finding.GradeCited)
		require.True(t, held, "§8.1.4 has a built-in label for a cited question in %s", lang)
		rendered["the §8.1.4 question label in "+lang.String()] = label
	}

	for name, text := range rendered {
		t.Run(name, func(t *testing.T) {
			// The agent's own sentence is not cr's description of
			// the record, and §8.1.7 renders it verbatim on
			// purpose. What is examined is everything else.
			crsOwn := strings.ReplaceAll(text, agentProse, "")
			assert.Empty(t, citedRegisterWords.FindAllString(crsOwn, -1),
				"§6.2.4: cited means only that a human-checkable location was supplied")
		})
	}
}
