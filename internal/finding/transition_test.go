package finding

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// creationCell is §9.1's first From cell, `— (new record)`, spelled as the
// spec spells it so the transcription below reads as the table does.
const creationCell = "— (new record)"

// specTransitions is §9.1's transition table with its multi-value cells
// expanded, transcribed from the spec by hand. It is written out here rather
// than read from `table`, because a test that read the implementation's rows
// would assert only that the code agrees with itself.
var specTransitions = []struct{ from, to, by string }{
	{creationCell, "draft", "cr record"},
	{"draft", "duplicate", "cr record"},
	{"draft", "suppressed", "cr record"},
	{"draft", "queued", "cr draft"},
	{"queued", "discarded", "cr draft"},
	{"queued", "discarded", "cr post --confirm"},
	{"queued", "posted", "cr post --confirm"},
	{"queued", "posted", "cr post --reconcile"},
	{"draft", "stale", "cr brief"},
	{"queued", "stale", "cr brief"},
}

// specActors is §9.1's third column, deduplicated. `cr merge` is not in it, and
// the sentence under the table says why: it runs on role output files before
// any record exists.
var specActors = []string{
	"cr record",
	"cr draft",
	"cr post --confirm",
	"cr post --reconcile",
	"cr brief",
}

// fromNamed resolves a From cell of §9.1's table.
func fromNamed(t *testing.T, cell string) From {
	t.Helper()
	if cell == creationCell {
		return Creation
	}
	parsed, err := ParseState(cell)
	require.NoError(t, err)
	return Existing(parsed)
}

// stateNamed resolves a To cell of §9.1's table.
func stateNamed(t *testing.T, cell string) State {
	t.Helper()
	parsed, err := ParseState(cell)
	require.NoError(t, err)
	return parsed
}

// actorNamed resolves a Command cell of §9.1's table, failing when the package
// has no actor going by that command line.
func actorNamed(t *testing.T, cell string) Actor {
	t.Helper()
	for _, actor := range Actors() {
		if actor.String() == cell {
			return actor
		}
	}
	require.FailNow(t, "§9.1 names a command that is no actor here", cell)
	return Actor{}
}

// actorNames renders a set of actors as the command lines §9.1's third column
// writes, so a failure names the command.
func actorNames(set []Actor) []string {
	rendered := make([]string, 0, len(set))
	for _, actor := range set {
		rendered = append(rendered, actor.String())
	}
	return rendered
}

// §9.1 opens with "Transitions are exhaustive", and that word is load-bearing
// in two directions at once: every cell of the table must be permitted, and
// everything else must be refused, because a transition not listed MUST be
// rejected with exit code 4.
//
// So the assertion is the whole cross product rather than a sample of it —
// every From the column can hold against every state and every actor, plus the
// two zero values that no row names. A rule that only checked the listed cells
// would pass on a decision that allowed everything, and that is precisely the
// failure §9.1's sentence exists to prevent.
//
// The two zero values are in the product deliberately. The zero Actor is a
// command that never said who it was, and the zero From is a record in no §9.1
// state — the state every unstamped Finding carries. Both are refused by being
// in no row, which is what closed-by-default means here: nothing had to
// remember to reject them.
func TestEveryTransitionIsTheTableAndNothingElse(t *testing.T) {
	listed := make(map[move]bool, len(specTransitions))
	for _, cell := range specTransitions {
		listed[move{
			from:  fromNamed(t, cell.from),
			to:    stateNamed(t, cell.to),
			actor: actorNamed(t, cell.by),
		}] = true
	}
	require.Len(t, listed, len(specTransitions), "the transcription repeats a cell")

	froms := []From{Creation, {}}
	for _, current := range States() {
		froms = append(froms, Existing(current))
	}
	tos := append(States(), State{})
	bys := append(Actors(), Actor{})

	permitted := 0
	for _, from := range froms {
		for _, to := range tos {
			for _, by := range bys {
				asked := move{from: from, to: to, actor: by}
				err := MayTransition("f1", from, to, by)
				if listed[asked] {
					permitted++
					require.NoError(t, err, "§9.1 lists %s → %s by %s", from, to, by)
					continue
				}
				var illegal *IllegalTransitionError
				require.ErrorAs(t, err, &illegal, "§9.1 does not list %s → %s by %s", from, to, by)
			}
		}
	}
	assert.Equal(t, len(specTransitions), permitted, "a listed cell was never reached by the product")
}

// §9.1 fixes what the refusal says as tightly as it fixes that there is one:
// naming the record and its current state. The record id cannot be derived
// from the states, and a message that named neither would leave a user with a
// refusal and no way to find what was refused.
//
// The hint is §12.4's: every error names the next actionable step, and the step
// out of an illegal transition is whichever of §9.1's rows does apply to the
// state the record is actually in. A terminal record has none, and saying so is
// the actionable answer — v0.1 ends at posting per §9, so there is nothing to
// go and do.
func TestARefusalNamesTheRecordAndItsCurrentState(t *testing.T) {
	queued := MayTransition("f7", Existing(StateQueued), StateDraft, ActorRecord)
	require.Error(t, queued)
	assert.Contains(t, queued.Error(), "f7", "the refusal names the record")
	assert.Contains(t, queued.Error(), "queued", "and its current state")
	assert.Contains(t, queued.Error(), "cr record", "and who asked")
	assert.Contains(t, queued.Error(), "posted by cr post --confirm", "§12.4: what may be done instead")

	posted := MayTransition("f7", Existing(StatePosted), StateQueued, ActorDraft)
	require.Error(t, posted)
	assert.Contains(t, posted.Error(), "f7 is posted")
	assert.Contains(t, posted.Error(), "§9.1 lists no move out of it",
		"v0.1 ends at posting, so the honest next step is that there is none")

	fresh := MayTransition("f7", Creation, StateQueued, ActorDraft)
	require.Error(t, fresh)
	assert.Contains(t, fresh.Error(), "f7 is a new record",
		"§9.1's first row has no state to name, so the refusal says what the record is instead")

	unstamped := MayTransition("f7", From{}, StateQueued, ActorDraft)
	require.Error(t, unstamped)
	assert.Contains(t, unstamped.Error(), "f7 is in no §9.1 state",
		"the zero From is not Creation: a record whose state did not read is not a new record")

	var illegal *IllegalTransitionError
	require.ErrorAs(t, queued, &illegal)
	assert.Equal(t, "f7", illegal.Record, "the id reaches a caller as a field, not only as text")
	assert.Equal(t, Existing(StateQueued), illegal.From)
	assert.Equal(t, StateDraft, illegal.To)
	assert.Equal(t, ActorRecord, illegal.Actor)
}

