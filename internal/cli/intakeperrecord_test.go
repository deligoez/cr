package cli

import (
	"encoding/json"
	"path/filepath"
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
// stored records of QA D-V1a-5's round.
func recordThreeStored(t *testing.T) {
	t.Helper()
	second := aGradedRecord("f2")
	second["class"] = "missing-test"
	third := aGradedRecord("f4")
	third["class"] = "dead-code"
	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "stored.ndjson", aGradedRecord("f1"), second, third), "--repo", fixtureSlug)
	require.NoError(t, err)
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
				recordThreeStored(t)
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

// A record carrying no id is counted under §6.4.1's identity: two records at
// one anchored line in one class are one key, and a record one line further
// down is another.
func TestARecordWithoutAnIDIsCountedByItsDedupIdentity(t *testing.T) {
	at := func(line int) *finding.Finding {
		record := &finding.Finding{Class: "long-function"}
		record.Anchor.Path, record.Anchor.Side, record.Anchor.Line = "app.go", git.Right, line
		return record
	}
	first, again, below := at(3), at(3), at(4)
	merge := mergeIntake{Records: []string{}, intakeDrops: intakeDrops{
		Waived:     finding.Drops{Dropped: 1, Waivers: []string{"wr1"}},
		WaivedKeys: droppedKeys([]*finding.Finding{first}, nil),
	}}
	recorded := map[string]recordIntake{"input": {intakeDrops: intakeDrops{
		Waived:     finding.Drops{Dropped: 2, Waivers: []string{"wr1"}},
		WaivedKeys: droppedKeys([]*finding.Finding{again, below}, []*finding.Finding{}),
	}}}

	raised, waived, posted := intakeTotals(&merge, recorded, "merged", nil)

	assert.Equal(t, []string{`"app.go" RIGHT 3 "long-function"`}, merge.WaivedKeys)
	assert.Equal(t, 2, raised)
	assert.Equal(t, finding.Drops{Dropped: 2, Waivers: []string{"wr1"}}, waived)
	assert.Equal(t, finding.PostedDrops{Dropped: 0, Posted: []string{}}, posted)
}

// A `record_intake` entry a v0.2.0 summary kept has no keys, and `cr record`
// writes it back beside its own entry with empty key lists, never null.
func TestAnIntakeEntryKeptWithoutKeysIsWrittenBackWithEmptyLists(t *testing.T) {
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

	_, err = runRecord(t, fixturePR,
		writeRecordFile(t, "stored.ndjson", aGradedRecord("f1")), "--repo", fixtureSlug)
	require.NoError(t, err)

	body, err := layout.ReadRound(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileSummary)
	require.NoError(t, err)
	var document struct {
		RecordIntake map[string]json.RawMessage `json:"record_intake"`
	}
	require.NoError(t, json.Unmarshal(body, &document))
	require.Len(t, document.RecordIntake, 2, "the kept entry and the one this run added")
	assert.JSONEq(t,
		`{"merged":false,"waived":{"dropped":0,"waivers":[]},"waived_keys":[],`+
			`"already_posted":{"dropped":0,"posted":[]},"posted_keys":[]}`,
		string(document.RecordIntake["0123456789abcdef"]))
}
