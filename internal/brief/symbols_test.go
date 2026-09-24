package brief

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/reinvention"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/symbol"
	"github.com/deligoez/cr/internal/unit"
)

// longFunction builds a repository whose change edits the first and the last
// statement of one twenty-two-line Go function, nineteen lines apart: further
// than `cluster.gap_lines`' default of 12, so adjacency alone splits them, and
// far enough apart that git's three lines of context leave two hunks.
func longFunction(t *testing.T) (dir, head, base string) {
	t.Helper()
	dir = t.TempDir()
	body := func(first, last int) string {
		var src strings.Builder
		src.WriteString("package shop\n\nfunc Total() int {\n\tsum := 0\n")
		for n := 1; n <= 20; n++ {
			value := n
			switch n {
			case 1:
				value = first
			case 20:
				value = last
			}
			fmt.Fprintf(&src, "\tsum += %d\n", value)
		}
		src.WriteString("\treturn sum\n}\n")
		return src.String()
	}
	write := func(text string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "order.go"), []byte(text), 0o600))
	}
	runGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write(body(1, 20))
	runGit(t, dir, "add", "order.go")
	runGit(t, dir, "commit", "--quiet", "-m", "the function before the change")
	base = runGit(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "checkout", "--quiet", "-b", "feature/"+testIssue+"-total")
	write(body(100, 200))
	runGit(t, dir, "commit", "--quiet", "-am", "weigh the first and the last item")
	head = runGit(t, dir, "rev-parse", "HEAD")
	return dir, head, base
}

// briefedUnits runs `cr brief`'s package over longFunction under one profile
// and returns the units it recorded.
func briefedUnits(t *testing.T, profileID, profileJSON string) []unit.Record {
	t.Helper()
	dir, head, base := longFunction(t)
	src := sources(t, dir, answering(head, base, noThreads))
	require.NoError(t, src.Layout.EnsureProfile(profileID, profileJSON))
	resolved, err := config.Resolve(config.Sources{Flags: map[string]any{"profile": profileID}})
	require.NoError(t, err)
	src.Config = resolved

	_, err = Run(src)
	require.NoError(t, err)
	stored, err := state.ReadRecords[unit.Record](src.Layout, testOwner, testRepo, testPR, state.FileUnits)
	require.NoError(t, err)
	return stored
}

// §3.4.4's symbol branch through `cr brief`: two hunks inside one function,
// further apart than adjacency joins, are one unit formed by symbol when the
// profile declares a language cr indexes — because cr brief now hands §4.3.1's
// head index to clustering — and two units formed by adjacency when it does
// not, which is §3.4.3's fallthrough.
func TestTwoHunksInsideOneFunctionAreOneUnitBySymbol(t *testing.T) {
	const axes = `"axes":{"intent":true,"correctness":true,"convention":true,"test":true}`

	indexed := briefedUnits(t, "shop",
		`{"id":"shop","match":{"files":[],"globs":["**/*.go"]},`+axes+`,"symbols":{"lang":"go"}}`)
	require.Len(t, indexed, 1, "both hunks sit inside Total, lines 3 to 26")
	assert.Equal(t, unit.BySymbol, indexed[0].Formation)
	assert.Equal(t, []unit.Range{{Start: 2, End: 8}, {Start: 21, End: 26}}, indexed[0].HunkRanges)

	plain := briefedUnits(t, "plain", `{"id":"plain","match":{"files":[],"globs":["**/*.go"]},`+axes+`}`)
	require.Len(t, plain, 2, "with no symbols.lang the same diff clusters by adjacency alone")
	for i := range plain {
		assert.Equal(t, unit.ByAdjacency, plain[i].Formation)
	}
}

// §4.5.4 over a pull request that deletes a test file under a profile whose
// reinvention half ran: the symbol half cannot read that file at the head and
// says so, and the halves report that one lens and nothing else.
func TestHalvesReportAnUnreadTestFileBesideAReinventionHalfThatRan(t *testing.T) {
	dir, head, _ := repository(t)
	p := &profile.Profile{ID: "shop", Symbols: profile.Symbols{Lang: "go"},
		Tests: profile.Tests{Globs: []string{"*_test.go"}}}
	deleted := []git.Hunk{{Path: "order_test.go", BaseStart: 1, BaseLines: 3, Side: git.Left}}

	halves, err := Halves(dir, head, p, &symbol.Index{Lang: "go"}, deleted, nil, reinvention.Ranking{})
	require.NoError(t, err)

	require.Len(t, halves, 1)
	assert.Contains(t, halves[0].Disclosure(), "cr could not read order_test.go at the head")
}
