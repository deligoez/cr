package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// statusRecordRound adds to statusHome's round one of everything §10.1.4
// through §10.1.6 have to report.
//
// Four records: one in every state §9.1 gives a round that has been drafted and
// posted — `posted`, `queued`, `duplicate` and `draft` — one of each of §6.1's
// four severities, and the three grades of §6.2 with `cited` twice. Two probes,
// of which one stands behind the `probed` record and one behind nothing. And a
// `waived` count in the round summary, which is the only place §6.4.4 lets a
// dropped finding leave a trace.
//
// The lines are written as stored JSON rather than through `cr record`, for the
// reason writeStatusRound writes its units and cells that way: what is being
// measured is the report over a round's state, and a fixture built through the
// commands would be measuring those commands as well.
func statusRecordRound(t *testing.T, layout state.Layout, head string) {
	t.Helper()
	record := func(id, kind, severity, grade, recordState, extra string) string {
		return `{"id":"` + id + `","kind":"` + kind + `","axis":"correctness",` +
			`"role":"correctness","class":"missing-error-check","severity":"` + severity + `",` +
			`"grade":"` + grade + `","unit":"u1",` +
			`"anchor":{"path":"lib.go","side":"RIGHT","start_line":4,"line":4,` +
			`"content_hash":"9a8b7c6"},"summary":"s","evidence":"e","state":"` + recordState + `"` +
			extra + `,"head":"` + head + `","round":1}` + "\n"
	}
	probe := func(id, kind string) string {
		return `{"id":"` + id + `","kind":"` + kind + `","input":"p","result":"failed",` +
			`"baseline":"r1","target":"lib.go:4","duration_ms":12,"output_tail":"",` +
			`"head":"` + head + `","round":1}` + "\n"
	}

	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(
		record("f1", "finding", "high", "probed", "posted", `,"probe":"p1"`)+
			record("f2", "question", "medium", "argued", "queued", "")+
			record("f3", "finding", "low", "cited", "duplicate", `,"duplicate_of":"f1"`)+
			record("f4", "finding", "critical", "cited", "draft", ""))))
	require.NoError(t, held.Write(state.FileProbes, []byte(probe("p1", "mutation")+probe("p2", "gap"))))
	require.NoError(t, state.UpdateRoundSection(held, 1, state.FileSummary, summaryWaived,
		finding.Drops{Dropped: 2, Waivers: []string{"wr1", "wp2"}}))
	require.NoError(t, held.Unlock())
}

// statusReport is the part of `cr status`'s document §10.1.4 through §10.1.6
// and §3.6.6 answer, decoded the way a caller of the JSON shape would.
type statusReport struct {
	Records struct {
		Total      int     `json:"total"`
		ByState    []tally `json:"by_state"`
		BySeverity []tally `json:"by_severity"`
		ByGrade    []tally `json:"by_grade"`
	} `json:"records"`
	Probes struct {
		Run    int `json:"run"`
		Graded int `json:"graded"`
	} `json:"probes"`
	Unstanding []unstandingNote `json:"unstanding_notes"`
	Honesty    []string         `json:"honesty"`
}

// readStatus runs `cr status` over the fixture and decodes the document.
func readStatus(t *testing.T) statusReport {
	t.Helper()
	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report statusReport
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report
}

// countedAs is the count a tally holds for one name, and -1 for a name the
// tally has no row for — which is a different failure from a count of zero and
// is asserted as one.
func countedAs(rows []tally, name string) int {
	for _, row := range rows {
		if row.Name == name {
			return row.Count
		}
	}
	return -1
}

// §10.1.4 and §10.1.5 over a round holding a record in four states, one of each
// severity, and two probes of which one graded a record.
//
// Every value of every vocabulary is asserted, the zeros included: §9.1's seven
// states, §6.1's four severities and §6.2's three grades each appear whether or
// not the round reached them, for the reason finding.Drops prints its own zero
// case — a row that appeared only when it was non-zero would leave a reader
// unable to tell a round that produced nothing in that bucket from a report
// that never counted it.
func TestStatusCountsRecordsByStateSeverityAndGradeWithTheProbesBehindThem(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	statusRecordRound(t, layout, meta.Head)

	report := readStatus(t)

	assert.Equal(t, 4, report.Records.Total)
	require.Len(t, report.Records.ByState, 7, "§9.1's table has seven states and each gets a row")
	for name, count := range map[string]int{
		"draft": 1, "queued": 1, "duplicate": 1,
		"suppressed": 0, "discarded": 0, "posted": 1, "stale": 0,
	} {
		assert.Equal(t, count, countedAs(report.Records.ByState, name), "state %s", name)
	}
	for name, count := range map[string]int{
		"critical": 1, "high": 1, "medium": 1, "low": 1,
	} {
		assert.Equal(t, count, countedAs(report.Records.BySeverity, name), "severity %s", name)
	}
	for name, count := range map[string]int{"probed": 1, "cited": 2, "argued": 1} {
		assert.Equal(t, count, countedAs(report.Records.ByGrade, name), "grade %s", name)
	}

	// §10.1.5. Both probes ran; only p1 stands behind a record graded
	// `probed`, and §6.2's table makes that grade unreachable without it.
	assert.Equal(t, 2, report.Probes.Run)
	assert.Equal(t, 1, report.Probes.Graded)
}

// §10.1.6's two counts reach the reader through the channel §11.1 exempts from
// `--quiet`, and say the same thing with the flag as without it.
//
// The waiver count is read out of the round summary because §6.4.4 leaves it
// nowhere else — a dropped finding is never written to findings.ndjson — and
// the duplicate count is read off the round's own records, because §6.4.3
// retains a suppressed duplicate in state `duplicate` with `duplicate_of`
// naming the representative.
func TestStatusDisclosesTheWaiverAndDuplicateCountsUnderQuiet(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	statusRecordRound(t, layout, meta.Head)

	disclosed := readStatus(t).Honesty
	honesty := strings.Join(disclosed, "\n")
	for _, expected := range []string{
		"2 finding(s) dropped by 2 active waiver(s), per §6.4.4",
		"1 record(s) suppressed as duplicates across 1 anchored line(s), per §6.4.3",
	} {
		// A whole disclosure, not a substring of the joined lines: gremlins
		// found a count running down to "-1 record(s) suppressed ..." passing
		// here, because that line contains this one.
		assert.Contains(t, disclosed, expected)
	}

	quieted, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug, "--quiet")
	require.NoError(t, err)
	var underQuiet statusReport
	require.NoError(t, json.Unmarshal([]byte(quieted), &underQuiet))
	assert.Equal(t, honesty, strings.Join(underQuiet.Honesty, "\n"),
		"§11.1: `--quiet` may not suppress §10.1.6's counts")
}

// A round whose summary carries no waiver count is reported as carrying none,
// not as having dropped nothing.
//
// The two are different facts and the second is the one worth knowing: §6.5.1
// makes `cr merge` write the count, so its absence says the merge has not run
// for this round — while a zero says the pass ran and matched nothing, and a
// reader shown a zero for both could not tell that the draft they are about to
// read may re-raise everything the reviewer set aside.
func TestAnAbsentWaiverCountIsReportedAsAbsentRatherThanAsZero(t *testing.T) {
	statusHome(t)

	honesty := strings.Join(readStatus(t).Honesty, "\n")

	assert.Contains(t, honesty, "no §6.4.4 waiver drop recorded for round 1")
	assert.NotContains(t, honesty, "0 finding(s) dropped",
		"a merge that has not run has not dropped nothing; it has not been asked")
}

// §3.6.6 over a round that rests on a note the reviewer has since retracted:
// the coverage cell citing it and the record whose claim came from it are both
// reported as needing re-evaluation, in the round the retraction happened in.
//
// The record reaches the note through its claim, which is the path §8.1.6's
// provenance region walks: §6.1's record carries no note of its own, and
// §3.3.1 gives a claim with `source: note` the id.
func TestStatusReportsTheNotesTheRoundRestsOnThatNoLongerStand(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	statusRecordRound(t, layout, meta.Head)
	retracted := retractedNoteRound(t, layout, meta.Head)

	report := readStatus(t)

	require.Len(t, report.Unstanding, 1)
	assert.Equal(t, retracted, report.Unstanding[0].Note)
	assert.Equal(t, note.StandingRetracted, report.Unstanding[0].Standing)
	assert.Equal(t, []string{"u1/intent-coverage"}, report.Unstanding[0].Cells,
		"§4.1.5 has the cell cite the note, and §4.5.6 keys a cell by (unit, role)")
	assert.Equal(t, []string{"f5"}, report.Unstanding[0].Records,
		"the record cites the note through the claim it names")
}

// A note that still stands is not reported, because nothing about it needs
// re-evaluating: §3.6.6's report is about revoked hearsay, and a report listing
// every note the round rests on would bury the one that was withdrawn.
func TestAStandingNoteIsNotReportedAsNeedingReEvaluation(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	statusRecordRound(t, layout, meta.Head)
	recorded, err := note.Append(layout, fixtureIssue, "the retry is deliberate",
		note.SourceChat, fixturePRNumber, time.Now())
	require.NoError(t, err)
	restNoteRound(t, layout, meta.Head, recorded.ID)

	assert.Empty(t, readStatus(t).Unstanding)
}

// retractedNoteRound records a note against the round's issue key, retracts it,
// and leaves the round resting on it from both sides §3.6.6 names: a coverage
// cell citing it, and a claim that a record names.
func retractedNoteRound(t *testing.T, layout state.Layout, head string) string {
	t.Helper()
	recorded, err := note.Append(layout, fixtureIssue, "the retry is deliberate",
		note.SourceChat, fixturePRNumber, time.Now())
	require.NoError(t, err)
	restNoteRound(t, layout, head, recorded.ID)
	_, err = note.Retract(layout, recorded.ID, time.Now())
	require.NoError(t, err)
	return recorded.ID
}

// restNoteRound leaves the round resting on one note: the intent-coverage cell
// of `u1` cites it, and a claim drawn from it is named by a record.
//
// The cells are rewritten rather than added to, so `u1` still holds one cell
// per active role and §10.1.1's counts are the ones statusHome's own test
// asserts.
func restNoteRound(t *testing.T, layout state.Layout, head, noteID string) {
	t.Helper()
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	var cells strings.Builder
	for _, role := range []string{"convention", "correctness", "intent-coverage"} {
		cited := ""
		if role == "intent-coverage" {
			cited = `,"note_id":"` + noteID + `"`
		}
		cells.WriteString(`{"unit":"u1","role":"` + role + `","result":"pass","unit_hash":"h1"` +
			cited + `,"head":"` + head + `","round":1}` + "\n")
	}
	require.NoError(t, held.Write(state.FileCoverage, []byte(cells.String())))

	claims, err := layout.ReadPR(fixtureOwner, fixtureProject, fixturePRNumber, state.FileClaims)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileClaims, append(claims,
		[]byte(`{"id":"`+fixtureIssue+`#c4","text":"Load retries twice.","source":"note",`+
			`"span":"the retry is deliberate","note_id":"`+noteID+`",`+
			`"head":"`+head+`","round":1}`+"\n")...)))

	records, err := layout.ReadPR(fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, append(records,
		[]byte(`{"id":"f5","kind":"question","axis":"intent","role":"intent-coverage",`+
			`"class":"unimplemented-claim","severity":"medium","grade":"argued","unit":"u1",`+
			`"claim":"`+fixtureIssue+`#c4",`+
			`"anchor":{"path":"lib.go","side":"RIGHT","start_line":4,"line":4,`+
			`"content_hash":"9a8b7c6"},"summary":"s","evidence":"e","state":"draft",`+
			`"head":"`+head+`","round":1}`+"\n")...)))
	require.NoError(t, held.Unlock())
}

// The terminal rendering carries §10.1.4 through §10.1.6 and §3.6.6 too, so a
// reader of one shape and a reader of the other are told the same thing (§12.1).
func TestTheStatusTextCarriesTheRecordCountsAndTheRetractedNotes(t *testing.T) {
	statusHome(t)
	layout := state.New(os.Getenv(state.HomeEnv))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	statusRecordRound(t, layout, meta.Head)
	retracted := retractedNoteRound(t, layout, meta.Head)

	printed := throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color")

	for _, expected := range []string{
		"records: 5 total",
		"  by state: draft 2, queued 1, duplicate 1, suppressed 0, discarded 0, posted 1, stale 0",
		"  by severity: critical 1, high 1, medium 2, low 1",
		"  by grade: probed 1, cited 2, argued 2",
		"probes: 2 run, 1 standing behind a graded record",
		"notes no longer standing, per §3.6.6: 1",
		retracted + " retracted: 1 cell(s), 1 record(s), 0 set-aside(s) need re-evaluation " +
			"(u1/intent-coverage, f5)",
		"2 finding(s) dropped by 2 active waiver(s), per §6.4.4",
		"1 record(s) suppressed as duplicates across 1 anchored line(s), per §6.4.3",
	} {
		assert.Contains(t, printed, expected)
	}
}
