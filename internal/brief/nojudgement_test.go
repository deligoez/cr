package brief

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// judgementFiles are the §2.3 rows that hold a judgement: findings.ndjson holds
// §6.1's records, coverage.ndjson holds §4.5.5's cells, and probes.ndjson holds
// §5's runs. §3.7 has `cr brief` create none of the three, and the file names
// are read from internal/state rather than spelled here so a renamed row cannot
// leave this guard watching a path nothing writes to.
var judgementFiles = []string{state.FileFindings, state.FileCoverage, state.FileProbes}

// derivedFiles are the rows §3.7 permits `cr brief` to persist: meta.json's
// round and recorded head, units.ndjson, and threads.ndjson. They are facts
// about the pull request rather than opinions about it, and §4.6 needs the same
// unit set, so a brief is allowed to differ here and nowhere else.
var derivedFiles = []string{state.FileMeta, state.FileUnits, state.FileThreads}

// calls records every gh invocation a run made, and passes each one on.
//
// It wraps the canned runner rather than replacing it, so the brief still gets
// the payloads it needs and the invocations it asked for are readable
// afterwards. gh.WithRunner substitutes the transport, which is what makes the
// record complete: in production the same invocations go through gh.Run, whose
// boundary refuses everything it cannot recognise as a read.
type calls struct {
	args [][]string
}

func (c *calls) through(next gh.Runner) gh.Runner {
	return func(args ...string) (string, error) {
		c.args = append(c.args, slices.Clone(args))
		return next(args...)
	}
}

// rows reads every §2.3 row of the pull request's state directory.
func rows(t *testing.T, src *Sources) map[string]string {
	t.Helper()
	out := make(map[string]string, len(state.PRFiles()))
	for _, name := range state.PRFiles() {
		body, err := os.ReadFile(src.Layout.PRFile(testOwner, testRepo, testPR, name))
		require.NoError(t, err, name)
		out[name] = string(body)
	}
	return out
}

// `cr brief` writes the derived inputs of §3.3 through §3.6 and leaves every
// other row of the §2.3 table exactly as it found it.
//
// This is brief-no-judgement's third criterion, widened by one step for the
// reason the criterion names the three files at all: §3.7 says `cr brief`
// creates no finding, coverage cell, probe, or thread, and a test that only
// read findings.ndjson, coverage.ndjson and probes.ndjson would pass a brief
// that had started writing mapping.ndjson or transitions.ndjson instead. So the
// whole table is compared and the permitted rows are the exception, which makes
// a new write a failure by default rather than by somebody remembering to add a
// line here.
//
// The sentinel matters as much as the emptiness. A first brief leaves the three
// rows empty, which a brief that truncated them would also do; a second brief
// over a seeded file is what tells the two apart.
func TestABriefWritesTheDerivedInputsAndNoJudgementArtefact(t *testing.T) {
	dir, head, base := repository(t)
	seen := &calls{}
	src := sources(t, dir, seen.through(answering(head, base, oneThread)))

	_, err := Run(src)
	require.NoError(t, err)

	opened := rows(t, src)
	for _, name := range judgementFiles {
		assert.Emptyf(t, opened[name],
			"§3.7: a brief creates no record in %s, so the row it opens holds none", name)
	}

	// Seeded through the §2.3.1 lock, which is the only way per-PR state is
	// written, so the fixture cannot be a write cr itself could not make.
	held, err := src.Layout.LockPR(testOwner, testRepo, testPR)
	require.NoError(t, err)
	sentinel := make(map[string]string, len(judgementFiles))
	for i, name := range judgementFiles {
		line := `{"id":"seeded-` + strconv.Itoa(i+1) + `"}` + "\n"
		require.NoError(t, held.Write(name, []byte(line)), name)
		sentinel[name] = line
	}
	require.NoError(t, held.Unlock())

	before := rows(t, src)
	// The head has not moved, so §9.3.3 makes this second brief idempotent
	// and every difference below is a write the brief chose to make.
	_, err = Run(src)
	require.NoError(t, err)
	after := rows(t, src)

	for _, name := range judgementFiles {
		assert.Equalf(t, sentinel[name], after[name],
			"§3.7: a brief neither adds to nor clears %s", name)
	}
	for _, name := range state.PRFiles() {
		if slices.Contains(derivedFiles, name) {
			continue
		}
		assert.Equalf(t, before[name], after[name],
			"§3.7 permits a brief the derived inputs of §3.3 to §3.6, and %s is not one of them", name)
	}
}

// Every gh invocation a brief makes is a read.
//
// §3.7's first half is that `cr brief` performs no network write, and the
// enforcement lives in internal/gh: Run is a read door closed by default, and
// nowrite_test.go asserts that nothing outside that package can mint the
// Confirmation the write door needs. What that argument cannot show is what
// this command asks for, because gh.WithRunner replaces the door in every test
// here — so the invocations are recorded and judged.
//
// The conditions below are strictly narrower than the boundary's own: an
// invocation that passes them is one the boundary admits, so this can refuse
// something internal/gh would allow and can never admit something it refuses.
// That is the safe direction for a guard restating a rule it does not own.
func TestABriefAsksGitHubForNothingButReads(t *testing.T) {
	dir, head, base := repository(t)
	seen := &calls{}
	src := sources(t, dir, seen.through(answering(head, base, oneThread)))

	_, err := Run(src)
	require.NoError(t, err)

	require.NotEmpty(t, seen.args,
		"a brief that reached GitHub not at all would say nothing about what it asks for")
	for _, argv := range seen.args {
		joined := strings.Join(argv, " ")
		require.Equalf(t, "api", argv[0],
			"§2.1.2: `gh api` is the whole of what cr reads with, and this run used %q", joined)
		for _, writes := range []string{"mutation", "subscription", "--method", "-X", "--input"} {
			assert.NotContainsf(t, joined, writes,
				"§3.7: a brief performs no network write, and %q names %s", joined, writes)
		}
	}
}
