package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// writeDraft puts a draft back where `cr draft` wrote it, the way a reviewer
// saves the file they edited.
func writeDraft(t *testing.T, layout state.Layout, file string) {
	t.Helper()
	require.NoError(t, os.WriteFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft),
		[]byte(file), 0o600))
}

// draftedFindings reads the round's records back out of findings.ndjson.
func draftedFindings(t *testing.T, layout state.Layout) []finding.Finding {
	t.Helper()
	stored, err := state.ReadRecords[finding.Finding](
		layout, draftOwner, draftRepo, draftPRNum, state.FileFindings)
	require.NoError(t, err)
	return stored
}

// §7.2 through the command: a marker edit the table does not admit aborts with
// exit code 1, naming the record id, and nothing of the run reaches disk.
//
// The three rows here are the ones that need no tree behind them. The location
// row's abort is in the test below, where there is a checkout to resolve
// against.
func TestAMarkerEditSection72RefusesAbortsWithTheValidationCode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		from  string
		to    string
		says  string
		names string
	}{
		{name: "kind names no register", from: `kind="finding"`, to: `kind="bug"`, says: "bug"},
		{
			name: "not-here is set by hand",
			from: `disposition=""`, to: `disposition="not-here"`,
			says: "delete the block to say it",
		},
		{
			name: "severity names no severity",
			from: `severity="high"`, to: `severity="urgent"`,
			says: "critical, high, medium, low",
		},
		{
			name: "the id is changed",
			from: `id="f1"`, to: `id="f9"`,
			says: "no manual-comment channel",
			// The id the abort names is the one the round does
			// not hold: §7.2's `id` is immutable, so the block
			// carrying f9 is the fault and f1 is a record with no
			// block rather than a record with a bad one.
			names: "f9",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout := draftedHome(t, aStoredRecord("f1", finding.StateDraft))
			redraft(t)
			edited := markerEdit(t, readDraft(t, layout), "f1", tc.from, tc.to)
			writeDraft(t, layout, edited)
			named := tc.names
			if named == "" {
				named = "f1"
			}

			_, err := runDraft(t, draftPR, "--repo", draftSlug)

			require.Error(t, err)
			assert.Equal(t, ExitValidation, exitCodeFor(err), "§7.2 codes it 1")
			assert.Contains(t, err.Error(), tc.says)
			assert.Contains(t, err.Error(), named, "§7.2: the abort names the record id")
			assert.Equal(t, edited, readDraft(t, layout),
				"a refused run leaves the reviewer's edits in the file for the next one to read")
			assert.Equal(t, finding.StateQueued, draftedFindings(t, layout)[0].State,
				"and leaves findings.ndjson as it was")
		})
	}
}

// The changed id names the id the round does not hold, which is the half of
// §7.2's immutable `id` row that is observable: the record it was taken from
// has no block left, and the block has no record.
func TestAChangedMarkerIDNamesTheIDTheRoundDoesNotHold(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f1", finding.StateDraft))
	redraft(t)
	writeDraft(t, layout, markerEdit(t, readDraft(t, layout), "f1", `id="f1"`, `id="f9"`))

	_, err := runDraft(t, draftPR, "--repo", draftSlug)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "f9")
}

// §7.2's severity row through the command: freely editable, applied to the
// stored record, and reported as something cr acted on.
func TestAnEditedSeverityReachesTheStoredRecordAndIsReported(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f1", finding.StateDraft))
	redraft(t)
	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f1", `severity="high"`, `severity="low"`))

	out := throughATerminal(t, "draft", draftPR, "--repo", draftSlug)

	assert.Equal(t, finding.SeverityLow, draftedFindings(t, layout)[0].Severity)
	assert.Contains(t, out, "\nf1: severity low\n")
	assert.Contains(t, readDraft(t, layout), `severity="low"`,
		"and the next rendering writes the reviewer's value back out")
}

// anchoredCheckout is a repository holding one file at one commit, and the head
// that commit is, so §7.2's location row has a tree to re-validate against.
func anchoredCheckout(t *testing.T, path string, lines ...string) (dir, head string) {
	t.Helper()
	dir = t.TempDir()
	full := filepath.Join(dir, filepath.FromSlash(path))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
	require.NoError(t, os.WriteFile(full, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	mustGit(t, dir, "add", path)
	mustGit(t, dir, "commit", "--quiet", "-m", "the commit under review")
	return dir, strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
}

// draftedAgainst is draftedHome with the round's head set to a real commit, so
// the anchor the marker moves resolves against a tree cr can open.
func draftedAgainst(t *testing.T, head string, records ...*finding.Finding) state.Layout {
	t.Helper()
	layout := draftedHome(t, records...)
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: draftOwner, Repo: draftRepo, PR: draftPRNum, Round: draftRound, Head: head,
	}))
	require.NoError(t, held.Unlock())
	return layout
}

// formedUnit writes the round's units.ndjson as one unit u1 adding head lines
// start to end of path, stamped with head, so §6.1.3's containment has the unit
// a moved anchor is measured against.
func formedUnit(t *testing.T, layout state.Layout, head, path string, start, end int) {
	t.Helper()
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileUnits, fmt.Appendf(nil,
		`{"id":"u1","path":%q,"side":"RIGHT","head_ranges":[{"start":%d,"end":%d}],"head":%q,"round":%d}`+"\n",
		path, start, end, head, draftRound)))
	require.NoError(t, held.Unlock())
}

// §7.2's `path`, `start_line` and `line` row through the command: re-validated
// per §6.1.2 against the round's head, applied to the stored record when it
// resolves, and refused when it does not — and, per round 12's
// anchor-hash-not-recomputed finding, carrying §9.2's content hash recomputed
// over the lines it now names.
func TestAMovedAnchorIsRevalidatedAgainstTheHeadAndRehashed(t *testing.T) {
	anchored := "internal/api/handler.go"
	dir, head := anchoredCheckout(t, anchored, "package api", "func Handle() {", "\treturn", "}")
	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })

	record := aStoredRecord("f1", finding.StateDraft)
	record.Anchor = finding.Anchor{Path: anchored, Side: "RIGHT", StartLine: 2, Line: 2, ContentHash: "stale"}
	layout := draftedAgainst(t, head, record)
	formedUnit(t, layout, head, anchored, 1, 4)
	redraft(t)
	first := readDraft(t, layout)

	// The whole pair is replaced: `start_line="2"` ends in `line="2"`, so a
	// replacement of the shorter string would move the field before it.
	writeDraft(t, layout,
		markerEdit(t, first, "f1", `start_line="2" line="2"`, `start_line="2" line="4"`))
	_, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	moved := draftedFindings(t, layout)[0].Anchor
	assert.Equal(t, 4, moved.Line, "the reviewer's range is the record's range")
	hash, err := finding.AnchorContentHash([]string{"func Handle() {", "\treturn", "}"})
	require.NoError(t, err)
	assert.Equal(t, hash, moved.ContentHash,
		"§9.2's hash is recomputed over the lines the anchor now names, not left on the ones it left")

	writeDraft(t, layout, markerEdit(t, readDraft(t, layout),
		"f1", `start_line="2" line="4"`, `start_line="2" line="90"`))
	_, err = runDraft(t, draftPR, "--repo", draftSlug)

	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Contains(t, err.Error(), "f1")
	assert.Contains(t, err.Error(), "holds 4 lines")
}
