package review

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// fillEvery stores a pass cell for every cell of the fan-out's expected set,
// at its round and head, as `cr cells record` would.
func fillEvery(t *testing.T, src *Sources, fan *Fanout) {
	t.Helper()
	cells := make([]*coverage.Cell, 0, len(fan.Expected))
	for _, at := range fan.Expected {
		cells = append(cells, &coverage.Cell{Unit: at.Unit, Role: at.Role, Result: coverage.ResultPass, UnitHash: "h"})
	}
	held, err := src.Layout.LockPR(runOwner, runRepo, runPR)
	require.NoError(t, err)
	require.NoError(t, state.AppendStamped(held, state.FileCoverage, state.Stamp{Head: fan.Head, Round: fan.Round}, cells))
	require.NoError(t, held.Unlock())
}

// minutesAfter is a clock reading base plus the minutes given.
func minutesAfter(base time.Time, minutes int) func() time.Time {
	return func() time.Time { return base.Add(time.Duration(minutes) * time.Minute) }
}

// marks is every expected cell as unit/role with its two marks.
func marks(fan *Fanout) map[string][2]bool {
	out := make(map[string][2]bool, len(fan.Expected))
	for _, cell := range fan.Expected {
		out[cell.Unit+"/"+cell.Role] = [2]bool{cell.Recorded, cell.StaleByNote}
	}
	return out
}

// everyHeld is the same pair of marks on every cell of the fan-out's set, each
// held by the round and stale by note or not.
func everyHeld(fan *Fanout, stale bool) map[string][2]bool {
	out := make(map[string][2]bool, len(fan.Expected))
	for _, cell := range fan.Expected {
		out[cell.Unit+"/"+cell.Role] = [2]bool{true, stale}
	}
	return out
}

// §4.6.1's default narrowing over a round whose every cell is held: nothing is
// emitted until a standing note with no link is recorded after the cells'
// latest emission, then every held cell is emitted again and marked
// `stale_by_note`, and once those prompts carried the note nothing is emitted
// again. A note retracted before the next run makes nothing stale.
func TestAHeldCellIsEmittedAgainOnlyWhileAStandingNotePostdatesItsLatestEmission(t *testing.T) {
	src := briefed(t)
	base := time.Now().Add(time.Hour)
	src.Clock = minutesAfter(base, 0)
	first, err := Run(src)
	require.NoError(t, err)
	require.Len(t, first.Prompts, 8, "the control: four active roles over two units, none held")
	fillEvery(t, src, first)

	src.Clock = minutesAfter(base, 1)
	quiet, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, []string{}, cellsOf(quiet.Prompts), "every cell is held and no note postdates a prompt")
	assert.Equal(t, everyHeld(first, false), marks(quiet))

	_, err = note.Append(src.Layout, runIssue, "an unlinked fact", note.SourceChat, runPR, base.Add(2*time.Minute))
	require.NoError(t, err)
	src.Clock = minutesAfter(base, 3)
	stale, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, cellsOf(first.Prompts), cellsOf(stale.Prompts), "a note on every unit makes every held cell stale")
	assert.Equal(t, everyHeld(first, true), marks(stale))

	src.Clock = minutesAfter(base, 4)
	carried, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, []string{}, cellsOf(carried.Prompts), "the re-emitted prompts carried the note")
	assert.Equal(t, everyHeld(first, false), marks(carried))

	retracted, err := note.Append(src.Layout, runIssue, "a fact taken back", note.SourceChat, runPR, base.Add(5*time.Minute))
	require.NoError(t, err)
	_, err = note.Retract(src.Layout, retracted.ID, base.Add(6*time.Minute))
	require.NoError(t, err)
	src.Clock = minutesAfter(base, 7)
	withdrawn, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, []string{}, cellsOf(withdrawn.Prompts), "a retracted note is not a standing one")
}

// §4.6.1's note placement: a note a claim was drawn from is on the units that
// claim is mapped to, and a note answering a record is on that record's unit.
// Each makes only its own unit's held cells stale.
func TestANoteIsOnTheUnitsItsClaimIsMappedToOrTheUnitOfTheRecordItAnswers(t *testing.T) {
	src := briefed(t)
	base := time.Now().Add(time.Hour)
	src.Clock = minutesAfter(base, 0)
	first, err := Run(src)
	require.NoError(t, err)
	fillEvery(t, src, first)
	meta, err := src.Layout.ReadMeta(runOwner, runRepo, runPR)
	require.NoError(t, err)
	stamp := state.Stamp{Head: meta.Head, Round: 1}

	drawn, err := note.Append(src.Layout, runIssue, "shipping is waived for staff", note.SourceChat, runPR,
		base.Add(time.Minute))
	require.NoError(t, err)
	held, err := src.Layout.LockPR(runOwner, runRepo, runPR)
	require.NoError(t, err)
	require.NoError(t, state.ReplaceStamped(held, state.FileClaims, stamp, []*intent.Claim{
		{ID: runIssue + "#c1", Text: "The total sums the subtotal and the shipping.",
			Source: intent.ClaimFromAcceptance, Span: "The total sums the subtotal and the shipping."},
		{ID: runIssue + "#c2", Text: "Staff pay no shipping.", Source: intent.ClaimFromNote,
			NoteID: drawn.ID, Span: drawn.Text},
	}))
	require.NoError(t, held.Unlock())
	storeMapping(t, src, meta.Head, []*mapping.Pair{
		{Claim: runIssue + "#c1", Unit: "u1"}, {Claim: runIssue + "#c2", Unit: "u2"},
	})

	src.Clock = minutesAfter(base, 2)
	claimNote, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, []string{"convention/u2", "correctness/u2", "intent-coverage/u2", "test-adequacy/u2"},
		cellsOf(claimNote.Prompts), "the note is on u2, the unit its claim is mapped to")

	held, err = src.Layout.LockPR(runOwner, runRepo, runPR)
	require.NoError(t, err)
	require.NoError(t, state.AppendStamped(held, state.FileFindings, stamp, []*finding.Finding{{
		ID: "f1", Kind: finding.KindQuestion, Role: "correctness", Unit: "u1", Class: "unchecked-error",
	}}))
	require.NoError(t, held.Unlock())
	_, err = note.Answer(src.Layout, runOwner, runRepo, runPR, "f1", "it is logged upstream", note.SourceChat,
		base.Add(3*time.Minute))
	require.NoError(t, err)

	src.Clock = minutesAfter(base, 4)
	answer, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, []string{"convention/u1", "correctness/u1", "intent-coverage/u1", "test-adequacy/u1"},
		cellsOf(answer.Prompts), "the answer is on u1, the unit of the record it answers")
	want := everyHeld(first, false)
	for _, cell := range []string{"u1/convention", "u1/correctness", "u1/intent-coverage", "u1/test-adequacy"} {
		want[cell] = [2]bool{true, true}
	}
	assert.Equal(t, want, marks(answer))
}

// `--units` and `--shard` pick units by §4.6.1's partition: position i of m
// units in ascending numeric order of id belongs to shard floor(i×n/m)+1. u10
// sorts after u3, so a partition by text would put it in the first shard.
func TestUnitsAndShardPickUnitsInNumericOrder(t *testing.T) {
	ids := []string{"u1", "u10", "u2", "u3"}
	for name, c := range map[string]struct {
		src  Sources
		want []string
	}{
		"no flag":           {Sources{}, nil},
		"shard 1 of 2":      {Sources{Shard: &Shard{K: 1, N: 2}}, []string{"u1", "u2"}},
		"shard 2 of 2":      {Sources{Shard: &Shard{K: 2, N: 2}}, []string{"u3", "u10"}},
		"shard 1 of 3":      {Sources{Shard: &Shard{K: 1, N: 3}}, []string{"u1", "u2"}},
		"shard 2 of 3":      {Sources{Shard: &Shard{K: 2, N: 3}}, []string{"u3"}},
		"shard 3 of 3":      {Sources{Shard: &Shard{K: 3, N: 3}}, []string{"u10"}},
		"shard 5 of 5":      {Sources{Shard: &Shard{K: 5, N: 5}}, []string{}},
		"units as given":    {Sources{Units: []string{"u10", "u1"}}, []string{"u10", "u1"}},
		"shard over nobody": {Sources{Shard: &Shard{K: 1, N: 1}}, []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			given := ids
			if name == "shard over nobody" {
				given = []string{}
			}
			picked, err := c.src.pickUnits(1, given)
			require.NoError(t, err)
			assert.Equal(t, c.want, picked)
		})
	}

	_, err := (&Sources{Units: []string{"u1", "u4"}}).pickUnits(2, ids)
	assert.Equal(t, &UnknownUnitError{Unit: "u4", Round: 2, Units: []string{"u1", "u2", "u3", "u10"}}, err)
	assert.Equal(t, `--units names "u4", which is not a unit of round 2; §4.6.1 narrows the prompts to units `+
		"of the current round, which are u1, u2, u3, u10", err.Error())
}

// `--units` narrows the prompts of a run and leaves §4.6.3's expected set whole;
// an id that is no unit of the round is refused before anything is written.
func TestUnitsNarrowThePromptsAndLeaveTheExpectedSetWhole(t *testing.T) {
	src := briefed(t)
	src.Units = []string{"u9"}
	_, err := Run(src)
	assert.Equal(t, &UnknownUnitError{Unit: "u9", Round: 1, Units: []string{"u1", "u2"}}, err)
	_, statErr := os.Stat(src.Layout.PRFile(runOwner, runRepo, runPR, state.FileEmissions))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "a refused run emits nothing")
	_, statErr = os.Stat(src.Layout.FanOutDir(runOwner, runRepo, runPR, 1, "u1"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "and writes no fan-out")

	src.Units = []string{"u2"}
	narrowed, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, []string{"convention/u2", "correctness/u2", "intent-coverage/u2", "test-adequacy/u2"},
		cellsOf(narrowed.Prompts))
	assert.Len(t, narrowed.Expected, 8, "§4.6.3: every cell, whatever §4.6.1 narrowed")

	src.Units, src.Shard = nil, &Shard{K: 1, N: 2}
	shard, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, []string{"convention/u1", "correctness/u1", "intent-coverage/u1", "test-adequacy/u1"},
		cellsOf(shard.Prompts))
}

// §4.6.5's second intent pass is exempt from §4.6.1's default narrowing: the
// unit mapped to zero claims is re-emitted though its intent cell is held. It
// is not exempt from `--units`, which narrows it like every other pass.
func TestTheSecondIntentPassIgnoresTheDefaultNarrowingAndKeepsUnits(t *testing.T) {
	src := briefed(t)
	src.Axis = axis.Intent
	first, err := Run(src)
	require.NoError(t, err)
	fillEvery(t, src, first)

	held, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, []string{"intent-coverage/u2"}, cellsOf(held.Prompts), "the held unmapped unit is re-emitted")

	src.Units = []string{"u1"}
	other, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, []string{}, cellsOf(other.Prompts), "u1 is mapped, and --units left out u2")

	src.Units = []string{"u2"}
	named, err := Run(src)
	require.NoError(t, err)
	assert.Equal(t, []string{"intent-coverage/u2"}, cellsOf(named.Prompts))
}
