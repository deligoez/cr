package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// runPost runs `cr post` with args against whatever CR_HOME points at, and
// returns what it printed and what it refused.
func runPost(t *testing.T, args ...string) (printed string, err error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"post"}, args...))
	err = cmd.Execute()
	return out.String(), err
}

// built is what a `cr post` run reported about the payload it assembled.
type built struct {
	Round    int             `json:"round"`
	Comments []postedComment `json:"comments"`
}

// postedReview runs `cr post` and decodes the payload it reported.
func postedReview(t *testing.T) built {
	t.Helper()
	printed, err := runPost(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)
	var got built
	require.NoError(t, json.Unmarshal([]byte(printed), &got))
	return got
}

// replaceMapping replaces the round's mapping.ndjson with the pairs given, which
// is what §4.1.6 lets `cr map record` do inside a round.
func replaceMapping(t *testing.T, layout state.Layout, pairs string) {
	t.Helper()
	held, err := layout.LockPR(draftOwner, draftRepo, draftPRNum)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileMapping, []byte(pairs)))
	require.NoError(t, held.Unlock())
}

// §7.2.2: `cr post` recomputes every record's grade before it reads the draft,
// so no draft edit can turn an `argued` record into a posted assertion.
//
// The two runs are the assertion. §7.2's `kind` row admits a hardening on "the
// recomputed grade", and the record here was stored graded `cited` while
// carrying no citation at all — the state §6.2.1's inputs moving inside a round
// leaves behind. `cr draft` reads the marker against the grade the round holds
// and admits it; `cr post` recomputes first, reaches `argued`, and refuses.
// Without the recomputation the second run would admit it too, and the author
// would receive as an assertion a record with nothing behind it.
func TestPostRecomputesTheGradeBeforeItReadsTheMarker(t *testing.T) {
	asked := aStoredRecord("f1", finding.StateDraft)
	asked.Kind = finding.KindQuestion
	asked.Summary = "Does the caller ever see the error Decode returns?"
	layout := draftedHome(t, asked)
	redraft(t)
	writeDraft(t, layout,
		markerEdit(t, readDraft(t, layout), "f1", `kind="question"`, `kind="finding"`))

	_, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err, "cr draft reads the marker against the grade the round recorded")

	_, err = runPost(t, draftPR, "--repo", draftSlug)

	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.3.3 refuses with exit code 1")
	assert.Contains(t, err.Error(), "f1", "naming the record id")
	assert.Contains(t, err.Error(), string(finding.GradeArgued),
		"§7.2.2: the grade the marker is read against is the recomputed one")
}

// aCitedRecord is a stored record carrying one citation outside its own unit,
// so §6.2's `cited` row still holds for it when §7.2.2 recomputes the grade.
//
// The fixtures of draft_test.go carry a grade and no citation, which is the
// state a round leaves behind when §6.2.1's inputs move after recording. That is
// exactly what the recomputation is for, so a test about anything else has to
// supply the evidence the grade rests on.
func aCitedRecord(id string) *finding.Finding {
	record := aStoredRecord(id, finding.StateDraft)
	record.Citations = []finding.Citation{{
		Path: "internal/api/decode.go", Line: 12,
		ContentHash: "0123456789abcdef", Origin: finding.OriginAgent,
	}}
	return record
}

// anIntentFinding is that record on the intent axis, claiming CR-7#c1.
// §6.3 has no reason to move it, so anything that makes it a question is
// §4.1.4's doing and nothing else's.
func anIntentFinding() *finding.Finding {
	record := aCitedRecord("f1")
	record.Axis, record.Role = axis.Intent, "intent"
	record.Claim = "CR-7#c1"
	return record
}

// asked is the agent's rewrite of a body into a question, made by editing
// draft.md — the one input path §8.1.2 leaves for reader-facing prose, and what
// §8.1.5 requires of a body that reaches the author as a question.
const asked = "Does the change implement CR-7#c1?"

// §4.1.4 re-applied after record time, carried here from
// intent-finding-on-unmapped-unit: the forcing runs at `cr draft` and `cr post`
// as well, so a mapping that changes inside a round cannot leave a recorded
// intent finding asserting against a claim the round no longer maps to it.
//
// The record is written as a finding while u1 is mapped, which is the state
// `cr record` leaves behind — §4.1.4 admitted it there. The mapping is then
// cleared, as §4.1.6 lets `cr map record` do, and both later commands are asked
// again.
func TestClearingTheMappingMakesARecordedIntentFindingAQuestion(t *testing.T) {
	record := anIntentFinding()
	layout := draftedHome(t, record)
	coveredRound(t, layout)
	mapped := `{"claim":"CR-7#c1","unit":"u1","head":"` + draftHead + `","round":2}` + "\n"
	replaceMapping(t, layout, mapped)

	redraft(t)
	writeDraft(t, layout, strings.Replace(readDraft(t, layout), record.Summary, asked, 1))
	redraft(t)
	assert.Contains(t, readDraft(t, layout), `id="f1" kind="finding"`,
		"while u1 is mapped, §4.1.4 admits the finding")
	assert.Equal(t, []postedComment{{ID: "f1", Kind: finding.KindFinding}}, postedReview(t).Comments)

	replaceMapping(t, layout, "")

	redraft(t)
	assert.Contains(t, readDraft(t, layout), `id="f1" kind="question"`,
		"§4.1.4 at draft time: the unit maps to no claim, so the finding is held as a question")
	assert.Equal(t, []postedComment{{ID: "f1", Kind: finding.KindQuestion}}, postedReview(t).Comments,
		"§4.1.4 at post time, over the payload the author would actually receive")
}

// `cr post` builds the payload out of what the draft left queued: a block the
// reviewer deleted is not a comment, and a record the round never queued is not
// one either. Nothing of the run reaches disk.
func TestPostBuildsThePayloadFromWhatTheDraftLeftQueued(t *testing.T) {
	duplicate := aCitedRecord("f3")
	duplicate.State = finding.StateDuplicate
	layout := draftedHome(t, aCitedRecord("f1"), aCitedRecord("f2"), duplicate)
	redraft(t)
	before := draftedFindings(t, layout)
	writeDraft(t, layout, deleteBlock(t, readDraft(t, layout), "f2"))

	review := postedReview(t)

	assert.Equal(t, draftRound, review.Round)
	assert.Equal(t, []postedComment{{ID: "f1", Kind: finding.KindFinding}}, review.Comments,
		"the deleted block is no comment, and a duplicate was never queued")
	assert.Equal(t, before, draftedFindings(t, layout),
		"§7.3.1: a run without --confirm changes nothing, so the discard is cr draft's to write")
}
