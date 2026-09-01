package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// gradedHome puts a state root behind CR_HOME holding one briefed pull request
// whose head is a real commit of a real checkout, so §6.2.3 can resolve a
// citation against it.
//
// The units carry §3.4.6's path and hunk ranges as well as the id, which
// recordedHome's do not: §6.2's `cited` row turns on whether a location lies
// inside the record's own unit, and a unit line holding no ranges would put
// every citation outside every unit and grade half the table by accident.
//
// The unit covers the third line of the fixture's app.go, which is the line the
// head branch changed, and the citations below point at the first — inside the
// same file and outside the unit's hunk.
func gradedHome(t *testing.T) state.Layout {
	t.Helper()
	fixture := fixtureRepository(t)
	head := strings.TrimSpace(mustGit(t, fixture, "rev-parse", fixtureHeadBranch))

	restore := repoDir
	repoDir = func() (string, error) { return fixture, nil }
	t.Cleanup(func() { repoDir = restore })

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 2, Head: head,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"app.go","side":"RIGHT",`+
			`"hunk_ranges":[{"start":3,"end":3}],"head":"`+head+`","round":2}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// aGradedRecord is a valid §6.1 record on that unit, whose prose asserts a
// great deal more than the record carries behind it.
func aGradedRecord(id string) map[string]any {
	return map[string]any{
		"id": id, "kind": "finding", "role": "correctness", "class": "unchecked-error",
		"severity": "medium", "unit": "u1", "claim": fixtureIssue + "#c1",
		"anchor": map[string]any{
			"path": "app.go", "side": "RIGHT", "start_line": 3, "line": 3,
			"content_hash": "0123456789abcdef",
		},
		"summary":  "Retry drops the error backoff returns.",
		"evidence": "Proven by experiment; the gap is confirmed and verified.",
	}
}

// gradesOf reads the grade cr stamped onto each stored record, by record id.
func gradesOf(t *testing.T, layout state.Layout) map[string]finding.Grade {
	t.Helper()
	stored, err := state.ReadRecords[finding.Finding](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	graded := make(map[string]finding.Grade, len(stored))
	for i := range stored {
		graded[stored[i].ID] = stored[i].Grade
	}
	return graded
}

// §6.2 through the command: `cr record` computes the grade and writes it, and
// the agent supplied none.
//
// Three records go in together, differing only in their citations, and come
// back carrying two different grades. That is the whole of §6.2.1's "computed
// by cr from the record": the field is refused on the wire by §6.1.4, so a
// grade on disk is cr's, and the three records were written by one fixture with
// one evidence sentence — so what separated them is the evidence cr resolved
// and not the prose the agent wrote.
//
// The citation that buys `cited` points at line 1 of app.go, which the head
// holds and the unit's single hunk does not cover. The one that does not points
// at line 3, which is the unit's own hunk: §6.2's row asks for a location the
// human can check the summary against, and the code the record is already about
// is not one.
func TestRecordComputesAndStampsTheGradeSection62Names(t *testing.T) {
	layout := gradedHome(t)

	outside := aGradedRecord("f1")
	outside["citations"] = []map[string]any{{"path": "app.go", "line": 1}}
	within := aGradedRecord("f2")
	within["citations"] = []map[string]any{{"path": "app.go", "line": 3}}
	file := writeRecordFile(t, "merged.ndjson", outside, within, aGradedRecord("f3"))

	_, err := runRecord(t, "7", file, "--repo", fixtureSlug)
	require.NoError(t, err)

	graded := gradesOf(t, layout)
	assert.Equal(t, finding.GradeCited, graded["f1"],
		"§6.2: an entry cr resolved that lies outside the record's own unit")
	assert.Equal(t, finding.GradeArgued, graded["f2"],
		"§6.2: a citation inside the record's own unit buys nothing")
	assert.Equal(t, finding.GradeArgued, graded["f3"],
		"§6.2's third row, whatever the evidence sentence claims")
}

// §4.4.2 through the command: a test-adequacy record cites the same location a
// correctness record is graded `cited` for, and grades `argued`.
//
// The two records differ in their `role` and in nothing else that grading
// reads, and the axis is not on either of them — §6.1.4 refuses the field on
// the wire, so what separates them is the axis cr computed from §2.5.5's
// corpus. Both are asserted on the stored record, because the stamped axis is
// the other half of the claim: a command that graded correctly while writing no
// axis would leave §8.1 and §10 reading a field nobody filled.
//
// §4.4.2 is what the difference is for. A role reading the suite can always
// find a test file to point at, and pointing at one is not evidence that the
// behaviour is untested — only an experiment is, so a test-adequacy record is
// `probed` or `argued` and §6.3 asks the second as a question.
func TestATestAdequacyRecordIsNotGradedCitedByItsCitations(t *testing.T) {
	layout := gradedHome(t)

	adequacy := aGradedRecord("f1")
	adequacy["role"] = "test-adequacy"
	adequacy["citations"] = []map[string]any{{"path": "app.go", "line": 1}}
	correctness := aGradedRecord("f2")
	correctness["citations"] = []map[string]any{{"path": "app.go", "line": 1}}
	file := writeRecordFile(t, "merged.ndjson", adequacy, correctness)

	_, err := runRecord(t, "7", file, "--repo", fixtureSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 2)

	assert.Equal(t, "test", stored[0].Axis, "§6.1: cr writes the axis of the record's role")
	assert.Equal(t, finding.GradeArgued, stored[0].Grade,
		"§4.4.2: a citation grades a test-adequacy record nothing")
	assert.Equal(t, "correctness", stored[1].Axis)
	assert.Equal(t, finding.GradeCited, stored[1].Grade,
		"and the same citation on another axis is exactly what §6.2's cited row asks for")
}

// §6.2.3 through the command: every citation is resolved against the head, the
// hash cr computed is stored, and an entry the head cannot open refuses the
// file with exit code 1 naming the line and the entry.
//
// The refusal is put on the second of two records so that the first would
// already have been stored by a command that wrote as it validated — the same
// shape §6.1.3's refusal is tested in, because §6.2.3 rejects for the same
// reason and the agent is about to hand the whole file in again.
func TestRecordResolvesEveryCitationAgainstTheHead(t *testing.T) {
	layout := gradedHome(t)

	sound := aGradedRecord("f1")
	sound["citations"] = []map[string]any{{"path": "app.go", "line": 1}}
	_, err := runRecord(t, "7", writeRecordFile(t, "sound.ndjson", sound), "--repo", fixtureSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Len(t, stored[0].Citations, 1)
	assert.NotEmpty(t, stored[0].Citations[0].ContentHash,
		"§6.2.3: cr computes and stores the hash of the line the citation names")

	unopenable := aGradedRecord("f3")
	unopenable["citations"] = []map[string]any{{"path": "app.go", "line": 900}}
	refused := writeRecordFile(t, "refused.ndjson", aGradedRecord("f2"), unopenable)

	_, err = runRecord(t, "7", refused, "--repo", fixtureSlug)
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.2.3 rejects with exit code 1")

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected, "the command raises no rejection of its own")
	assert.Equal(t, refused, rejected.File)
	assert.Equal(t, 2, rejected.Line, "the one-based line the record sits on")
	assert.Equal(t, "citations[0].line", rejected.Field, "and the entry at fault, by index")

	after := gradesOf(t, layout)
	assert.Len(t, after, 1, "§6.2.3's refusal takes the whole file with it")
}
