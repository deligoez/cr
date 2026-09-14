package cli

import (
	"encoding/json"
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The claims statusHome's round leaves unimplemented: CR-7#c2, set aside, and
// CR-7#c3, still open, with the text each was extracted from.
var unimplementedClaims = []string{
	fixtureIssue + "#c2", "Load retries.", fixtureIssue + "#c3", "Load logs.",
}

// §4.1.3 through the commands: an unimplemented-claim entry reaches `cr status`,
// and it reaches neither findings.ndjson, nor draft.md, nor the payload `cr post`
// would send — v0.1 has no unanchored comment channel per §1.6.1. The reviewer
// raises it with the author out of band and records the answer as a §3.6 note,
// and that note moves nothing into the draft either.
func TestAnIntentGapReachesStatusAndNeverTheDraftOrThePayload(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	queued := aStoredRecord("f1", finding.StateDraft)
	queued.Kind, queued.Grade, queued.Axis = finding.KindQuestion, finding.GradeArgued, "correctness"
	queued.Anchor = finding.Anchor{Path: "lib.go", Side: "RIGHT", StartLine: 4, Line: 4}
	queued.Summary = "Does Load drop the error parse returns?"
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, state.ReplaceStamped(held, state.FileFindings,
		state.Stamp{Head: meta.Head, Round: meta.Round}, []*finding.Finding{queued}))
	require.NoError(t, held.Unlock())

	status, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report struct {
		Intent intentCoverage `json:"intent"`
	}
	require.NoError(t, json.Unmarshal([]byte(status), &report))
	reported := make([]string, 0, len(report.Intent.Gaps))
	for i := range report.Intent.Gaps {
		reported = append(reported, report.Intent.Gaps[i].Claim)
	}
	assert.Equal(t, []string{fixtureIssue + "#c2", fixtureIssue + "#c3"}, reported,
		"§4.1.3 and §10.1.2: cr status reports every entry")

	_, err = runCLIPrinting(t, "note", fixtureIssue, "Logging is a follow-up issue.",
		"--source", "chat", "--pr", fixturePR)
	require.NoError(t, err, "§3.6: the out-of-band answer is recorded as a note")

	drafted := draftedFixture(t, layout)
	require.Contains(t, drafted, `<!-- cr:record id="f1" `, "the control: the round's record is drafted")
	payload, err := runPost(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	require.Contains(t, payload, `"comments"`, "the control: the dry run printed the payload")
	stored, err := layout.ReadPR(fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)

	for _, gap := range unimplementedClaims {
		assert.NotContainsf(t, drafted, gap, "§4.1.3: %s is never drafted", gap)
		assert.NotContainsf(t, payload, gap, "§4.1.3: %s is never posted", gap)
		assert.NotContainsf(t, string(stored), gap, "§4.1.3: %s never becomes a record", gap)
	}
}

// The structural half of §4.1.3's "never": intent-gaps.ndjson is named by the
// command that derives it, the command that stamps a set-aside on it, and the
// report that lists it — and by nothing that writes a record, a draft or a
// payload. A reader added anywhere else is a route from an entry towards a
// comment, and has to be a deliberate change here.
func TestOnlyTheMappingTheSetAsideAndStatusNameTheIntentGaps(t *testing.T) {
	var naming []string
	eachSourceFile(t, func(rel string, file *ast.File) {
		rel = filepath.ToSlash(rel)
		if filepath.Dir(rel) == "internal/state" {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if selector, isSelector := n.(*ast.SelectorExpr); isSelector &&
				selector.Sel.Name == "FileIntentGaps" && !slices.Contains(naming, rel) {
				naming = append(naming, rel)
			}
			return true
		})
	})
	slices.Sort(naming)
	assert.Equal(t, []string{
		"internal/cli/claims.go", "internal/cli/mapcmd.go", "internal/cli/status.go",
	}, naming, "§4.1.3: an unimplemented-claim entry has no route into a record, a draft or a payload")
}
