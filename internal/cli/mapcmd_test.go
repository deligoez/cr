package cli

import (
	"go/ast"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/state"
)

// The pull request the mapping tests run against.
const (
	mapOwner = "acme"
	mapRepo  = "api"
	mapSlug  = mapOwner + "/" + mapRepo
	mapPR    = 11
	mapHead  = "be7e2c75aeb661ba3c96d7a7634f3f8e84bb7b91"
	mapIssue = "CR-11"
)

// briefedForMapping writes the state a round leaves behind: three claims and
// three units, so §4.1.1's "zero or more" has room to be exercised in both
// directions — a unit mapped to two claims, and a unit mapped to none.
//
// It writes an earlier round alongside, because §9.3.5 scopes the replacement
// §4.1.6 performs and a fixture with one round could not tell a writer that
// replaces the round from one that replaces the file.
func briefedForMapping(t *testing.T) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(mapOwner, mapRepo, mapPR))

	held, err := layout.LockPR(mapOwner, mapRepo, mapPR)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: mapOwner, Repo: mapRepo, PR: mapPR,
		IssueKey: mapIssue, Round: 2, Head: mapHead,
	}))
	require.NoError(t, held.Write(state.FileUnits,
		[]byte(`{"id":"u1","path":"src/Order.php","head":"`+mapHead+`","round":2}`+"\n"+
			`{"id":"u2","path":"src/TaxRate.php","head":"`+mapHead+`","round":2}`+"\n"+
			`{"id":"u3","path":"src/Money.php","head":"`+mapHead+`","round":2}`+"\n")))
	require.NoError(t, held.Write(state.FileClaims,
		[]byte(`{"id":"`+mapIssue+`#c1","text":"a","source":"acceptance","span":"a",`+
			`"head":"`+mapHead+`","round":2}`+"\n"+
			`{"id":"`+mapIssue+`#c2","text":"b","source":"acceptance","span":"b",`+
			`"head":"`+mapHead+`","round":2}`+"\n"+
			`{"id":"`+mapIssue+`#c3","text":"c","source":"acceptance","span":"c",`+
			`"head":"`+mapHead+`","round":2}`+"\n")))
	// Round 1's mapping, which §9.3.5 calls history.
	require.NoError(t, held.Write(state.FileMapping,
		[]byte(`{"claim":"`+mapIssue+`#c1","unit":"u7","head":"0f1e2d3","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout
}

// recordMapping writes an NDJSON file outside the state tree and hands it to
// `cr map record`.
func recordMapping(t *testing.T, lines ...string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mapping.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return runCLI(t, "map", "record", strconv.Itoa(mapPR), path, "--repo", mapSlug)
}

// storedMapping is mapping.ndjson as `claim/unit` pairs with the round each one
// stands in, which is the shape §4.1.2 and §4.1.3 read it in.
func storedMapping(t *testing.T, l state.Layout) []string {
	t.Helper()
	stored, err := state.ReadRecords[mapping.Pair](l, mapOwner, mapRepo, mapPR, state.FileMapping)
	require.NoError(t, err)
	pairs := make([]string, 0, len(stored))
	for i := range stored {
		pairs = append(pairs,
			stored[i].Claim+"/"+stored[i].Unit+"@"+strconv.Itoa(stored[i].Round))
	}
	return pairs
}

// `cr map record` replaces the round's mapping with one {claim, unit} pair per
// line, and every unit is mapped to zero or more claims.
//
// §4.1.1's "zero or more" is asserted from both ends in one round, because both
// ends are silent failures. u1 carries two claims, which a writer keyed on the
// unit would collapse to one and §4.2.1 would then evaluate against half the
// intent it was meant to. u3 carries none, which is not an omission but the
// input §4.1.2 exists to read — a command that refused it, or filled it in,
// would take away the only way an unmapped unit can be stated.
//
// The second recording is what makes it a replacement rather than an append:
// the round's mapping afterwards is the second file's, whole, and the pair the
// first file held is gone. That is where §4.1.6 and §4.5.6 part company — a
// mapping is one judgement about the whole diff, so a pair the new file leaves
// out is a pair the agent withdrew.
func TestMapRecordReplacesTheRoundsMappingWithThePairsItNames(t *testing.T) {
	layout := briefedForMapping(t)

	require.NoError(t, recordMapping(t,
		`{"claim":"`+mapIssue+`#c1","unit":"u1"}`,
		`{"claim":"`+mapIssue+`#c2","unit":"u1"}`,
		`{"claim":"`+mapIssue+`#c3","unit":"u2"}`))

	assert.Equal(t, []string{
		mapIssue + "#c1/u7@1",
		mapIssue + "#c1/u1@2",
		mapIssue + "#c2/u1@2",
		mapIssue + "#c3/u2@2",
	}, storedMapping(t, layout),
		"§4.1.1: u1 carries two claims and u3 carries none, and §9.3.5 leaves round 1 alone")

	require.NoError(t, recordMapping(t, `{"claim":"`+mapIssue+`#c1","unit":"u2"}`))

	assert.Equal(t, []string{
		mapIssue + "#c1/u7@1",
		mapIssue + "#c1/u2@2",
	}, storedMapping(t, layout),
		"§4.1.6: the round's mapping is replaced by the file, and earlier rounds stay intact")
}

// §4.1.6 rejects an unknown claim or unit id with exit code 1, and the round's
// mapping is left exactly as it was.
//
// The two ids fail for the same reason from opposite sides. A pair naming a
// claim the round does not hold would have §4.1.3 report an unimplemented claim
// that something is mapped to; one naming a unit the round never formed would
// have §4.1.2 read a unit as mapped that §3.4 never produced. Either way the
// join points at a row that does not exist, and every rule in §4.1 through §4.4
// that speaks of the claims a unit is mapped to reads this file.
//
// The untouched mapping is the ordering the other recording commands already
// fix: the whole file is validated before anything is written, so a bad line
// among good ones does not cost the agent the mapping it had.
func TestAMapPairNamingAnUnknownIDExitsOneAndKeepsTheMapping(t *testing.T) {
	layout := briefedForMapping(t)
	require.NoError(t, recordMapping(t, `{"claim":"`+mapIssue+`#c1","unit":"u1"}`))
	standing := storedMapping(t, layout)

	for _, tc := range []struct {
		name  string
		line  string
		field string
	}{
		{"a claim no round recorded", `{"claim":"` + mapIssue + `#c9","unit":"u1"}`, "claim"},
		{"a claim of another issue", `{"claim":"CR-99#c1","unit":"u1"}`, "claim"},
		{"a unit no round formed", `{"claim":"` + mapIssue + `#c1","unit":"u9"}`, "unit"},
		{"no claim at all", `{"unit":"u1"}`, "claim"},
		{"no unit at all", `{"claim":"` + mapIssue + `#c1"}`, "unit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := recordMapping(t, `{"claim":"`+mapIssue+`#c2","unit":"u2"}`, tc.line)

			var rejected *mapping.RejectedPairError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, ExitValidation, exitCodeFor(err),
				"§4.1.6 and §11.2 code an unknown id 1")
			assert.Equal(t, tc.field, rejected.Field)
			assert.Equal(t, 2, rejected.Line,
				"the refusal names the line the user has to open")
			assert.Equal(t, standing, storedMapping(t, layout),
				"the good line above the bad one is not written either")
		})
	}
}

// joinFields are the two JSON keys that together make a claim-to-unit join.
// A struct carrying both is a second opinion about which claim covers which
// unit, whatever it is called.
var joinFields = []string{"claim", "unit"}

// joinsClaimToUnit reports the structs in one file that carry both keys.
func joinsClaimToUnit(file *ast.File) int {
	joins := 0
	ast.Inspect(file, func(n ast.Node) bool {
		structure, declared := n.(*ast.StructType)
		if !declared {
			return true
		}
		held := make(map[string]bool, len(joinFields))
		for _, field := range structure.Fields.List {
			if field.Tag == nil {
				continue
			}
			for _, key := range joinFields {
				if strings.Contains(field.Tag.Value, `json:"`+key+`"`) {
					held[key] = true
				}
			}
		}
		if held["claim"] && held["unit"] {
			joins++
		}
		return true
	})
	return joins
}

// mapping.ndjson is the only claim-to-unit join in cr.
//
// §4.1.6 ends with the sentence this guard is: "every rule in §4.1 through §4.4
// that speaks of the claims a unit is mapped to reads this file". §4.1.2 turns
// an unmapped unit into a question, §4.1.3 turns an unmapped claim into an
// intent gap, §4.2.1 evaluates a unit against the claims it is mapped to, and
// §4.4.1 attaches tests to the same units — four rules, all reading one join.
// A second structure carrying both ids is how they would come to disagree: the
// mapping the agent recorded and a mapping something else derived, with nothing
// in the run saying which one a report was built from.
//
// The claim is checked as "no other struct carries both keys", which is what a
// second join has to look like on disk. `claims` in the plural is a different
// field and a different rule — §6.1's record cites the claim it violates rather
// than joining anything — so the tag is matched exactly.
func TestTheMappingFileIsTheOnlyClaimToUnitJoin(t *testing.T) {
	joins := map[string]int{}
	eachSourceFile(t, func(rel string, file *ast.File) {
		if found := joinsClaimToUnit(file); found > 0 {
			joins[rel] = found
		}
	})

	assert.Equal(t, map[string]int{filepath.Join("internal", "mapping", "mapping.go"): 1}, joins,
		"§4.1.6: every rule in §4.1 through §4.4 reads one mapping, so cr holds one")
}
