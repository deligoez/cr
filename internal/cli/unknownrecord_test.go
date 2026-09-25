package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §9.5.5, §9.6.1 and §9.6.2 name a record by id across every round, so an id
// no round holds is refused by each of the three commands with exit 1, naming
// the id and sending the reader to the listings that hold the ids there are,
// and nothing is written.
func TestARecordIDNoRoundHoldsIsRefusedByEveryCommandThatSettlesOne(t *testing.T) {
	for name, args := range map[string][]string{
		"verify":   {"verify", fixturePR, "f9", "answered", "--evidence", "the author replied"},
		"resolve":  {"resolve", fixturePR, "f9"},
		"withdraw": {"withdraw", fixturePR, "f9", "wrong"},
	} {
		t.Run(name, func(t *testing.T) {
			statusHome(t)
			layout, err := state.Default()
			require.NoError(t, err)
			meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
			require.NoError(t, err)
			holdRecords(t, layout, fixtureOwner, fixtureProject, fixturePRNumber,
				`{"id":"f3","kind":"question","summary":"why is the retry unbounded?","state":"posted",`+
					`"thread_id":"PRRT_q","head":"`+meta.Head+`","round":1}`)
			findings := layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
			before, err := os.ReadFile(findings)
			require.NoError(t, err)

			_, err = runCLIPrinting(t, append(args, "--repo", fixtureSlug)...)

			require.Error(t, err)
			assert.Equal(t, "no record f9 is stored for this pull request, in any round", err.Error())
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, "run cr recheck to list the posted records, or cr status for the current round's",
				hintFor(err))
			after, err := os.ReadFile(findings)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after), "a refused command writes nothing")
		})
	}
}
