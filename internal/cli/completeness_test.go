package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/state"
)

// approvalWords are the words §10.2 forbids cr to say about a round.
//
// §10.2 has cr neither approve the pull request nor describe a complete round
// as a settled review: v0.1 ends at posting, and whether the author addressed
// anything is outside what cr can observe. The list is the vocabulary a reader
// would take as either — an approval, or a review that is over — and it is
// asserted against the whole printed report rather than against one sentence,
// because a word anywhere in the output is a word the reader reads.
var approvalWords = []string{
	"approve", "approved", "approval", "lgtm", "looks good", "sign off",
	"signed off", "settled", "ship it", "good to merge", "ready to merge",
}

// completeStatusHome is statusHome's round with all four §10.2 conditions met:
// every unit holds a row for every active role at its current unit hash, and
// every claim is mapped to a unit. The head has not moved and the round holds
// no record at all, so §10.2.1 and §10.2.4 hold as statusHome leaves them.
//
// It is a fixture of its own rather than an edit to statusHome, because
// statusHome is deliberately a round that is not complete — an oversized unit,
// a coverage gap, and two unimplemented claims — and §10.1's own tests measure
// exactly that.
func completeStatusHome(t *testing.T) {
	t.Helper()
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)

	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	cells := ""
	for _, unit := range []struct{ id, hash string }{{"u1", "h1"}, {"u2", "h2"}} {
		for _, role := range meta.ActiveRoles {
			cells += `{"unit":"` + unit.id + `","role":"` + role + `","result":"pass",` +
				`"unit_hash":"` + unit.hash + `","head":"` + meta.Head + `","round":1}` + "\n"
		}
	}
	require.NoError(t, held.Write(state.FileCoverage, []byte(cells)))
	pairs := ""
	for _, claim := range []string{"#c1", "#c2", "#c3"} {
		pairs += `{"claim":"` + fixtureIssue + claim + `","unit":"u1",` +
			`"head":"` + meta.Head + `","round":1}` + "\n"
	}
	require.NoError(t, held.Write(state.FileMapping, []byte(pairs)))
	require.NoError(t, held.Unlock())
}

// completenessReport is the half of `cr status`'s document §10.2 answers.
type completenessReport struct {
	Completeness coverage.Completeness `json:"completeness"`
	Honesty      []string              `json:"honesty"`
}

// readCompleteness runs `cr status` over the fixture and decodes the verdict.
func readCompleteness(t *testing.T) completenessReport {
	t.Helper()
	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report completenessReport
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report
}

// A complete round is reported complete, and the verdict is printed together
// with every lens of §4.5.4 that did not run.
//
// That pairing is the whole of §10.2's second paragraph: completeness across
// three axes is not completeness across four, and neither is completeness with
// a role that never looked. The fixture resolves the shipped `generic` profile,
// which declares no `tests.cmd` and no `symbols.lang`, so the round is complete
// and four lenses did not look — which is exactly the case a verdict printed
// alone would misrepresent.
func TestACompleteVerdictIsPrintedWithEveryLensThatDidNotRun(t *testing.T) {
	completeStatusHome(t)

	report := readCompleteness(t)

	assert.True(t, report.Completeness.Complete)
	assert.Empty(t, report.Completeness.Reasons,
		"§10.2 asks for a reason only when the verdict is false")

	honesty := strings.Join(report.Honesty, "\n")
	assert.Contains(t, honesty, "round complete, per §10.2")
	for _, lens := range []string{
		"axis test disabled, per §4.5.2",
		"lens convention/reinvention unavailable, per §4.3.1",
		"lens test/symbols unavailable, per §4.5.4",
		"role test-adequacy skipped, per §4.6.4",
	} {
		assert.Contains(t, honesty, lens,
			"§10.2: a verdict of complete is printed together with every lens that did not run")
	}
}

// A round that is not complete carries the exact reason, naming the §10.2 item
// that blocked it.
//
// statusHome's round breaks two of the four: `u2` holds no cell for any active
// role, and two of its three claims are mapped to no unit, one of them without
// §4.1.8's stamp.
func TestAnIncompleteVerdictCarriesTheExactReason(t *testing.T) {
	statusHome(t)

	report := readCompleteness(t)

	assert.False(t, report.Completeness.Complete)
	require.NotEmpty(t, report.Completeness.Reasons)
	joined := strings.Join(report.Completeness.Reasons, "\n")
	assert.Contains(t, joined, "§10.2.2: 1 of 2 unit(s) hold no complete row")
	assert.Contains(t, joined,
		"§10.2.3: 1 claim(s) are mapped to no unit and not set aside: "+fixtureIssue+"#c3",
		"§4.1.8's stamp takes #c2 out of the blocking set and leaves #c3 in it")
	assert.Contains(t, strings.Join(report.Honesty, "\n"), "round not complete, per §10.2: ")
}

// cr never approves the pull request and never describes a complete round as a
// settled review, in either output shape (§10.2).
//
// The round asserted over is the complete one, because that is the only round
// on which such a sentence could be tempting: a report saying a round is not
// complete cannot be read as an approval however it is worded.
func TestNoApprovalOrSettledWordingIsReachableOnACompleteRound(t *testing.T) {
	completeStatusHome(t)

	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	shown := throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color")

	for _, shape := range map[string]string{"the document": printed, "the terminal": shown} {
		for _, word := range approvalWords {
			assert.NotContains(t, strings.ToLower(shape), word,
				"§10.2: cr must not approve the pull request or call a complete round settled")
		}
	}
	// The sentence cr does say, so the assertion above is over a report that
	// reached the verdict rather than over one that never printed it.
	assert.Contains(t, shown, "round complete, per §10.2")
}

// §11.1's exemption covers the verdict too, because §10.2 has it printed
// together with the lenses that §11.1 protects: a verdict `--quiet` could
// suppress while the lens list stayed would break that pairing from the other
// side.
func TestQuietSuppressesNeitherTheVerdictNorItsLenses(t *testing.T) {
	completeStatusHome(t)

	loud := readCompleteness(t)
	quieted, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug, "--quiet")
	require.NoError(t, err)
	var underQuiet completenessReport
	require.NoError(t, json.Unmarshal([]byte(quieted), &underQuiet))

	assert.Equal(t, loud.Honesty, underQuiet.Honesty)
	assert.Contains(t, strings.Join(underQuiet.Honesty, "\n"), "round complete, per §10.2")
}
