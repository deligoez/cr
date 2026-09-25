package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/proposal"
	"github.com/deligoez/cr/internal/state"
)

// proposalPrompt is what §4.6.2 gave one prompt of the round: the role, the
// unit, and the first proposal id it may write.
type proposalPrompt struct {
	Role   string `json:"role"`
	Unit   string `json:"unit"`
	FirstP string `json:"first_proposal_id"`
}

// briefedForProposals opens the round over probeFixture's pull request and
// returns the first prompt `cr review` emitted.
//
// The ids come from the prompt rather than from arithmetic written out here.
// §4.6.2's grid is what decides them, and a test that computed its own would
// pass while the prompt and the recorder disagreed — which is the one thing
// §5.7.1's block refusal exists to catch.
func briefedForProposals(t *testing.T, prepared state.Layout) proposalPrompt {
	t.Helper()
	// probeFixture names its profile in meta.json alone, and `cr brief`
	// resolves the profile again: the fixture carries no marker file, so
	// without §2.4.1's per-repository override the round would come back
	// with no profile, no test axis, and every proposal unrunnable.
	require.NoError(t, os.WriteFile(
		prepared.RepoConfig(fixtureOwner, fixtureProject), []byte(`{"profile":"qa"}`), 0o600))
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": the retry backs off.\n"), 0o600))
	_, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	// §4.6.5 refuses the remaining axes until the intent pass has recorded
	// a mapping. Both files are empty, which is a real §4.1.1 answer: every
	// unit may be mapped to zero claims.
	empty := filepath.Join(t.TempDir(), "empty.ndjson")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	_, err = runCLIPrinting(t, "claims", "record", fixturePR, empty,
		"--repo", fixtureSlug, "--intent-file", issue)
	require.NoError(t, err)
	_, err = runCLIPrinting(t, "map", "record", fixturePR, empty, "--repo", fixtureSlug)
	require.NoError(t, err)

	printed, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var fanout struct {
		Prompts []proposalPrompt `json:"prompts"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &fanout))
	require.NotEmpty(t, fanout.Prompts, "the round emits at least one prompt")
	require.NotEmpty(t, fanout.Prompts[0].FirstP, "§4.6.2 gives every prompt a block of proposal ids")
	return fanout.Prompts[0]
}

// proposalFile writes one §5.7 proposal outside the repository under review, at
// the id and seat the prompt gave, and returns its path.
func proposalFile(t *testing.T, at proposalPrompt, names string) string {
	t.Helper()
	line := `{"id":` + mustJSON(t, at.FirstP) +
		`,"kind":"mutation","role":` + mustJSON(t, at.Role) +
		`,"unit":` + mustJSON(t, at.Unit) +
		`,"target":"app.go:3",` +
		`"hypothesis":"No test notices the dropped retry.",` +
		`"settles":"A suite that stays green under the mutation proves the gap.",` +
		`"input":` + mustJSON(t, fixtureDiff)
	if names != "" {
		line += `,"finding":` + mustJSON(t, names)
	}
	path := filepath.Join(t.TempDir(), "proposals.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(line+"}\n"), 0o600))
	return path
}

// mustJSON is a value as a JSON string literal, so a patch's newlines reach the
// file the way §5.7's `input` carries them.
func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

// recordFile writes one record at the prompt's own seat, graded `argued`
// because it cites nothing and names no probe, and returns its path and id.
func recordFile(t *testing.T, at proposalPrompt) (path, id string) {
	t.Helper()
	id = "f" + at.FirstP[1:]
	path = filepath.Join(t.TempDir(), "merged.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(`{"id":`+mustJSON(t, id)+`,"kind":"finding",`+
		`"role":`+mustJSON(t, at.Role)+`,"class":"untested-branch","severity":"high",`+
		`"unit":`+mustJSON(t, at.Unit)+`,`+
		`"anchor":{"path":"app.go","side":"RIGHT","start_line":3,"line":3},`+
		`"summary":"No test exercises the retry.",`+
		`"evidence":"The branch has no assertion behind it."}`+"\n"), 0o600))
	return path, id
}

// §5.7.3 and §5.7.4 end to end: the proposal runs as its kind's probe, becomes
// `run` carrying the probe it produced, and the record it names is re-graded
// from `argued` to `probed`.
//
// The re-grade is the whole point of §5.7, so it is asserted from the stored
// record rather than from the command's own report: what the round holds is
// what the draft will render.
func TestRunningAProposalRegradesTheRecordItNames(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	records, recordID := recordFile(t, at)
	_, err := runCLIPrinting(t, "record", fixturePR, records, "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runCLIPrinting(t, "proposals", "record", fixturePR,
		proposalFile(t, at, recordID), "--repo", fixtureSlug)
	require.NoError(t, err)

	before := storedRecords(t, prepared, state.FileFindings)
	require.Len(t, before, 1)
	require.Equal(t, "argued", before[0]["grade"],
		"a record citing nothing and naming no probe is argued")

	printed, err := runCLIPrinting(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--proposal", at.FirstP)
	require.NoError(t, err)
	var reported map[string]any
	require.NoError(t, json.Unmarshal([]byte(printed), &reported))
	assert.Equal(t, at.FirstP, reported["proposal"])
	assert.Equal(t, map[string]any{
		"record": recordID, "probe": "p1", "was": "argued", "now": "probed",
	}, reported["regraded"], "§5.7.4 reports the grade the record held and the grade it holds")

	after := storedRecords(t, prepared, state.FileFindings)
	require.Len(t, after, 1)
	assert.Equal(t, "probed", after[0]["grade"],
		"§5.7.4: running the experiment is what changes the register")
	assert.Equal(t, "p1", after[0]["probe"])

	proposals := storedRecords(t, prepared, state.FileProposals)
	require.Len(t, proposals, 1)
	assert.Equal(t, "run", proposals[0]["state"])
	assert.Equal(t, "p1", proposals[0]["probe"])
}

// A proposal that names no record still runs and still settles; the probe
// stands on its own and nothing is re-graded.
func TestAProposalNamingNoRecordRunsAndRegradesNothing(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	_, err := runCLIPrinting(t, "proposals", "record", fixturePR,
		proposalFile(t, at, ""), "--repo", fixtureSlug)
	require.NoError(t, err)

	printed, err := runCLIPrinting(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
		"--proposal", at.FirstP)
	require.NoError(t, err)
	var reported map[string]any
	require.NoError(t, json.Unmarshal([]byte(printed), &reported))
	assert.Equal(t, "p1", reported["probe"])
	assert.NotContains(t, reported, "regraded")

	proposals := storedRecords(t, prepared, state.FileProposals)
	require.Len(t, proposals, 1)
	assert.Equal(t, "run", proposals[0]["state"])
}

// §5.7.4 writes a proposal's probe once: a second run of the same proposal is
// refused with the state code, naming the probe the first produced.
func TestAProposalRunsOnce(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	_, err := runCLIPrinting(t, "proposals", "record", fixturePR,
		proposalFile(t, at, ""), "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runCLIPrinting(t, "probe", "run", fixturePR, "--repo", fixtureSlug, "--proposal", at.FirstP)
	require.NoError(t, err)

	err = runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug, "--proposal", at.FirstP)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "proposal "+at.FirstP+" was already run and produced probe p1")
	assert.Equal(t, ExitState, exitCodeFor(err))
}

// §5.7.3 refuses every input flag beside `--proposal`, with the usage code: the
// proposal carries the whole experiment, and a flag beside it would run
// something the role did not propose.
func TestAnInputFlagBesideProposalIsRefused(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	_, err := runCLIPrinting(t, "proposals", "record", fixturePR,
		proposalFile(t, at, ""), "--repo", fixtureSlug)
	require.NoError(t, err)

	for flag, argv := range map[string][]string{
		"--kind":   {"--kind", "mutation"},
		"--patch":  {"--patch", writePatch(t, fixtureDiff)},
		"--test":   {"--test", writePatch(t, "x")},
		"--target": {"--target", "app.go:3"},
		"--filter": {"--filter", "TestRetry"},
		"--path":   {"--path", "app.go"},
	} {
		t.Run(flag, func(t *testing.T) {
			err := runCLI(t, append([]string{
				"probe", "run", fixturePR, "--repo", fixtureSlug, "--proposal", at.FirstP,
			}, argv...)...)
			require.Error(t, err)
			assert.Contains(t, err.Error(), flag+" is rejected beside --proposal")
			assert.Equal(t, ExitUsage, exitCodeFor(err))
		})
	}
}

// A `--proposal` naming nothing the pull request holds is refused with the
// validation code and the step that lists the open ones.
func TestAnUnknownProposalIsRefused(t *testing.T) {
	probeFixture(t, "echo 'Tests:  4 passed'\n")

	err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug, "--proposal", "x9")

	require.Error(t, err)
	assert.Contains(t, err.Error(), `--proposal "x9" names no proposal this pull request holds`)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
}

// §10.1.8: `cr status` reports the round's proposals by state and names the
// open ones, so the reader knows which experiment is still unanswered.
func TestStatusReportsTheRoundsProposals(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	_, err := runCLIPrinting(t, "proposals", "record", fixturePR,
		proposalFile(t, at, ""), "--repo", fixtureSlug)
	require.NoError(t, err)

	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report struct {
		Proposals proposalReport `json:"proposals"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	assert.Equal(t, proposalReport{
		Total: 1, Open: 1, OpenIDs: []string{at.FirstP},
		Unrunnables: []unrunnableProposal{},
	}, report.Proposals)
}

// §5.7.1: a proposal whose id lies outside the block §4.6.2 gave its role on
// its unit is refused once `cr review` has emitted that block.
func TestAProposalIDOutsideItsBlockIsRefused(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	first, ok := proposal.IDSuffix(at.FirstP)
	require.True(t, ok)
	outside := at
	outside.FirstP = proposal.IDOf(first + 1000)

	err := runCLI(t, "proposals", "record", fixturePR, proposalFile(t, outside, ""), "--repo", fixtureSlug)

	require.Error(t, err)
	assert.Contains(t, err.Error(), mustJSON(t, outside.FirstP)+" lies outside "+at.FirstP+"..")
	assert.Empty(t, storedRecords(t, prepared, state.FileProposals))
}

// §5.7.1 refuses an id outside "the block §4.6.2 gave", and a block is given by
// a prompt: on a unit `cr review` has emitted nothing for, no role was told
// which ids to write, so the same out-of-block id is stored.
func TestAProposalIDOutsideABlockNoPromptGaveIsStored(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	meta, err := prepared.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(prepared.FanOutDir(fixtureOwner, fixtureProject, fixturePRNumber, meta.Round, at.Unit)))
	first, ok := proposal.IDSuffix(at.FirstP)
	require.True(t, ok)
	outside := at
	outside.FirstP = proposal.IDOf(first + 1000)

	_, err = runCLIPrinting(t, "proposals", "record", fixturePR, proposalFile(t, outside, ""), "--repo", fixtureSlug)

	require.NoError(t, err)
	stored := storedRecords(t, prepared, state.FileProposals)
	require.Len(t, stored, 1)
	assert.Equal(t, outside.FirstP, stored[0]["id"])
}

// §5.7.1: a proposal whose id a stored proposal already holds is refused,
// naming the round that holds it.
func TestAProposalIDAlreadyStoredIsRefused(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	file := proposalFile(t, at, "")
	_, err := runCLIPrinting(t, "proposals", "record", fixturePR, file, "--repo", fixtureSlug)
	require.NoError(t, err)

	err = runCLI(t, "proposals", "record", fixturePR, file, "--repo", fixtureSlug)

	require.Error(t, err)
	assert.Contains(t, err.Error(), mustJSON(t, at.FirstP)+" is already held by the proposal stored in round ")
	assert.Len(t, storedRecords(t, prepared, state.FileProposals), 1)
}

// `cr proposals record` reports the proposals it stored, in file order, and
// the round they stand in.
func TestRecordingProposalsReportsWhatItStored(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)

	printed, err := runCLIPrinting(t, "proposals", "record", fixturePR,
		proposalFile(t, at, ""), "--repo", fixtureSlug)

	require.NoError(t, err)
	var reported struct {
		Recorded []struct {
			ID string `json:"id"`
		} `json:"recorded"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &reported))
	require.Len(t, reported.Recorded, 1)
	assert.Equal(t, at.FirstP, reported.Recorded[0].ID)
}

// §5.7.5: a proposal carrying `paths` under a profile with no tests.paths_arg
// is stored `unrunnable` with a reason naming the field (the same proposal
// with no paths is stored open, per TestStatusReportsTheRoundsProposals).
// Measured on tarfin-labs/backend#6328 with cr
// 0.13.0: such a proposal was stored open and `cr probe run --proposal` then
// exited 3 with "tests.paths_arg is not set".
func TestAProposalWithPathsTheProfileCannotPassIsStoredUnrunnable(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	pathed := filepath.Join(t.TempDir(), "pathed.ndjson")
	plain, err := os.ReadFile(proposalFile(t, at, ""))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(pathed,
		[]byte(strings.Replace(string(plain), `"kind":`, `"paths":["internal"],"kind":`, 1)), 0o600))

	_, err = runCLIPrinting(t, "proposals", "record", fixturePR, pathed, "--repo", fixtureSlug)

	require.NoError(t, err)
	stored := storedRecords(t, prepared, state.FileProposals)
	require.Len(t, stored, 1)
	assert.Equal(t, "unrunnable", stored[0]["state"])
	assert.Equal(t, "the proposal names paths and the resolved profile qa declares no tests.paths_arg, "+
		"so §5.2.1 has no way to pass them to the runner", stored[0]["reason"])
}

// §5.7.5: a proposal recorded in a round whose profile gives cr no test runner
// is stored `unrunnable` with the reason, which says no profile was resolved.
func TestAProposalWithNoRunnerIsStoredUnrunnable(t *testing.T) {
	prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
	at := briefedForProposals(t, prepared)
	meta, err := prepared.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.ProfileID = ""
	held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())

	_, err = runCLIPrinting(t, "proposals", "record", fixturePR, proposalFile(t, at, ""), "--repo", fixtureSlug)

	require.NoError(t, err)
	stored := storedRecords(t, prepared, state.FileProposals)
	require.Len(t, stored, 1)
	assert.Equal(t, "unrunnable", stored[0]["state"])
	assert.Equal(t, "the resolved profile (none: no profile matched this repository) declares no tests.cmd, "+
		"so §5.2.1 has no runner to perform this experiment", stored[0]["reason"])
}

// §5.7.6 through `cr status`: the round's open proposals are disclosed when
// they outnumber the executions `probe.max_per_round` still leaves, and only
// then.
//
// One probe has already run, so the executions left are the cap less one and
// not the cap itself; a disclosure that added the spent probes instead would
// read the same only while nothing had run. The second case sits on the
// boundary: one open proposal and one execution left is a round that can still
// answer every ask, and saying otherwise is a notice a reader learns to skip.
func TestStatusDisclosesOpenProposalsOnlyPastTheProbesLeft(t *testing.T) {
	for _, tc := range []struct {
		name   string
		capped string
		want   []string
	}{
		{name: "one open proposal and no execution left", capped: "1", want: []string{
			"§5.7.6: the round holds 1 open proposal(s) and probe.max_per_round leaves 0 execution(s) " +
				"of its 1, so not every experiment the roles asked for can be run in this round",
		}},
		{name: "one open proposal and one execution left", capped: "2", want: []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepared, _, _, _ := probeFixture(t, "echo 'Tests:  4 passed'\n")
			at := briefedForProposals(t, prepared)
			_, err := runCLIPrinting(t, "proposals", "record", fixturePR,
				proposalFile(t, at, ""), "--repo", fixtureSlug)
			require.NoError(t, err)
			require.NoError(t, runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
				"--kind", "mutation", "--patch", writePatch(t, fixtureDiff)))
			require.NoError(t, os.WriteFile(prepared.RepoConfig(fixtureOwner, fixtureProject),
				[]byte(`{"profile":"qa","probe":{"max_per_round":`+tc.capped+`}}`), 0o600))

			printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
			require.NoError(t, err)
			var report struct {
				Honesty []string `json:"honesty"`
			}
			require.NoError(t, json.Unmarshal([]byte(printed), &report))
			disclosed := make([]string, 0)
			for _, line := range report.Honesty {
				if strings.HasPrefix(line, "§5.7.6") {
					disclosed = append(disclosed, line)
				}
			}
			assert.Equal(t, tc.want, disclosed)
		})
	}
}
