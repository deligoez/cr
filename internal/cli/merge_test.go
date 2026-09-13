package cli

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// mergedAnchor is the anchor the fixtures below share, so §6.4.1's key and
// §7.4.1's key are both computable from the test rather than copied out of a
// run.
var mergedAnchor = map[string]any{
	"path":         "internal/api/handler.go",
	"side":         "RIGHT",
	"start_line":   42,
	"line":         44,
	"content_hash": "0123456789abcdef",
}

// aRoleRecord is the §6.1 fields one role writes into its §4.6.2 output file.
// The role is taken as an argument because §6.1.3 rejects a record whose `role`
// is not the role whose file it arrived in, and that binding is half of what
// `cr merge` exists to check.
func aRoleRecord(id, role, class, unit string) map[string]any {
	return map[string]any{
		"id":       id,
		"kind":     "finding",
		"role":     role,
		"class":    class,
		"severity": "high",
		"unit":     unit,
		"anchor":   mergedAnchor,
		"summary":  "The error Decode returns is dropped.",
		"evidence": "The call's second result is assigned to the blank identifier.",
	}
}

// writeFanOut writes one role's §4.6.2 output file under name, which must be
// the fan-out name finding.FanOutFile gives that role: `cr merge` binds a
// file's records to the role its name carries, and refuses a name that carries
// none.
func writeFanOut(t *testing.T, dir, role string, records ...map[string]any) string {
	t.Helper()
	var body bytes.Buffer
	for _, record := range records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		body.Write(line)
		body.WriteByte('\n')
	}
	path := filepath.Join(dir, finding.FanOutFile(role))
	require.NoError(t, os.WriteFile(path, body.Bytes(), 0o600))
	return path
}

// runMergeCLI runs `cr merge` against whatever CR_HOME points at, with the
// repository and pull request `cr merge` requires, and returns what it printed
// and what it refused.
func runMergeCLI(t *testing.T, out string, files ...string) (printed string, err error) {
	t.Helper()
	args := append([]string{"merge"}, files...)
	args = append(args, "-o", out, "--repo", recordSlug, "--pr", recordPR)
	return runCLIPrinting(t, args...)
}

// mergedOut is a path for `-o`, in a directory of its own outside the state
// tree: §2.2 keeps cr's state under `~/.cr`, and this file is the caller's.
func mergedOut(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "merged.ndjson")
}

// decodeMergeResult reads the JSON document `cr merge` printed.
func decodeMergeResult(t *testing.T, printed string) mergeResult {
	t.Helper()
	var reported mergeResult
	require.NoError(t, json.Unmarshal([]byte(printed), &reported))
	return reported
}

// §6.5.1 end to end: two roles' files are merged, §6.4.1 groups the two records
// that sit on one anchored line in one class, §6.4.2 picks the representative,
// and the output is a file `cr record` accepts.
//
// The last clause is the one that matters, and it is asserted by running the
// command rather than by reading the file: §2.3.3 puts head and round on a
// stored record without `omitempty`, so a merge that marshalled its records
// whole would write `"head":""` on every line and `cr record` would refuse all
// of them. A test asserting on the file alone would pass while the pipeline
// was broken.
func TestMergeWritesAFileCRRecordAccepts(t *testing.T) {
	layout := recordedHome(t)
	dir := t.TempDir()
	out := mergedOut(t)

	correctness := writeFanOut(t, dir, "correctness",
		aRoleRecord("f1", "correctness", "unchecked-error", "u1"))
	// f3 is on u2, so it is anchored inside u2's range: `cr record` binds a
	// record's anchor to the unit it names.
	onTheOtherUnit := aRoleRecord("f3", "convention", "missing-test", "u2")
	onTheOtherUnit["anchor"] = aRecord("f3", "u2")["anchor"]
	convention := writeFanOut(t, dir, "convention",
		aRoleRecord("f2", "convention", "unchecked-error", "u1"), onTheOtherUnit)

	printed, err := runMergeCLI(t, out, correctness, convention)
	require.NoError(t, err)
	reported := decodeMergeResult(t, printed)
	assert.Equal(t, out, reported.Output)
	assert.Equal(t, 3, reported.Merged,
		"§6.4.3 retains a suppressed duplicate; it is §6.4.4 and §9.3.6 that drop")
	require.Len(t, reported.Overlaps, 1,
		"§6.4.1 keys on (path, side, line, class), so the two unchecked-error records are one group")
	assert.Equal(t, 1, reported.Overlaps[0].Suppressed)
	assert.ElementsMatch(t, []string{"correctness", "convention"}, reported.Overlaps[0].Roles)

	body, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.NotContains(t, string(body), `"head"`,
		"§2.3.3's pair is the writer's, and state.DecodeStamped rejects it by presence")
	assert.NotContains(t, string(body), `"grade"`,
		"§6.5.1: the grade is computed in memory for the counts and never written")

	recorded, err := runRecord(t, recordPR, out, "--repo", recordSlug)
	require.NoError(t, err, "§6.5.1 hands this file to `cr record`, which has to accept it")
	assert.Contains(t, recorded, finding.StateDuplicate.String(),
		"§6.4.3: `cr record` applies the `duplicate_of` `cr merge` wrote")

	stored, err := state.ReadStamped[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings, recordRound)
	require.NoError(t, err)
	require.Len(t, stored, 3)
	suppressed := 0
	for _, record := range stored {
		assert.Equal(t, recordHead, record.Head, "§2.3.3's stamp is `cr record`'s")
		assert.Equal(t, recordRound, record.Round)
		if record.DuplicateOf != "" {
			suppressed++
			assert.Equal(t, finding.StateDuplicate, record.State)
		}
	}
	assert.Equal(t, 1, suppressed)
}

// §6.4.4: a finding an active waiver covers never reaches the merged file, and
// the drop is counted and attributed to the waiver that made it.
//
// The waiver is written through finding.Waive rather than as bytes, so what is
// measured is the store `cr waivers` writes and §7.4.1's key as WaiverKeyOf
// builds it — a hand-written line could agree with the key this test expects
// and disagree with the one triage produces.
func TestMergeDropsAWaivedFinding(t *testing.T) {
	layout := recordedHome(t)
	waived := finding.Waiver{
		WaiverKey: finding.WaiverKey{
			Path: "internal/api/handler.go", Side: "RIGHT",
			Class: "unchecked-error", ContentHash: recordedHash(t, "u1"),
		},
		Disposition: finding.DispositionNotHere,
	}
	stored, err := finding.Waive(layout, recordOwner, recordRepo, &waived,
		finding.WaiverProvenance{Round: recordRound, PR: recordPRNum, Head: recordHead})
	require.NoError(t, err)

	dir, out := t.TempDir(), mergedOut(t)
	file := writeFanOut(t, dir, "correctness",
		aRoleRecord("f1", "correctness", "unchecked-error", "u1"),
		aRoleRecord("f2", "correctness", "missing-test", "u2"))

	printed, err := runMergeCLI(t, out, file)
	require.NoError(t, err)
	reported := decodeMergeResult(t, printed)

	assert.Equal(t, 1, reported.Merged, "§6.4.4 drops the waived finding")
	assert.Equal(t, 1, reported.Waived.Dropped)
	assert.Equal(t, []string{stored.ID}, reported.Waived.Waivers,
		"§7.4.7 lists the waiver by id, so the drop names the decision the reviewer already wrote")
	assert.Contains(t, reported.Honesty, reported.Waived.Disclosure())

	body, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "unchecked-error",
		"§6.4.4: a dropped finding is not written at all, so no record sits in a state §9.1 lacks")
	assert.Contains(t, string(body), "missing-test")
}

// §9.3.6: a finding the posted index already holds an entry for is dropped at
// merge exactly as §6.4.4 drops a waived one, and the drop names the record the
// author already received.
func TestMergeDropsAnAlreadyPostedFinding(t *testing.T) {
	layout := recordedHome(t)
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, finding.AppendPosted(held, layout, recordOwner, recordRepo, recordPRNum,
		[]finding.PostedEntry{{
			Record: "f9",
			WaiverKey: finding.WaiverKey{
				Path: "internal/api/handler.go", Side: "RIGHT",
				Class: "unchecked-error", ContentHash: recordedHash(t, "u1"),
			},
			Round: recordRound - 1, Head: recordHead,
		}}))
	require.NoError(t, held.Unlock())

	dir, out := t.TempDir(), mergedOut(t)
	file := writeFanOut(t, dir, "correctness",
		aRoleRecord("f1", "correctness", "unchecked-error", "u1"),
		aRoleRecord("f2", "correctness", "missing-test", "u2"))

	printed, err := runMergeCLI(t, out, file)
	require.NoError(t, err)
	reported := decodeMergeResult(t, printed)

	assert.Equal(t, 1, reported.Merged)
	assert.Equal(t, 1, reported.AlreadyPosted.Dropped)
	assert.Equal(t, []string{"f9"}, reported.AlreadyPosted.Posted,
		"§9.3.6's report names the comment the author has, never the finding that was dropped")
	assert.Contains(t, reported.Honesty, reported.AlreadyPosted.Disclosure())

	body, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "unchecked-error")
}

// §6.5.2: a record missing a required field fails with exit code 1 and names
// the line it sits on, counting the blank lines that were skipped to reach it.
func TestMergeRefusesAMissingRequiredFieldNamingTheLine(t *testing.T) {
	recordedHome(t)
	dir, out := t.TempDir(), mergedOut(t)

	incomplete := aRoleRecord("f2", "correctness", "unchecked-error", "u1")
	delete(incomplete, "evidence")
	file := filepath.Join(dir, finding.FanOutFile("correctness"))
	first, err := json.Marshal(aRoleRecord("f1", "correctness", "missing-test", "u1"))
	require.NoError(t, err)
	second, err := json.Marshal(incomplete)
	require.NoError(t, err)
	// A blank line between them, so the number in the refusal is the line
	// the user opens and not the record's position in the file.
	require.NoError(t, os.WriteFile(file,
		[]byte(string(first)+"\n\n"+string(second)+"\n"), 0o600))

	_, err = runMergeCLI(t, out, file)
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.5.2 codes a missing required field 1")
	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, 3, rejected.Line, "§6.5.2 names the offending line")
	assert.Equal(t, "evidence", rejected.Field)

	_, statErr := os.Stat(out)
	assert.True(t, os.IsNotExist(statErr),
		"nothing is written until every input has been read, so a refused merge leaves no half-file")
}

// An input whose name binds its records to no role is refused whole.
//
// §6.1.3 rejects a record whose `role` is not the role whose §4.6.2 output file
// it arrived in, and a name carrying no role leaves nothing to compare against
// — so the file's `role` fields, and with them the axis §6.2 grades on, would
// revert to being taken on the agent's word.
func TestMergeRefusesAFileThatBindsItsRecordsToNoRole(t *testing.T) {
	recordedHome(t)
	dir, out := t.TempDir(), mergedOut(t)
	line, err := json.Marshal(aRoleRecord("f1", "correctness", "unchecked-error", "u1"))
	require.NoError(t, err)
	unattributable := filepath.Join(dir, "findings.ndjson")
	require.NoError(t, os.WriteFile(unattributable, append(line, '\n'), 0o600))

	_, err = runMergeCLI(t, out, unattributable)
	require.Error(t, err)
	var unbound *finding.UnattributableFileError
	require.ErrorAs(t, err, &unbound)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Contains(t, err.Error(), finding.FanOutFile("<role-id>"),
		"the refusal names the shape §4.6.2 gives a fan-out file")
}

// §6.5.1 requires the repository and the pull request, and §11.2 codes a
// malformed invocation 2.
//
// Both are required for a reason that is not bookkeeping: §6.4.2 needs the
// repository's resolved role corpus and §6.4.4 its repository-wide waivers,
// while §7.4.1 scopes `not-here` waivers to the pull request — so a merge that
// ran without either would resurface exactly what the reviewer set aside.
func TestMergeRequiresTheRepositoryAndThePullRequest(t *testing.T) {
	recordedHome(t)
	dir, out := t.TempDir(), mergedOut(t)
	file := writeFanOut(t, dir, "correctness",
		aRoleRecord("f1", "correctness", "unchecked-error", "u1"))

	missing := map[string][]string{
		"--repo": {"merge", file, "-o", out, "--pr", recordPR},
		"--pr":   {"merge", file, "-o", out, "--repo", recordSlug},
		"-o":     {"merge", file, "--repo", recordSlug, "--pr", recordPR},
	}
	for flag, argv := range missing {
		t.Run(flag, func(t *testing.T) {
			_, err := runCLIPrinting(t, argv...)
			require.Error(t, err, "§6.5.1 spells %s into the invocation", flag)
			assert.Equal(t, ExitUsage, exitCodeFor(err), "§11.2 codes a malformed invocation 2")
		})
	}

	_, err := runCLIPrinting(t, "merge", file, "-o", out, "--repo", recordSlug, "--pr", "0")
	require.Error(t, err, "0 is not a pull request")
	assert.Contains(t, err.Error(), "invalid pull request",
		"the flag is refused with parsePR's sentence rather than a second one")
}

// §6.4.2 picks the representative by the grade cr computes and not by the one
// the agent asserted, which it cannot: §6.1.4 refuses the field on the wire.
//
// Both records here are `argued` — neither carries a probe or a citation cr
// resolved — so the three keys fall through to §2.5.5's corpus order, and the
// representative is the earliest role in it. What this holds is that the
// ordering ran at all: a merge that skipped §6.4.2 would leave `duplicate_of`
// on whichever record happened to arrive second.
func TestMergePicksTheRepresentativeByCorpusOrder(t *testing.T) {
	recordedHome(t)
	dir, out := t.TempDir(), mergedOut(t)
	// The lower-ranked role's file is read first, so the representative is
	// not simply the first record in.
	convention := writeFanOut(t, dir, "convention",
		aRoleRecord("f1", "convention", "unchecked-error", "u1"))
	correctness := writeFanOut(t, dir, "correctness",
		aRoleRecord("f2", "correctness", "unchecked-error", "u1"))

	printed, err := runMergeCLI(t, out, convention, correctness)
	require.NoError(t, err)
	reported := decodeMergeResult(t, printed)
	require.Len(t, reported.Overlaps, 1)

	body, err := os.ReadFile(out)
	require.NoError(t, err)
	marked := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		var held struct {
			ID          string `json:"id"`
			DuplicateOf string `json:"duplicate_of"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &held))
		marked[held.ID] = held.DuplicateOf
	}
	representative := reported.Overlaps[0].Representative
	assert.Empty(t, marked[representative], "§6.4.3 leaves the representative unmarked")
	for id, of := range marked {
		if id != representative {
			assert.Equal(t, representative, of,
				"§6.4.3 writes the representative's id into every other record of the group")
		}
	}
}

// tallyNames are the row names of one of §6.5.1's breakdowns, in the order the
// breakdown holds them, so a test can say which vocabulary was counted without
// repeating the counts.
func tallyNames(rows []tally) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names
}

// tallyTotal is the sum of one breakdown's rows.
func tallyTotal(rows []tally) int {
	total := 0
	for _, row := range rows {
		total += row.Count
	}
	return total
}

// countIn is one row's count, and zero for a name the breakdown has no row for
// — which a test then catches through tallyNames rather than through a
// silently missing row.
func countIn(rows []tally, name string) int {
	for _, row := range rows {
		if row.Name == name {
			return row.Count
		}
	}
	return 0
}

// mergeSummaryOf reads the two drop counts §6.5.1 writes back out of the round's
// summary.json.
//
// It decodes the document as its own fields rather than through the types
// `cr merge` stored, so what is asserted is what a reader of the file finds
// there — a shape read back through the writer's own struct would agree with
// the writer by construction.
func mergeSummaryOf(t *testing.T, layout state.Layout) (waived, alreadyPosted *finding.Drops) {
	t.Helper()
	body, err := layout.ReadRound(recordOwner, recordRepo, recordPRNum, recordRound, state.FileSummary)
	require.NoError(t, err)
	var summary struct {
		Waived        *finding.Drops `json:"waived"`
		AlreadyPosted *finding.Drops `json:"already_posted"`
	}
	require.NoError(t, json.Unmarshal(body, &summary))
	return summary.Waived, summary.AlreadyPosted
}

// §6.5.1's two halves in one run: the four breakdowns are reported, and the
// drop counts reach the round's summary.json per §10.3.
//
// The breakdowns are asserted as four cuts of one set rather than as four
// numbers. Each carries its whole vocabulary — the resolved role corpus,
// §1.5's four axes, §6.1's severities and §6.2's grades — and each sums to the
// merged count, which is what makes a reader able to check one against another.
// A breakdown that listed only the values something landed in would read the
// same whether a role found nothing or never ran.
//
// The §9.3.6 count is asserted present at zero rather than left out. §10.1.6
// reads an absent count as "the merge has not run for this round", so a writer
// that wrote the key only when it was non-zero would make every clean round
// indistinguishable from an unmerged one.
func TestMergeReportsFourBreakdownsAndRecordsItsDropsInTheSummary(t *testing.T) {
	layout := recordedHome(t)
	waiver := finding.Waiver{
		WaiverKey: finding.WaiverKey{
			Path: "internal/api/handler.go", Side: "RIGHT",
			Class: "unchecked-error", ContentHash: recordedHash(t, "u1"),
		},
		Disposition: finding.DispositionNotHere,
	}
	stored, err := finding.Waive(layout, recordOwner, recordRepo, &waiver,
		finding.WaiverProvenance{Round: recordRound, PR: recordPRNum, Head: recordHead})
	require.NoError(t, err)

	dir, out := t.TempDir(), mergedOut(t)
	correctness := writeFanOut(t, dir, "correctness",
		aRoleRecord("f1", "correctness", "unchecked-error", "u1"),
		aRoleRecord("f2", "correctness", "missing-test", "u1"))
	convention := writeFanOut(t, dir, "convention",
		aRoleRecord("f3", "convention", "missing-test", "u2"),
		aRoleRecord("f4", "convention", "long-function", "u2"))

	printed, err := runMergeCLI(t, out, correctness, convention)
	require.NoError(t, err)
	reported := decodeMergeResult(t, printed)
	require.Equal(t, 3, reported.Merged, "§6.4.4 drops the waived finding and keeps the other three")

	assert.Equal(t,
		[]string{"convention", "correctness", "intent-coverage", "test-adequacy"},
		tallyNames(reported.Counts.ByRole),
		"§6.5.1's role cut carries §2.5.5's whole corpus, so a role that found nothing is a zero")
	assert.Equal(t, []string{"intent", "correctness", "convention", "test"},
		tallyNames(reported.Counts.ByAxis), "§1.5's four, in that section's order")
	assert.Equal(t, []string{"critical", "high", "medium", "low"},
		tallyNames(reported.Counts.BySeverity), "§6.1's four, highest first")
	assert.Equal(t, []string{"probed", "cited", "argued"},
		tallyNames(reported.Counts.ByGrade), "§6.2's three, strongest first")

	for dimension, rows := range map[string][]tally{
		"role":     reported.Counts.ByRole,
		"axis":     reported.Counts.ByAxis,
		"severity": reported.Counts.BySeverity,
		"grade":    reported.Counts.ByGrade,
	} {
		assert.Equalf(t, reported.Merged, tallyTotal(rows),
			"the %s cut counts the records the file holds, so it sums to the merged count", dimension)
	}
	assert.Equal(t, 1, countIn(reported.Counts.ByRole, "correctness"))
	assert.Equal(t, 2, countIn(reported.Counts.ByRole, "convention"))
	assert.Equal(t, 1, countIn(reported.Counts.ByAxis, "correctness"),
		"§6.1 computes the axis from the role, so the two cuts agree")
	assert.Equal(t, 2, countIn(reported.Counts.ByAxis, "convention"))
	assert.Equal(t, 3, countIn(reported.Counts.BySeverity, "high"))
	assert.Equal(t, 3, countIn(reported.Counts.ByGrade, "argued"),
		"§6.5.1 computes the grade in memory for these counts, and nothing here carries evidence")

	assert.Contains(t, printed, `"by_role"`)
	assert.Contains(t, printed, `"by_grade"`)

	waived, alreadyPosted := mergeSummaryOf(t, layout)
	require.NotNil(t, waived, "§10.3: `cr merge` writes its drop counts into summary.json")
	assert.Equal(t, 1, waived.Dropped)
	assert.Equal(t, []string{stored.ID}, waived.Waivers)
	require.NotNil(t, alreadyPosted,
		"§9.3.6's drop is a key of its own, written at zero as well")
	assert.Equal(t, 0, alreadyPosted.Dropped)
}

// §2.2 measured over the one path `cr merge` writes that §2.2 does not derive.
//
// `-o` is that path, and this is where the reading behind it is measured rather
// than argued. §2.2 governs cr's own state, which is why the round summary this
// same run writes lands under the state root; the `-o` file is the caller's own
// choice of location, which is why pointing it inside the repository under
// review is not refused. So the whole of what this run leaves in the checkout is
// the file the caller named, and nothing cr derived is in there beside it.
//
// The command is run from inside the repository, so a relative path anywhere in
// cr would land here and be seen.
func TestAMergeWritesNothingCrDerivedInsideTheRepositoryUnderReview(t *testing.T) {
	layout := recordedHome(t)
	under := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(under, "handler.go"), []byte("package api\n"), 0o600))
	t.Chdir(under)

	dir := t.TempDir()
	file := writeFanOut(t, dir, "correctness",
		aRoleRecord("f1", "correctness", "unchecked-error", "u1"))
	out := filepath.Join(under, "merged.ndjson")

	_, err := runMergeCLI(t, out, file)
	require.NoError(t, err)

	left := make([]string, 0, 2)
	require.NoError(t, filepath.WalkDir(under, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(under, path)
		if relErr != nil {
			return relErr
		}
		left = append(left, rel)
		return nil
	}))
	assert.ElementsMatch(t, []string{"handler.go", "merged.ndjson"}, left,
		"§2.2: the file `-o` named is the caller's, and nothing cr derived joins it")

	waived, _ := mergeSummaryOf(t, layout)
	require.NotNil(t, waived,
		"§2.2 and §10.3 together: the state this run derived is under the state root")
}
