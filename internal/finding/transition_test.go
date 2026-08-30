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

