package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The pull request `cr record` is exercised against, and the round it stands
// in. The round is past its first because §3.4.6 scopes a unit id to its round:
// a fixture in round 1 could not tell a unit set drawn from the round apart
// from one drawn from the whole file.
const (
	recordOwner = "acme"
	recordRepo  = "api"
	recordSlug  = recordOwner + "/" + recordRepo
	recordPR    = "13"
	recordPRNum = 13
	recordRound = 2
	recordHead  = "103679608f0a803e39b1511f519e8e70067443c6"
	// staleUnit is a unit of the round before, which this round did not
	// form and no record of it may name.
	staleUnit = "u3"
)

// recordedHome puts a state root behind CR_HOME holding one pull request in
// round recordRound, with two units of that round and one of the round before.
//
// The units are written as bytes rather than through a record type, and the
// line carries only the id, the path, the hunk ranges and the stamp. That is
// deliberate: those are the whole of what `cr record` reads out of the file —
// the id for §6.1.3, and the path and ranges for the anchor binding of round
// 13's agent-chosen-grading-boundary — and a fixture built from §3.4.6's record
// would stop proving that a line holding no other field is read correctly. The
// two units of the round hold disjoint ranges of one file, so a record naming
// the wrong one is refused rather than passing on an overlap.
func recordedHome(t *testing.T) state.Layout {
	t.Helper()
	dir := recordCheckout(t)
	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(recordOwner, recordRepo, recordPRNum))

	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: recordOwner, Repo: recordRepo, PR: recordPRNum,
		Round: recordRound, Head: recordHead,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(strings.Join([]string{
		unitLine("u1", recordHead, recordRound, recordUnitStart["u1"]),
		unitLine("u2", recordHead, recordRound, recordUnitStart["u2"]),
		unitLine(staleUnit, "1f2e3d4c5b6a79880997a6b5c4d3e2f11f2e3d4c", recordRound-1, recordUnitStart["u1"]),
	}, "\n")+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// recordPath is the file every unit of the fixture round is formed in, and the
// file aRecord anchors on.
const recordPath = "internal/api/handler.go"

// recordCheckout is the repository recordHead names: one commit holding
// recordPath, a hundred numbered lines long, so every anchor aRecord writes
// resolves at the round's head and §9.2.3's hash and window can be read off it.
//
// The commit is built with a fixed identity and fixed dates, so its id is the
// same on every machine and recordHead can stay a constant the assertions name.
// The check below is what keeps that true: a git that wrote the object
// differently fails here rather than as a head mismatch somewhere downstream.
func recordCheckout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body := make([]string, 0, 100)
	for n := 1; n <= 100; n++ {
		body = append(body, fmt.Sprintf("handler line %d", n))
	}
	path := filepath.Join(dir, filepath.FromSlash(recordPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(body, "\n")+"\n"), 0o600))
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	mustGit(t, dir, "add", recordPath)
	commit := exec.Command("git", "-C", dir, "-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "the handler under review")
	commit.Env = []string{
		"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=cr fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=cr fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid",
		"GIT_AUTHOR_DATE=2026-09-13T00:00:00Z", "GIT_COMMITTER_DATE=2026-09-13T00:00:00Z",
	}
	out, err := commit.CombinedOutput()
	require.NoError(t, err, string(out))
	require.Equal(t, recordHead, strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD")),
		"the fixture commit is built deterministically so recordHead can name it")
	return dir
}

// recordUnitStart is the first head line of each fixture unit's one hunk range,
// which runs for seven lines; aRecord anchors two lines into it.
var recordUnitStart = map[string]int{"u1": 40, "u2": 90}

// unitLine is one line of units.ndjson carrying the fields this command reads:
// §3.4.6's id, path and hunk ranges, and §2.3.3's round.
func unitLine(id, head string, round, start int) string {
	return fmt.Sprintf(
		`{"id":%q,"path":%q,"hunk_ranges":[{"start":%d,"end":%d}],"head":%q,"round":%d}`,
		id, recordPath, start, start+6, head, round)
}

// aRecord is the §6.1 fields an agent supplies, complete and valid. A test
// makes exactly one thing wrong with a copy of it, so what a rejection proves
// is that one fault and not some second thing the fixture never had.
//
// The anchor sits inside the unit the record names, two lines into its range,
// so the binding of round 13's agent-chosen-grading-boundary is one of the
// things the record gets right. A unit the fixture round does not form takes
// u1's lines, since §6.1.3 refuses it before the anchor is read.
func aRecord(id, unit string) map[string]any {
	start, formed := recordUnitStart[unit]
	if !formed {
		start = recordUnitStart["u1"]
	}
	return map[string]any{
		"id":       id,
		"kind":     "finding",
		"role":     "correctness",
		"class":    "unchecked-error",
		"severity": "high",
		"unit":     unit,
		"anchor": map[string]any{
			"path":         recordPath,
			"side":         "RIGHT",
			"start_line":   start + 2,
			"line":         start + 4,
			"content_hash": "0123456789abcdef",
		},
		"summary":  "The error Decode returns is dropped.",
		"evidence": "The call's second result is assigned to the blank identifier.",
	}
}

// writeRecordFile writes records as the NDJSON file an agent hands `cr record`,
// and returns its path. It sits in a directory of its own, outside the state
// tree: the file is the agent's own output, and cr only ever reads it.
func writeRecordFile(t *testing.T, name string, records ...map[string]any) string {
	t.Helper()
	var body bytes.Buffer
	for _, record := range records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		body.Write(line)
		body.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, body.Bytes(), 0o600))
	return path
}

// runRecord runs `cr record` with args against whatever CR_HOME points at, and
// returns what it printed and what it refused.
func runRecord(t *testing.T, args ...string) (printed string, err error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"record"}, args...))
	// Executed first and read after, for the reason runAnswer gives.
	err = cmd.Execute()
	return out.String(), err
}

// §11 and §9.1 through the command: every record of an accepted file reaches
// findings.ndjson, in `draft`, stamped with the round's head.
//
// The three fields asserted on each stored record are the three the agent could
// not have written. §6.1.4 reserves `state` and §2.3.3 reserves head and round,
// and all three are refused on the wire, so a record carrying them on disk got
// them from cr — which is what makes reading them back a test of this command
// rather than of the fixture.
func TestRecordStoresEveryRecordOfAnAcceptedFile(t *testing.T) {
	layout := recordedHome(t)
	file := writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"), aRecord("f2", "u2"))

	printed, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings,
	)
	require.NoError(t, err)
	require.Len(t, stored, 2, "§11: cr record stores the round's merged findings")
	assert.Equal(t, []string{"f1", "f2"}, []string{stored[0].ID, stored[1].ID},
		"the file's order is the file's, and cr reorders nothing")

	for _, held := range stored {
		assert.Equal(t, finding.StateDraft, held.State,
			"§9.1: cr record is the actor that brings a new record into draft")
		assert.Equal(t, recordHead, held.Head, "§2.3.3: cr writes head on every write")
		assert.Equal(t, recordRound, held.Round, "§2.3.3: and round with it")
	}

	assert.Contains(t, printed, `"state": "draft"`,
		"the stored records are handed back, carrying what cr wrote onto them")
}

// §6.1.3 through the command: a refused line takes the whole file with it, and
// the refusal names the file, the one-based line, and the field.
//
// The fault is put on the third line of four on purpose. Two well-formed
// records precede it, so a command that wrote as it validated would already
// have stored them, and one follows it, so a command that carried on would have
// stored that too. Neither may reach the file: the agent is about to correct
// the input and hand the whole of it in again, and a half-stored round would
// duplicate every line above the fault.
//
// A round is recorded first, so "nothing is written" is asserted against a file
// that already holds something. findings.ndjson is appended to (§2.3), and an
// append that failed by publishing a truncated file would pass a check that
// only counted the new records.
func TestARefusedLineLeavesFindingsExactlyAsItWas(t *testing.T) {
	layout := recordedHome(t)
	accepted := writeRecordFile(t, "first.ndjson", aRecord("f1", "u1"))
	_, err := runRecord(t, recordPR, accepted, "--repo", recordSlug)
	require.NoError(t, err)

	stored := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
	before, err := os.ReadFile(stored)
	require.NoError(t, err)
	require.NotEmpty(t, before, "the fixture round has to have reached the file")

	faulty := aRecord("f4", "u1")
	delete(faulty, "evidence")
	refused := writeRecordFile(t, "second.ndjson",
		aRecord("f2", "u1"), aRecord("f3", "u2"), faulty, aRecord("f5", "u2"))

	_, err = runRecord(t, recordPR, refused, "--repo", recordSlug)
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.1.3 rejects with exit code 1")
	assert.Contains(t, err.Error(), refused, "the refusal names the file")
	assert.Contains(t, err.Error(), "line 3", "and the one-based line the record sits on")
	assert.Contains(t, err.Error(), "evidence", "and the field at fault")

	after, err := os.ReadFile(stored)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"§6.1.3 refuses the file, so neither the lines above the fault nor the line below it are stored")
}

// §6.1.3's three refusals reach the command, each naming the field at fault.
//
// The point is not that three faults are caught. It is that the command adds no
// reading of §6.1 of its own: finding.Decode already holds every line to the
// section, and all three refusals arrive here as the RejectedRecordError it
// raises, carrying the file, the line and the field it named. A command that
// validated for itself would answer some of these and not others, or answer
// them in a shape internal/cli does not map onto §11.2's code 1.
//
// The unknown unit is the round before's rather than an invented id, so the
// refusal proves the scoping as well as the membership: §3.4.6 makes a unit id
// round-scoped, and a set drawn from the whole of units.ndjson would accept it.
func TestRecordRefusesTheThreeFaultsSection613Names(t *testing.T) {
	recordedHome(t)

	refuse := func(t *testing.T, name string, record map[string]any) string {
		t.Helper()
		file := writeRecordFile(t, name, record)
		_, err := runRecord(t, recordPR, file, "--repo", recordSlug)
		require.Error(t, err)
		assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.1.3 rejects with exit code 1")

		var rejected *finding.RejectedRecordError
		require.ErrorAs(t, err, &rejected, "the command raises no rejection of its own")
		assert.Equal(t, file, rejected.File)
		assert.Equal(t, 1, rejected.Line)
		return rejected.Field
	}

	missing := aRecord("f1", "u1")
	delete(missing, "summary")
	assert.Equal(t, "summary", refuse(t, "missing.ndjson", missing),
		"a record missing a field §6.1 requires is refused by that field's name")

	assert.Equal(t, "unit", refuse(t, "stale-unit.ndjson", aRecord("f1", staleUnit)),
		"§3.4.6 scopes a unit id to its round, so the round before formed no unit of this one")

	assert.Equal(t, "role", refuse(t, finding.FanOutFile("test"), aRecord("f1", "u1")),
		"§6.1.3 binds a record's role to the role whose §4.6.2 output file it arrived in")
}

// §12.1's other shape for this command. A terminal reader gets the count and
// the state, not the documents: the records came out of the caller's own file,
// so the one thing the run produced that they do not already have is what §9.1
// made of them.
func TestATerminalRecordNamesTheCountAndTheState(t *testing.T) {
	recordedHome(t)
	file := writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"), aRecord("f2", "u2"))

	out := throughATerminal(t, "record", recordPR, file, "--repo", recordSlug)

	assert.Contains(t, out, "recorded ")
	assert.Contains(t, out, "\x1b[36m2\x1b[0m",
		"the count is accented, as every terminal rendering accents its answer")
	assert.Contains(t, out, " in state draft")
}

// §6.4.3 retains a suppressed duplicate rather than dropping it, and §6.5.1
// says where that happens: `cr merge`'s output carries `duplicate_of` and no
// `state` at all, so this command is what applies §6.4.3 from it and stamps the
// state §9.1's second row allows.
//
// Both halves are asserted on the file, because both are how a duplicate stays
// accountable. The record is still there, so `cr status` can report what was
// suppressed and a reviewer can see that two roles agreed; and it names the
// representative, so what spoke in its place is a record id rather than a
// recollection of the merge that ran.
func TestASuppressedDuplicateIsStoredInThatStateNamingItsRepresentative(t *testing.T) {
	layout := recordedHome(t)
	suppressed := aRecord("f2", "u2")
	suppressed["duplicate_of"] = "f1"
	// Three records and one duplicate, so the two counts differ: a
	// fixture with one of each reads the same whichever state is being
	// counted, and mutation testing found exactly that hole here.
	file := writeRecordFile(t, "merged.ndjson",
		aRecord("f1", "u1"), suppressed, aRecord("f3", "u2"))

	printed, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings,
	)
	require.NoError(t, err)
	require.Len(t, stored, 3, "§6.4.3 retains the duplicate; a suppressed record is not a dropped one")

	assert.Equal(t, finding.StateDraft, stored[0].State,
		"the representative is an ordinary new record")
	assert.Equal(t, finding.StateDuplicate, stored[1].State,
		"§6.4.3 and §9.1's second row: cr record moves a marked record on to duplicate")
	assert.Equal(t, "f1", stored[1].DuplicateOf,
		"§6.4.3: the retained record names the representative it was retired for")

	assert.Contains(t, printed, `"duplicates": 1`,
		"§10.1.6's count reaches the caller as a number, not only inside the records")
}

// §12.1's other shape for the same run. A terminal reader is told the two
// states apart once any record was retired, because §6.4.3 keeps a suppressed
// duplicate in the file and a line calling every stored record `draft` would
// count records this round will never draft.
func TestATerminalRecordNamesTheDuplicatesApartFromTheDrafts(t *testing.T) {
	recordedHome(t)
	suppressed := aRecord("f2", "u2")
	suppressed["duplicate_of"] = "f1"
	// Two drafts against one duplicate, for the reason the case above
	// gives: equal counts cannot tell the two states apart.
	file := writeRecordFile(t, "merged.ndjson",
		aRecord("f1", "u1"), suppressed, aRecord("f3", "u2"))

	out := throughATerminal(t, "record", recordPR, file, "--repo", recordSlug)

	assert.Contains(t, out, "\x1b[36m3\x1b[0m", "the count is accented, as every terminal rendering is")
	assert.Contains(t, out, "2 in state draft")
	assert.Contains(t, out, "1 in state duplicate")
}

// §2.3.1's write is what the command is for, and a caller has to be told when
// it fails. A findings.ndjson that is a directory refuses at exactly that
// point: the round is briefed, the file is accepted, §2.3.1's lock is taken,
// and only then does the store refuse to be read back and appended to.
//
// gremlins found this. Negating the guard on the append's error leaves
// `cr record` reporting success on a round that reached no file — the lock is
// released on the way out of both branches, so the whole of the difference is
// whether the failure reaches the caller, and nothing asserted that it does.
//
// The second run is the other half of the same comment: the lock is released on
// the way out of the failing branch too, so a run that follows a failed one
// reaches the same refusal rather than a lock timeout.
func TestAFailedAppendIsReportedRatherThanSwallowed(t *testing.T) {
	layout := recordedHome(t)
	dir := filepath.Dir(layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings))
	// Read-only rather than a directory in the store's place: §2.3's write is
	// atomic, so it fails on the temporary file it cannot create, while every
	// read of the round — the meta, the units, and the read-back the rule
	// statistics take — still succeeds. That is what leaves the append as the
	// only thing that failed.
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	require.NoError(t, os.Chmod(dir, 0o500))

	file := writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"))

	_, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.Error(t, err, "a round that reached no file was not recorded")
	assert.Contains(t, err.Error(), dir, "the refusal names where the write failed")

	_, again := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.Error(t, again, "the lock was released, so the second run reaches the write too")
	assert.Contains(t, again.Error(), dir)
}
