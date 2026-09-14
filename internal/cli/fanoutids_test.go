package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// fanoutOf runs `cr review` over statusHome's pull request and decodes what it
// printed.
func fanoutOf(t *testing.T, args ...string) review.Fanout {
	t.Helper()
	printed, err := runCLIPrinting(t, append([]string{"review", fixturePR, "--repo", fixtureSlug}, args...)...)
	require.NoError(t, err)
	var fan review.Fanout
	require.NoError(t, json.Unmarshal([]byte(printed), &fan))
	return fan
}

// idRun is a prompt's run of record ids as numbers, first and last.
type idRun struct {
	first, last int
}

// idRunOf reads the run a prompt's JSON names, and holds its text to naming the
// same run in the sentence cr writes, compared as a whole line.
func idRunOf(t *testing.T, prompt *review.Prompt, round int) idRun {
	t.Helper()
	first, ok := finding.IDSuffix(prompt.FirstID)
	require.Truef(t, ok, "%s on %s names a first id: %q", prompt.Role, prompt.Unit, prompt.FirstID)
	last, ok := finding.IDSuffix(prompt.LastID)
	require.Truef(t, ok, "%s on %s names a last id: %q", prompt.Role, prompt.Unit, prompt.LastID)
	sentence := fmt.Sprintf("Give the records you write here the ids %s through %s, in order from %s. "+
		"No other prompt of round %d is given any of them, and none is held by a stored record. "+
		"An id outside them may be another prompt's, and cr merge refuses an id two records carry, "+
		"with exit code 1 (§6.1).", prompt.FirstID, prompt.LastID, prompt.FirstID, round)
	assert.Contains(t, strings.Split(prompt.Text, "\n"), sentence)
	return idRun{first: first, last: last}
}

// assertNoTwoRunsOverlap holds every prompt's run to sharing no id with any
// other prompt's.
func assertNoTwoRunsOverlap(t *testing.T, runs map[string]idRun) {
	t.Helper()
	for a, one := range runs {
		for b, other := range runs {
			if a != b {
				assert.Falsef(t, one.first <= other.last && other.first <= one.last,
					"%s holds f%d..f%d and %s holds f%d..f%d", a, one.first, one.last, b, other.first, other.last)
			}
		}
	}
}

// writeAtFirstIDs writes one record per prompt, at the first id its prompt
// names, to the output path it names, and returns the files written.
func writeAtFirstIDs(t *testing.T, prompts []review.Prompt) []string {
	t.Helper()
	files := make([]string, 0, len(prompts))
	for i := range prompts {
		prompt := &prompts[i]
		line, err := json.Marshal(map[string]any{
			"id": prompt.FirstID, "kind": "question", "role": prompt.Role, "class": "unchecked-error",
			"severity": "low", "unit": prompt.Unit,
			"anchor":  map[string]any{"path": "lib.go", "side": "RIGHT", "start_line": 1, "line": 1},
			"summary": "Is the error Load returns dropped?", "evidence": "Nothing reads the result.",
		})
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(prompt.Output), 0o750))
		require.NoError(t, os.WriteFile(prompt.Output, append(line, '\n'), 0o600))
		files = append(files, prompt.Output)
	}
	return files
}

// mergeFanOut runs `cr merge` over the files and returns the output path and
// how many records it merged.
func mergeFanOut(t *testing.T, files []string) (out string, merged int) {
	t.Helper()
	out = mergedOut(t)
	args := append(append([]string{"merge"}, files...), "-o", out, "--repo", fixtureSlug, "--pr", fixturePR)
	printed, err := runCLIPrinting(t, args...)
	require.NoError(t, err, "cr merge accepts one record per prompt at the first id each prompt names")
	return out, decodeMergeResult(t, printed).Merged
}

// runsOf reads every prompt's run, keyed by role and unit.
func runsOf(t *testing.T, fan *review.Fanout) map[string]idRun {
	t.Helper()
	runs := make(map[string]idRun, len(fan.Prompts))
	for i := range fan.Prompts {
		runs[fan.Prompts[i].Role+"/"+fan.Prompts[i].Unit] = idRunOf(t, &fan.Prompts[i], fan.Round)
	}
	return runs
}

// §4.6.2 and §6.1 end to end, release QA's fan-out-f1 collision: every prompt
// `cr review` emits over three roles and two units names a run of ids no other
// prompt of the round is given, so one record per prompt at the first id each
// names merges. After `cr record` stores them, the same round's prompts keep
// their blocks and move their first id past the stored one, an `--axis` pass
// hands its prompts the blocks the full fan-out does, and a round opened after
// the stored one starts every run past every id the store holds.
func TestEveryReviewPromptNamesARunOfIDsNoOtherPromptOfTheRoundIsGiven(t *testing.T) {
	statusHome(t)
	layout, err := state.Default()
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	// statusHome files a pass cell on u1 from every role, which §4.5.6 has
	// `cr record` refuse a record against; this round's roles have not
	// reported yet.
	require.NoError(t, held.Write(state.FileCoverage, []byte{}))
	require.NoError(t, held.Unlock())

	first := fanoutOf(t)
	require.Len(t, first.Prompts, 6, "three active roles over two units")
	firstRuns := runsOf(t, &first)
	assertNoTwoRunsOverlap(t, firstRuns)
	out, merged := mergeFanOut(t, writeAtFirstIDs(t, first.Prompts))
	assert.Equal(t, 6, merged)
	_, err = runCLIPrinting(t, "record", fixturePR, out, "--repo", fixtureSlug)
	require.NoError(t, err)

	again := fanoutOf(t)
	require.Len(t, again.Prompts, 6)
	againRuns := runsOf(t, &again)
	for key, run := range firstRuns {
		assert.Equalf(t, idRun{first: run.first + 1, last: run.last}, againRuns[key],
			"%s keeps its block and skips the id its stored record holds", key)
	}
	assertNoTwoRunsOverlap(t, againRuns)
	_, merged = mergeFanOut(t, writeAtFirstIDs(t, again.Prompts))
	assert.Equal(t, 6, merged)

	intent := fanoutOf(t, "--axis", "intent")
	require.NotEmpty(t, intent.Prompts)
	for i := range intent.Prompts {
		prompt := &intent.Prompts[i]
		key := prompt.Role + "/" + prompt.Unit
		assert.Equalf(t, againRuns[key], idRunOf(t, prompt, intent.Round), "%s is given the full fan-out's run", key)
	}

	openRound(t, layout, 2)
	next := fanoutOf(t)
	require.Equal(t, 2, next.Round)
	require.Len(t, next.Prompts, 6)
	stored, err := state.ReadRecords[finding.Finding](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 6)
	highest := 0
	for i := range stored {
		n, ok := finding.IDSuffix(stored[i].ID)
		require.True(t, ok)
		highest = max(highest, n)
	}
	nextRuns := runsOf(t, &next)
	for key, run := range nextRuns {
		assert.Greaterf(t, run.first, highest, "%s starts past every id the store holds", key)
	}
	assertNoTwoRunsOverlap(t, nextRuns)
	_, merged = mergeFanOut(t, writeAtFirstIDs(t, next.Prompts))
	assert.Equal(t, 6, merged)
}

// openRound moves statusHome's pull request to round, at the same head, with the
// same units and a mapping stamp: the least a later round needs for `cr review`
// to emit over it, leaving findings.ndjson as the earlier round stored it.
func openRound(t *testing.T, layout state.Layout, round int) {
	t.Helper()
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	units, err := os.ReadFile(layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileUnits))
	require.NoError(t, err)
	stamp := fmt.Sprintf(`"round":%d}`, meta.Round)
	require.Equal(t, 2, strings.Count(string(units), stamp))
	restamped := strings.ReplaceAll(string(units), stamp, fmt.Sprintf(`"round":%d}`, round))
	meta.Round, meta.MappingRound = round, round
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Write(state.FileUnits, []byte(restamped)))
	require.NoError(t, held.Unlock())
	assert.False(t, slices.Contains(strings.Split(restamped, "\n"), stamp))
}
