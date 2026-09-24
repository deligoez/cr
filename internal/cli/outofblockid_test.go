package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// outsideBlock is the refusal of a new record whose id lies outside the block
// its prompt was given, with the free id of that block the refusal names.
func outsideBlock(id string, round int, prompt *review.Prompt, free string) string {
	return `"` + id + `" lies outside the block of ids cr review gave its prompt; §4.6.2 gives every prompt of round ` +
		strconv.Itoa(round) + " a block no other prompt is given, so an id outside it may be another prompt's; " +
		"the next free id in " + prompt.FirstID + ".." + prompt.LastID + ", the block cr review gave the " +
		prompt.Role + " prompt on unit " + prompt.Unit + ", is " + free
}

// clearStatusCoverage empties statusHome's coverage cells, whose pass on u1
// from every role §4.5.6 has `cr record` refuse a record against.
func clearStatusCoverage(t *testing.T) state.Layout {
	t.Helper()
	statusHome(t)
	layout, err := state.Default()
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileCoverage, []byte{}))
	require.NoError(t, held.Unlock())
	return layout
}

// A role that ignored its prompt: one of the six prompts `cr review` emits over
// statusHome is answered at an id two past its block, inside the block of the
// prompt placed after it. `cr merge` and `cr record` both refuse it with exit
// code 1, naming the file's line, the record, its role and unit and that
// prompt's block, and write nothing; the same record at the first id of its
// block merges and records.
func TestANewRecordOutsideItsPromptsBlockIsRefusedByMergeAndRecord(t *testing.T) {
	layout := clearStatusCoverage(t)
	fan := fanoutOf(t)
	require.Len(t, fan.Prompts, 6, "three active roles over two units")
	strayed := slices.Clone(fan.Prompts)
	last, numbered := finding.IDSuffix(strayed[0].LastID)
	require.True(t, numbered)
	strayed[0].FirstID = finding.IDOf(last + 2)
	files := writeAtFirstIDs(t, strayed)
	refusal := outsideBlock(finding.IDOf(last+2), fan.Round, &fan.Prompts[0], fan.Prompts[0].FirstID)

	out := mergedOut(t)
	_, err := runCLIPrinting(t, append(append([]string{"merge"}, files...),
		"-o", out, "--repo", fixtureSlug, "--pr", fixturePR)...)
	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, finding.RejectedRecordError{File: files[0], Line: 1, Field: "id", Problem: refusal}, *rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.NoFileExists(t, out)

	_, err = runCLIPrinting(t, "record", fixturePR, files[0], "--repo", fixtureSlug)
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, finding.RejectedRecordError{File: files[0], Line: 1, Field: "id", Problem: refusal}, *rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	stored, err := state.ReadRecords[finding.Finding](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	assert.Empty(t, stored, "a refused record file stores nothing")

	merged, count := mergeFanOut(t, writeAtFirstIDs(t, fan.Prompts))
	assert.Equal(t, 6, count)
	_, err = runCLIPrinting(t, "record", fixturePR, merged, "--repo", fixtureSlug)
	require.NoError(t, err, "every record at an id inside its prompt's block is recorded")
}

// promptFor is the prompt the fan-out gave role on unit.
func promptFor(t *testing.T, fan *review.Fanout, role, unit string) *review.Prompt {
	t.Helper()
	for i := range fan.Prompts {
		if fan.Prompts[i].Role == role && fan.Prompts[i].Unit == unit {
			return &fan.Prompts[i]
		}
	}
	require.Failf(t, "no prompt", "%s on %s", role, unit)
	return nil
}

// firstRecordOf decodes the first line of a role's output file.
func firstRecordOf(t *testing.T, file string) finding.Finding {
	t.Helper()
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	var record finding.Finding
	require.NoError(t, json.Unmarshal(body, &record))
	return record
}

// A record that is not new is never measured against a block: the same role
// file recorded again in its own round, and again once the next round's blocks
// start past it, is refused as held by the stored record — the refusal §6.1
// owes it — and not as an id outside its prompt's block, though in the later
// round it lies outside every block that round gives. The hint is the free id
// of the block the current round gives its prompt.
func TestARecordThatIsNotNewIsRefusedAsHeldAndNotAsOutsideItsBlock(t *testing.T) {
	layout := clearStatusCoverage(t)
	first := fanoutOf(t)
	files := writeAtFirstIDs(t, first.Prompts)
	merged, _ := mergeFanOut(t, files)
	_, err := runCLIPrinting(t, "record", fixturePR, merged, "--repo", fixtureSlug)
	require.NoError(t, err)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	record := firstRecordOf(t, files[0])
	held := func(prompt *review.Prompt, free string) string {
		return `"` + record.ID + `" is already held by the record stored for this pull request in round 1 at head ` +
			meta.Head + "; §6.1 makes a record id stable for the life of the pull request, so give this record an id " +
			"no stored record holds; the next free id in " + finding.IDOf(blockStart(t, prompt)) + ".." + prompt.LastID +
			", the block cr review gave the " + prompt.Role + " prompt on unit " + prompt.Unit + ", is " + free
	}

	again := fanoutOf(t)
	_, err = runCLIPrinting(t, "record", fixturePR, files[0], "--repo", fixtureSlug)
	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	prompt := promptFor(t, &again, record.Role, record.Unit)
	assert.Equal(t, finding.RejectedRecordError{
		File: files[0], Line: 1, Field: "id", Problem: held(prompt, prompt.FirstID),
	}, *rejected, "re-recorded in its own round")

	openRound(t, layout, 2)
	next := fanoutOf(t)
	prompt = promptFor(t, &next, record.Role, record.Unit)
	n, _ := finding.IDSuffix(record.ID)
	start := blockStart(t, prompt)
	require.Less(t, n, start, "the stored id lies outside the block round 2 gives its prompt")
	_, err = runCLIPrinting(t, "record", fixturePR, files[0], "--repo", fixtureSlug)
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, finding.RejectedRecordError{
		File: files[0], Line: 1, Field: "id", Problem: held(prompt, prompt.FirstID),
	}, *rejected, "carried into the next round")
	assert.Equal(t, ExitValidation, exitCodeFor(err))
}

// blockStart is the first id of a prompt's hundred, stored or not.
func blockStart(t *testing.T, prompt *review.Prompt) int {
	t.Helper()
	last, numbered := finding.IDSuffix(prompt.LastID)
	require.True(t, numbered)
	return last - 99
}

// ensureRecordFanOut gives recordedHome's round the fan-out directories
// `cr review` creates before it emits a prompt.
func ensureRecordFanOut(t *testing.T, layout state.Layout) {
	t.Helper()
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.EnsureFanOut(recordRound, []string{"u1", "u2"}))
	require.NoError(t, held.Unlock())
}

// A round no `cr review` has emitted prompts for gave no prompt a block, so a
// record there is held to none: f1 from the correctness role on u1 is recorded,
// though the block the round's fan-out would give that prompt is f201..f300.
// Once the round has its fan-out, f2 from the same seat is refused, naming that
// block and its first free id.
func TestARoundCrReviewHasNotEmittedHoldsNoRecordToABlock(t *testing.T) {
	layout := recordedHome(t)
	_, err := runRecord(t, recordPR,
		writeRecordFile(t, "unreviewed.ndjson", aRoleRecord("f1", "correctness", "unchecked-error", "u1")),
		"--repo", recordSlug)
	require.NoError(t, err, "no prompt of the round was given a block")

	ensureRecordFanOut(t, layout)
	file := writeRecordFile(t, "reviewed.ndjson", aRoleRecord("f2", "correctness", "missing-test", "u1"))
	_, err = runRecord(t, recordPR, file, "--repo", recordSlug)
	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, finding.RejectedRecordError{
		File: file, Line: 1, Field: "id",
		Problem: outsideBlock("f2", recordRound,
			&review.Prompt{Role: "correctness", Unit: "u1", FirstID: "f201", LastID: "f300"}, "f201"),
	}, *rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
}

// The block's last id is one of its own: with f201..f299 carried by the input,
// the refusal of a record outside the block names f300 as the free id rather
// than calling the block full.
//
// gremlins found this. Walking the block up to but not past its last id left
// f300 unread and sent the role to a block it could still write in as though
// none were left.
func TestTheLastIDOfABlockIsNamedWhenItIsTheOnlyOneFree(t *testing.T) {
	layout := recordedHome(t)
	ensureRecordFanOut(t, layout)
	records := []map[string]any{aRoleRecord("f2", "correctness", "missing-test", "u1")}
	for n := 201; n < 300; n++ {
		records = append(records, aRoleRecord(finding.IDOf(n), "correctness", "unchecked-error", "u1"))
	}
	file := writeRecordFile(t, "crowded.ndjson", records...)

	_, err := runRecord(t, recordPR, file, "--repo", recordSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, finding.RejectedRecordError{
		File: file, Line: 1, Field: "id",
		Problem: outsideBlock("f2", recordRound,
			&review.Prompt{Role: "correctness", Unit: "u1", FirstID: "f201", LastID: "f300"}, "f300"),
	}, *rejected)
}

// A fan-out directory cr cannot inspect is a file failure, coded 3, rather than
// a round read as never reviewed.
func TestAFanOutDirectoryCrCannotInspectIsAFileFailure(t *testing.T) {
	layout := recordedHome(t)
	ensureRecordFanOut(t, layout)
	unit := layout.FanOutDir(recordOwner, recordRepo, recordPRNum, recordRound, "u1")
	round := filepath.Dir(unit)
	require.NoError(t, os.Chmod(round, 0o000))
	t.Cleanup(func() { _ = os.Chmod(round, 0o750) })

	_, err := runRecord(t, recordPR,
		writeRecordFile(t, "sealed.ndjson", aRoleRecord("f1", "correctness", "unchecked-error", "u1")),
		"--repo", recordSlug)

	var failed *state.FileError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, "cannot read "+unit+": stat "+unit+": permission denied", err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err))
}
