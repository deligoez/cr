package cli

import (
	"encoding/json"
	"go/ast"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// stampedFileConstants is §2.3.3's set as the tree spells it, which is what a
// walk over the source can see. The values are checked against
// state.StampedFiles below, so a file added to §2.3.3 and left out here fails
// rather than quietly narrowing the guard.
var stampedFileConstants = map[string]string{
	"FileClaims":     state.FileClaims,
	"FileUnits":      state.FileUnits,
	"FileMapping":    state.FileMapping,
	"FileFindings":   state.FileFindings,
	"FileProbes":     state.FileProbes,
	"FileRuns":       state.FileRuns,
	"FileIntentGaps": state.FileIntentGaps,
	"FileCoverage":   state.FileCoverage,
}

// unstampedFileConstants are the per-PR NDJSON files §2.3.3 does not list, so
// their records carry no round and §9.3.5 has no scoping to impose on a read of
// them. Two of them are §9.3.5's own exemptions — waivers.ndjson per §7.4 and
// posted-index.ndjson per §9.3.6 — and the exemption is by construction: they
// hold no round to be scoped by, so a reader could not narrow to one if it
// tried. §3.6's context store is the third, and it is not here because it is
// not per-PR state at all.
var unstampedFileConstants = map[string]bool{
	"FilePostedIndex": true,
	"FileThreads":     true,
	"FileTransitions": true,
	"FileWaivers":     true,
}

// crossRoundReaders are the production reads of a §2.3.3 file that are not
// scoped to one round, each with the sentence that exempts it.
//
// Every one of them is an identity that outlives a round rather than a record
// of one: a probe id, a run id, a baseline at a head, or the repository's whole
// posting history. None of them is a round's working state, which is what
// §9.3.5 calls history the moment the round closes.
var crossRoundReaders = map[string]string{
	"internal/cli/grade.go: readRoundEvidence": "§5.5.1 keeps every probe record the pull request " +
		"ever wrote and §5.5.3 decides at grading time which of them may stand behind a grade, so " +
		"the ladder is asked of each record rather than of the round; runs.ndjson is read the same " +
		"way because §5.2.6 admits a baseline performed at this head whichever round performed it. " +
		"The mapping is scoped where it is used: probe.MapClaim and forceUnmappedIntent are each " +
		"given the round.",
	"internal/cli/probe.go: probeBaselines": "§5.2.6 reuses a baseline performed at this head, and " +
		"a head outlives the round that first measured it.",
	"internal/cli/probe.go: runGapProbe": "probe.NextID reserves an id no record has taken, and " +
		"§5.5.1 keeps every record the pull request ever wrote, so an id drawn from one round's " +
		"records would be issued a second time by the next.",
	"internal/cli/probe.go: appendProbe": "probe.NextID, for the reason runGapProbe gives.",
	"internal/cli/provenance.go: readProbes": "a probe is looked up by id for §8.1.7's evidence " +
		"region, and §5.5.3 has already decided at grading time which records may stand behind the " +
		"grade this renders.",
	"internal/cli/rulessuggest.go: postedComments": "§2.6.3.1 harvests every recorded round of " +
		"every pull request of the repository, so there is no one round to scope to.",
	"internal/cli/test.go: appendRun": "run.NextID, for the reason probe.NextID is.",
}

// §9.3.5's first sentence over the tree: a command reads only the current
// round's records.
//
// The set that has to obey it is the one nobody can keep by hand — a reader
// written next month that filters downstream, or not at all. So it is closed
// from the other end: state.ReadStamped is the round-scoped read, and every
// other read of a §2.3.3 file is found here and has to be declared with the
// reason §9.3.5 does not bind it.
//
// A read whose file argument is not a §2.3 constant at all counts as
// undeclared too. A helper taking the file name as a parameter is exactly how a
// whole-file read hides from a walk like this one, and the two reads it would
// hide are the ones this guard exists to find.
func TestEveryStampedReadIsRoundScopedOrDeclared(t *testing.T) {
	require.ElementsMatch(t, state.StampedFiles(), slices.Collect(maps.Values(stampedFileConstants)),
		"§2.3.3's set is internal/state's, and this guard has to be measuring the same one")

	found, scoped := map[string]bool{}, 0
	eachSourceFile(t, func(rel string, file *ast.File) {
		for _, decl := range file.Decls {
			within, isFunc := decl.(*ast.FuncDecl)
			if !isFunc {
				continue
			}
			ast.Inspect(within, func(n ast.Node) bool {
				call, isCall := n.(*ast.CallExpr)
				if !isCall {
					return true
				}
				switch calledName(call) {
				case "ReadStamped":
					scoped++
				case "ReadRecords":
					if !unstampedFileConstants[fileArgOf(call)] {
						found[filepath.ToSlash(rel)+": "+within.Name.Name] = true
					}
				}
				return true
			})
		}
	})

	require.Greater(t, scoped, 5,
		"only %d round-scoped reads were found, so this guard measured almost nothing", scoped)
	assert.ElementsMatch(t,
		slices.Collect(maps.Keys(crossRoundReaders)), slices.Collect(maps.Keys(found)),
		"§9.3.5 scopes every read of a §2.3.3 file: read it through state.ReadStamped, "+
			"or declare here why the round does not bind it")
}

// §9.3.5 over a pull request that has been through two rounds: round 2's
// commands see round 2's records and none of round 1's, while the three stores
// §9.3.5 exempts are read across the boundary exactly as they were written.
//
// It is measured through `cr status` rather than through the readers, because
// the sentence binds commands. The round is opened at the same head the first
// was recorded at, so §9.3.1's comparison still agrees and what is measured is
// the scoping and not a refusal.
//
// The exemptions are asserted twice over, and the second half is the one that
// lasts. That they load is a fact about today's fixture; that ReadStamped
// refuses their files at all is a fact about the design — none of the three
// carries §2.3.3's round, so a reader could not narrow one to a round if it
// tried, and the exemption cannot be forgotten by a command written later.
func TestRoundTwoSeesNoneOfRoundOnesRecordsWhileTheExemptionsLoad(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	openSecondRound(t, layout)

	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	var report struct {
		Round    int           `json:"round"`
		Coverage coverage.Rows `json:"coverage"`
		Intent   struct {
			Claims int           `json:"claims"`
			Mapped int           `json:"mapped"`
			Gaps   []mapping.Gap `json:"gaps"`
		} `json:"intent"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))

	assert.Equal(t, 2, report.Round)
	assert.Equal(t, coverage.Rows{Units: 1, Complete: 1, Roles: 3}, report.Coverage,
		"§9.3.5: round 1's two units and their cells are history, and round 2 formed one unit")
	assert.Equal(t, 1, report.Intent.Claims, "round 1's three claims are not round 2's")
	assert.Equal(t, 0, report.Intent.Mapped, "round 1's mapping is not round 2's")
	assert.Empty(t, report.Intent.Gaps, "round 1's §4.1.3 entries are not round 2's")

	waivers, err := finding.PullRequestWaivers(layout, fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	assert.Len(t, waivers, 1, "§7.4's waivers apply across rounds")
	notes, err := note.Load(layout, fixtureIssue)
	require.NoError(t, err)
	assert.Len(t, notes, 1, "§3.6's context store applies across rounds")
	posted, err := state.ReadRecords[finding.WaiverKey](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FilePostedIndex)
	require.NoError(t, err)
	assert.Len(t, posted, 1, "§9.3.6's posted index applies across rounds")

	for name := range unstampedFileConstants {
		_, err := state.ReadStamped[struct{}](
			layout, fixtureOwner, fixtureProject, fixturePRNumber, fileNamed(t, name), 2)
		assert.Errorf(t, err, "%s carries no round, so a round-scoped read of it is refused", name)
	}
}

// openSecondRound moves the fixture into round 2 at the same head: round 2's
// own unit, cells and claim are added under round 1's records, and one of each
// of §9.3.5's three exemptions is recorded while round 1 is still the current
// one.
func openSecondRound(t *testing.T, layout state.Layout) {
	t.Helper()
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	head := meta.Head

	_, err = note.Append(layout, fixtureIssue, "the retry is deliberate",
		note.SourceChat, fixturePRNumber, time.Now())
	require.NoError(t, err)

	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	add := func(name, lines string) {
		t.Helper()
		body, readErr := layout.ReadPR(fixtureOwner, fixtureProject, fixturePRNumber, name)
		require.NoError(t, readErr)
		require.NoError(t, held.Write(name, append(body, []byte(lines)...)))
	}
	add(state.FileUnits,
		`{"id":"u3","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":3,"end":6}],`+
			`"hash":"h3","oversized":false,"head":"`+head+`","round":2}`+"\n")
	cells := ""
	for _, role := range []string{"convention", "correctness", "intent-coverage"} {
		cells += `{"unit":"u3","role":"` + role + `","result":"pass","unit_hash":"h3",` +
			`"head":"` + head + `","round":2}` + "\n"
	}
	add(state.FileCoverage, cells)
	add(state.FileClaims,
		`{"id":"`+fixtureIssue+`#c4","text":"Load stores.","source":"acceptance",`+
			`"span":"stores","head":"`+head+`","round":2}`+"\n")

	// §7.4's waiver and §9.3.6's posted-index entry, both recorded in round
	// 1 and both expected to be read in round 2.
	require.NoError(t, held.Write(state.FileWaivers,
		[]byte(`{"id":"wp1","path":"lib.go","side":"RIGHT","class":"missing-error-check",`+
			`"content_hash":"9a8b7c6","disposition":"not-here","round":1,"pr":7,`+
			`"head":"`+head+`"}`+"\n")))
	require.NoError(t, held.Write(state.FilePostedIndex,
		[]byte(`{"path":"lib.go","side":"RIGHT","class":"unchecked-error",`+
			`"content_hash":"0f1e2d3"}`+"\n")))

	meta.Round = 2
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())
}

// fileNamed is one of internal/state's per-PR file constants by the name the
// guard above spells it with.
func fileNamed(t *testing.T, constant string) string {
	t.Helper()
	named := map[string]string{
		"FilePostedIndex": state.FilePostedIndex,
		"FileThreads":     state.FileThreads,
		"FileTransitions": state.FileTransitions,
		"FileWaivers":     state.FileWaivers,
	}
	name, known := named[constant]
	require.True(t, known, "%s is declared unstamped and this helper does not know it", constant)
	return name
}

// fileArgOf is the §2.3 file a read names, as the source spells it, and the
// empty string for a read whose file is not one of internal/state's constants.
//
// The position is the one both readers share, so the argument tested is the
// file rather than whichever argument happened to be a selector.
func fileArgOf(call *ast.CallExpr) string {
	const fileArg = 4
	if len(call.Args) <= fileArg {
		return ""
	}
	named, isSelector := call.Args[fileArg].(*ast.SelectorExpr)
	if !isSelector {
		return ""
	}
	return named.Sel.Name
}
