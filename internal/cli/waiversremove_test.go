package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// openNextRound moves recordedHome's pull request on to the round after the
// one it was recorded in, with the same two units, the way a later `cr brief`
// leaves it for the merge that round runs.
func openNextRound(t *testing.T, layout state.Layout) {
	t.Helper()
	next := recordRound + 1
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: recordOwner, Repo: recordRepo, PR: recordPRNum, Round: next, Head: recordHead,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		unitLine("u1", recordHead, next, recordUnitStart["u1"])+"\n"+
			unitLine("u2", recordHead, next, recordUnitStart["u2"])+"\n")))
	require.NoError(t, held.Unlock())
}

// §7.4.7 and §7.4.4 from both scopes: a waiver removed from either file stops
// silencing, so the finding it dropped at one round's merge is raised again at
// the next round's.
//
// Each scope is waived the way triage waives it — `wrong` into the
// repository-wide file, `not-here` into the pull request's — and removed by the
// id the listing prints, `--pr` given for the pull request's. The first merge
// is the control: the same record, dropped, which is what makes the second
// merge's raised record the removal's doing rather than a key that never
// matched.
func TestARemovedWaiverRaisesItsFindingAgainOnTheNextRound(t *testing.T) {
	for scope, removal := range map[string]struct {
		disposition finding.Disposition
		flags       []string
	}{
		"repository":   {disposition: finding.DispositionWrong},
		"pull-request": {disposition: finding.DispositionNotHere, flags: []string{"--pr", recordPR}},
	} {
		t.Run(scope, func(t *testing.T) {
			layout := recordedHome(t)
			stored, err := finding.Waive(layout, recordOwner, recordRepo, &finding.Waiver{
				WaiverKey: finding.WaiverKey{
					Path: recordPath, Side: "RIGHT", Class: "unchecked-error", ContentHash: recordedHash(t, "u1"),
				},
				Disposition: removal.disposition,
			}, finding.WaiverProvenance{Round: recordRound, PR: recordPRNum, Head: recordHead})
			require.NoError(t, err)
			file := writeFanOut(t, t.TempDir(), "correctness",
				aRoleRecord("f1", "correctness", "unchecked-error", "u1"))

			dropped, err := runMergeCLI(t, mergedOut(t), file)
			require.NoError(t, err)
			require.Equal(t, 1, decodeMergeResult(t, dropped).Waived.Dropped,
				"the control: while the waiver stands, §6.4.4 drops the finding")

			printed, err := runCLIPrinting(t, append([]string{
				"waivers", "remove", stored.ID, "--repo", recordSlug,
			}, removal.flags...)...)
			require.NoError(t, err)
			var removed struct {
				Removed struct {
					ID    string `json:"id"`
					Scope string `json:"scope"`
				} `json:"removed"`
			}
			require.NoError(t, json.Unmarshal([]byte(printed), &removed))
			assert.Equal(t, stored.ID, removed.Removed.ID)
			assert.Equal(t, scope, removed.Removed.Scope, "§7.4.7 deletes from either scope")
			active, err := finding.ActiveWaivers(layout, recordOwner, recordRepo, recordPRNum)
			require.NoError(t, err)
			assert.Empty(t, active, "§7.4.4: the removal leaves neither file holding the waiver")

			openNextRound(t, layout)
			out := mergedOut(t)
			raised, err := runMergeCLI(t, out, file)
			require.NoError(t, err)
			reported := decodeMergeResult(t, raised)
			assert.Equal(t, 0, reported.Waived.Dropped, "nothing silences the finding any more")
			assert.Equal(t, 1, reported.Merged, "the finding the waiver dropped is raised again")
			body, err := os.ReadFile(out)
			require.NoError(t, err)
			assert.Contains(t, string(body), `"id":"f1"`)
		})
	}
}

// An id no file holds is refused with §11.2's code 1 and a hint naming the
// listing, and a pull-request id without `--pr` is refused before any file is
// opened, since the id alone does not say which pull request's file it is in.
func TestARemovalNamingNoWaiverIsRefused(t *testing.T) {
	recordedHome(t)

	err := runCLI(t, "waivers", "remove", "wr9", "--repo", recordSlug)
	var unknown *finding.UnknownWaiverError
	require.ErrorAs(t, err, &unknown)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Contains(t, hintFor(err), "cr waivers list")

	err = runCLI(t, "waivers", "remove", "wp1", "--repo", recordSlug)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--pr")
}
