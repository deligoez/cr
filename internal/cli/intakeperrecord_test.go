package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// postLongFunction appends the posted-index entry a long-function finding on
// the fixture unit's line leaves behind, under §7.4.1's key waiveLongFunction's
// waiver carries, so §9.3.6 drops such a finding as already posted.
func postLongFunction(t *testing.T, layout state.Layout) {
	t.Helper()
	stamped, err := finding.ContextKeyHash(
		[]string{"package app", ""}, []string{"func Retry() { backoff() }"}, []string{})
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, finding.AppendPosted(held, layout, fixtureOwner, fixtureProject, fixturePRNumber,
		[]finding.PostedEntry{{
			Record: "f90",
			WaiverKey: finding.WaiverKey{
				Path: "app.go", Side: "RIGHT", Class: "long-function", ContentHash: stamped,
			},
			Round: 1, Head: "0a1b2c3",
		}}))
	require.NoError(t, held.Unlock())
}

// recordThreeStored records three findings of three classes on the fixture
// unit, which the round holds before the dropped finding arrives — the three
// stored records of QA D-V1a-5's round — and returns the file it recorded.
func recordThreeStored(t *testing.T) string {
	t.Helper()
	second := aGradedRecord("f2")
	second["class"] = "missing-test"
	third := aGradedRecord("f4")
	third["class"] = "dead-code"
	file := writeRecordFile(t, "stored.ndjson", aGradedRecord("f1"), second, third)
	_, err := runRecord(t, fixturePR, file, "--repo", fixtureSlug)
	require.NoError(t, err)
	return file
}

// intakeCommand runs `cr record` or `cr merge` over one role file, the merge
// writing its output into dir.
func intakeCommand(t *testing.T, command, file, dir string) {
	t.Helper()
	var err error
	if command == "record" {
		_, err = runRecord(t, fixturePR, file, "--repo", fixtureSlug)
	} else {
		_, err = runCLIPrinting(t, "merge", file, "-o", filepath.Join(dir, "merged.ndjson"),
			"--repo", fixtureSlug, "--pr", fixturePR)
	}
	require.NoError(t, err, "cr %s", command)
}

// dropKinds are §6.4.4's and §9.3.6's drops: how each is set up on the fixture,
// and the whole of the two sections one dropped finding leaves in summary.json.
func dropKinds(t *testing.T) map[string]struct {
	setUp func(*testing.T, state.Layout)
	drops func(state.Layout, int) (finding.Drops, finding.PostedDrops)
} {
	t.Helper()
	return map[string]struct {
		setUp func(*testing.T, state.Layout)
		drops func(state.Layout, int) (finding.Drops, finding.PostedDrops)
	}{
		"waived": {
			setUp: waiveLongFunction,
			drops: func(layout state.Layout, n int) (finding.Drops, finding.PostedDrops) {
				return finding.Drops{Dropped: n, Waivers: []string{waivedID(t, layout)}},
					finding.PostedDrops{Dropped: 0, Posted: []string{}}
			},
		},
		"already posted": {
			setUp: postLongFunction,
			drops: func(_ state.Layout, n int) (finding.Drops, finding.PostedDrops) {
				return finding.Drops{Dropped: 0, Waivers: []string{}},
					finding.PostedDrops{Dropped: n, Posted: []string{"f90"}}
			},
		},
	}
}

// QA D-V1a-5 through the commands: one dropped finding handed to both
// `cr record` and `cr merge`, in either order and again, is one finding in the
// round summary — raised is the three stored records and it, and the drop is
// counted once.
//
// Before, each command kept its own share of the drops by count alone, so the
// second command's drop of the same record was added to the first: raised 5
// and waived.dropped 2 for one finding.
func TestOneDroppedFindingGivenToRecordAndMergeIsCountedOnce(t *testing.T) {
	for kind, drop := range dropKinds(t) {
		for order, commands := range map[string][]string{
			"record then merge": {"record", "merge"},
			"merge then record": {"merge", "record"},
		} {
			t.Run(kind+"/"+order, func(t *testing.T) {
				layout := gradedHome(t)
				drop.setUp(t, layout)
				storedFile := recordThreeStored(t)
				dir := t.TempDir()
				file := writeFanOut(t, dir, "correctness", longFunction("f3"))

				for range 2 {
					for _, command := range commands {
						intakeCommand(t, command, file, dir)
						raised, waived, posted := intakeOf(t, layout)
						wantWaived, wantPosted := drop.drops(layout, 1)
						assert.Equal(t, 4, raised, "after cr %s", command)
						assert.Equal(t, wantWaived, waived, "after cr %s", command)
						assert.Equal(t, wantPosted, posted, "after cr %s", command)
					}
				}
				assert.Len(t, storedFindings(t, layout), 3, "the dropped finding is never stored")
				// forced_records is §6.3.1's list of the stored records the
				// forcing moved, which is not the intake's to keep.
				for section, named := range summaryStrings(t, layout) {
					for _, id := range []string{"f1", "f2", "f3", "f4"} {
						if section != summaryForcedRecords || id == "f3" {
							assert.False(t, named[id], "summary.json's %s names the record %s", section, id)
						}
					}
				}
				keys := `{"waived_keys":[{"kind":"record_id","id":"f3"}],"posted_keys":[]}`
				if kind == "already posted" {
					keys = `{"waived_keys":[],"posted_keys":[{"kind":"record_id","id":"f3"}]}`
				}
				stored := `{"waived_keys":[],"posted_keys":[]}`
				assert.JSONEq(t, `{"merge_intake":`+keys+`,"record_intake":{`+
					`"`+inputDigest(t, file)+`":`+keys+`,`+
					`"`+inputDigest(t, storedFile)+`":`+stored+`}}`,
					intakeFile(t, layout))
			})
		}
	}
}

// Keyed by record, two distinct dropped findings are still two: one handed to
// `cr record` and another to `cr merge`, whichever runs first.
func TestTwoDistinctDroppedFindingsAreCountedTwice(t *testing.T) {
	for kind, drop := range dropKinds(t) {
		for order, commands := range map[string][]string{
			"record then merge": {"record", "merge"},
			"merge then record": {"merge", "record"},
		} {
			t.Run(kind+"/"+order, func(t *testing.T) {
				layout := gradedHome(t)
				drop.setUp(t, layout)
				recordThreeStored(t)
				files := map[string]string{
					"record": writeRecordFile(t, "direct.ndjson", longFunction("f3")),
					"merge":  writeFanOut(t, t.TempDir(), "correctness", longFunction("f5")),
				}
				dir := t.TempDir()
				for _, command := range commands {
					intakeCommand(t, command, files[command], dir)
				}

				raised, waived, posted := intakeOf(t, layout)
				wantWaived, wantPosted := drop.drops(layout, 2)
				assert.Equal(t, 5, raised, "three stored and two dropped")
				assert.Equal(t, wantWaived, waived)
				assert.Equal(t, wantPosted, posted)
			})
		}
	}
}

// A drop `cr record` applied to an earlier merge's output is not counted once a
// later merge has written another output: that merge read the role files
// again, and here it kept the finding, because the waiver was removed in
// between.
func TestADropOverAnEarlierMergesOutputIsNotCountedAfterTheNextMerge(t *testing.T) {
	layout := gradedHome(t)
	dir := t.TempDir()
	intakeCommand(t, "merge", writeFanOut(t, dir, "correctness", longFunction("f1")), dir)
	waiveLongFunction(t, layout)
	_, err := runRecord(t, fixturePR, filepath.Join(dir, "merged.ndjson"), "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runCLIPrinting(t, "waivers", "remove", waivedID(t, layout), "--repo", fixtureSlug)
	require.NoError(t, err)

	other := aGradedRecord("f2")
	other["class"] = "missing-test"
	intakeCommand(t, "merge", writeFanOut(t, dir, "correctness", longFunction("f1"), other), dir)

	raised, waived, posted := intakeOf(t, layout)
	assert.Equal(t, 2, raised, "f1 and f2, held by the last merge's output")
	assert.Equal(t, finding.Drops{Dropped: 0, Waivers: []string{}}, waived)
	assert.Equal(t, finding.PostedDrops{Dropped: 0, Posted: []string{}}, posted)
}

// A record carrying no id is counted under §6.4.1's identity, and its key says
// so: two records at one anchored line in one class are one key, and a record
// one line further down is another.
func TestARecordWithoutAnIDIsCountedByItsDedupIdentity(t *testing.T) {
	at := func(line int) *finding.Finding {
		record := &finding.Finding{Class: "long-function"}
		record.Anchor.Path, record.Anchor.Side, record.Anchor.Line = "app.go", git.Right, line
		return record
	}
	first, again, below := at(3), at(3), at(4)
	mergeKeys := intakeKeys{WaivedKeys: droppedKeys([]*finding.Finding{first}, nil), PostedKeys: []intakeKey{}}
	in := &roundIntake{
		merge: mergeIntake{Records: []string{}, intakeDrops: intakeDrops{
			Waived: finding.Drops{Dropped: 1, Waivers: []string{"wr1"}},
		}},
		mergeKeys: &mergeKeys,
		recorded: map[string]recordIntake{"input": {intakeDrops: intakeDrops{
			Waived: finding.Drops{Dropped: 2, Waivers: []string{"wr1"}},
		}}},
		recordKeys: map[string]intakeKeys{"input": {
			WaivedKeys: droppedKeys([]*finding.Finding{again, below}, []*finding.Finding{}),
		}},
		mergedHash: "merged",
	}

	raised, waived, posted := intakeTotals(in, nil)

	identity := finding.DedupKey{Path: "app.go", Side: git.Right, Line: 3, Class: "long-function"}
	assert.Equal(t, []intakeKey{{Kind: intakeKindIdentity, Identity: &identity}}, mergeKeys.WaivedKeys)
	spelled, err := json.Marshal(mergeKeys.WaivedKeys[0])
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"kind":"identity","identity":{"path":"app.go","side":"RIGHT","line":3,"class":"long-function"}}`,
		string(spelled))
	assert.Equal(t, 2, raised)
	assert.Equal(t, finding.Drops{Dropped: 2, Waivers: []string{"wr1"}}, waived)
	assert.Equal(t, finding.PostedDrops{Dropped: 0, Posted: []string{}}, posted)
}

// A `record_intake` entry a v0.2.0 summary kept is written back as it was,
// beside the entry this run added, and intake.json holds keys only for that
// run's file.
func TestAnIntakeEntryKeptWithoutKeysIsWrittenBackAsItWas(t *testing.T) {
	layout := gradedHome(t)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, writeSummary(held, 2, ownerRecord, []summaryCount{
		{key: summaryDeduplicated, value: 0},
		{key: summarySuppressedByThread, value: 0},
		{key: summaryRecordedAt, value: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		{key: summaryRecordIntake, value: map[string]any{"0123456789abcdef": map[string]any{
			"merged":         false,
			"waived":         map[string]any{"dropped": 0, "waivers": []string{}},
			"already_posted": map[string]any{"dropped": 0, "posted": []string{}},
		}}},
	}))
	require.NoError(t, held.Unlock())

	file := writeRecordFile(t, "stored.ndjson", aGradedRecord("f1"))
	_, err = runRecord(t, fixturePR, file, "--repo", fixtureSlug)
	require.NoError(t, err)

	body, err := layout.ReadRound(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileSummary)
	require.NoError(t, err)
	var document struct {
		RecordIntake map[string]json.RawMessage `json:"record_intake"`
	}
	require.NoError(t, json.Unmarshal(body, &document))
	require.Len(t, document.RecordIntake, 2, "the kept entry and the one this run added")
	assert.JSONEq(t,
		`{"merged":false,"waived":{"dropped":0,"waivers":[]},"already_posted":{"dropped":0,"posted":[]}}`,
		string(document.RecordIntake["0123456789abcdef"]))
	assert.JSONEq(t,
		`{"record_intake":{"`+inputDigest(t, file)+`":{"waived_keys":[],"posted_keys":[]}}}`,
		intakeFile(t, layout))
}

// inputDigest is the digest `cr record` keys its share of a file by.
func inputDigest(t *testing.T, file string) string {
	t.Helper()
	body, err := os.ReadFile(file)
	require.NoError(t, err)
	digest, err := mergedDigest(body)
	require.NoError(t, err)
	return digest
}

// intakeFile is the round's intake.json, whole.
func intakeFile(t *testing.T, layout state.Layout) string {
	t.Helper()
	body, err := layout.ReadRound(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileIntake)
	require.NoError(t, err)
	return string(body)
}

// summaryStrings is every string each section of summary.json holds, as a key
// or as a value, at any depth, by section.
func summaryStrings(t *testing.T, layout state.Layout) map[string]map[string]bool {
	t.Helper()
	body, err := layout.ReadRound(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileSummary)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(body, &document))
	sections := make(map[string]map[string]bool, len(document))
	for section, value := range document {
		held := make(map[string]bool)
		var walk func(any)
		walk = func(value any) {
			switch value := value.(type) {
			case string:
				held[value] = true
			case []any:
				for _, item := range value {
					walk(item)
				}
			case map[string]any:
				for key, item := range value {
					held[key] = true
					walk(item)
				}
			}
		}
		walk(value)
		sections[section] = held
	}
	return sections
}

// A round a v0.2.0 intake left — counts in summary.json, no intake.json — keeps
// the counts v0.2.0 gave it once a v0.2.1 command runs: the share with no keys
// is added by count, so the finding both commands dropped is counted twice, as
// v0.2.0 counted it, and never read as no drop at all.
func TestARoundWithoutIntakeKeysKeepsTheCountsV020Gave(t *testing.T) {
	for order, commands := range map[string][]string{
		"record then merge": {"record", "merge"},
		"merge then record": {"merge", "record"},
	} {
		t.Run(order, func(t *testing.T) {
			layout := gradedHome(t)
			waiveLongFunction(t, layout)
			recordThreeStored(t)
			dir := t.TempDir()
			file := writeFanOut(t, dir, "correctness", longFunction("f3"))
			intakeCommand(t, commands[0], file, dir)
			require.NoError(t, os.Remove(
				layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileIntake)))

			intakeCommand(t, commands[1], file, dir)

			raised, waived, posted := intakeOf(t, layout)
			assert.Equal(t, 5, raised, "three stored, and the one drop counted by each command")
			assert.Equal(t, finding.Drops{Dropped: 2, Waivers: []string{waivedID(t, layout)}}, waived)
			assert.Equal(t, finding.PostedDrops{Dropped: 0, Posted: []string{}}, posted)
		})
	}
}

// An intake.json whose summary.json is gone is read as keys beside a summary
// holding nothing: the next command runs, and counts the round by those keys.
func TestAnIntakeFileWithoutASummaryIsCountedByItsKeys(t *testing.T) {
	for order, commands := range map[string][]string{
		"record then merge": {"record", "merge"},
		"merge then record": {"merge", "record"},
	} {
		t.Run(order, func(t *testing.T) {
			layout := gradedHome(t)
			waiveLongFunction(t, layout)
			recordThreeStored(t)
			dir := t.TempDir()
			file := writeFanOut(t, dir, "correctness", longFunction("f3"))
			intakeCommand(t, commands[0], file, dir)
			require.NoError(t, os.Remove(
				layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileSummary)))

			intakeCommand(t, commands[1], file, dir)

			raised, waived, posted := intakeOf(t, layout)
			assert.Equal(t, 4, raised)
			assert.Equal(t, finding.Drops{Dropped: 1, Waivers: []string{waivedID(t, layout)}}, waived)
			assert.Equal(t, finding.PostedDrops{Dropped: 0, Posted: []string{}}, posted)
		})
	}
}

// A file whose summary entry is gone and whose keys intake.json still holds is
// still counted, by those keys, when another file is recorded after it. Which
// waiver dropped it went with the summary, so the drop is counted and names
// none.
func TestAnIntakeEntryWithoutASummaryEntryIsCountedByItsKeys(t *testing.T) {
	layout := gradedHome(t)
	waiveLongFunction(t, layout)
	recordThreeStored(t)
	dir := t.TempDir()
	intakeCommand(t, "record", writeFanOut(t, dir, "correctness", longFunction("f3")), dir)
	require.NoError(t, os.Remove(
		layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileSummary)))

	other := aGradedRecord("f5")
	other["class"] = "naming"
	intakeCommand(t, "record", writeRecordFile(t, "other.ndjson", other), dir)

	raised, waived, posted := intakeOf(t, layout)
	assert.Equal(t, 5, raised, "f1, f2, f4 and f5 stored, and f3 dropped")
	assert.Equal(t, finding.Drops{Dropped: 1, Waivers: []string{}}, waived)
	assert.Equal(t, finding.PostedDrops{Dropped: 0, Posted: []string{}}, posted)
}

// Nothing a colleague reads is built from intake.json: no production file of
// the packages that render and post a review names it.
func TestNothingColleagueFacingReadsTheIntakeKeys(t *testing.T) {
	for _, dir := range []string{filepath.Join("..", "render"), filepath.Join("..", "post")} {
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.NotEmpty(t, entries, dir)
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			require.NoError(t, err)
			for _, name := range []string{state.FileIntake, "FileIntake"} {
				assert.NotContains(t, string(body), name, entry.Name())
			}
		}
	}
}

// A share intake.json holds no keys for is added by count, and its already
// posted drops count as well as its waived ones: toward already posted, and
// toward raised beside the waived.
func TestAShareKeptWithoutKeysAddsItsAlreadyPostedDropsByCount(t *testing.T) {
	in := &roundIntake{
		merge: mergeIntake{Records: []string{}},
		recorded: map[string]recordIntake{"0123456789abcdef": {intakeDrops: intakeDrops{
			Waived:        finding.Drops{Dropped: 1, Waivers: []string{"wr1"}},
			AlreadyPosted: finding.PostedDrops{Dropped: 2, Posted: []string{"f7"}},
		}}},
		recordKeys: map[string]intakeKeys{},
		mergedHash: "merged",
	}

	raised, waived, posted := intakeTotals(in, nil)

	assert.Equal(t, 3, raised, "one waived and two already posted, each raised before it was dropped")
	assert.Equal(t, finding.Drops{Dropped: 1, Waivers: []string{"wr1"}}, waived)
	assert.Equal(t, finding.PostedDrops{Dropped: 2, Posted: []string{"f7"}}, posted)
}
