package review

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/brief"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/observation"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// kindsProfile is shopProfile's axes with §4.6.7's changelog kind, read by the
// intent role alone.
const kindsProfile = `{"id":"shop","match":{"files":[],"globs":["**/*.go"]},` +
	`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},` +
	`"tests":{"cmd":["go","test"],"globs":["**/*_test.go"]},"symbols":{"lang":"go"},` +
	`"units":{"kinds":[{"kind":"changelog","globs":["changelogs/**"],"roles":["intent-coverage"]}]}}`

// eventTest is the event test both of the measured case's directories held,
// and change is the one edit both received.
const (
	eventTest = "package events\n\nfunc TestPlaced() {\n\tfire(\"placed\")\n\tassertSent()\n}\n"
	change    = "package events\n\nfunc TestPlaced() {\n\tfire(\"placed\", locale())\n\tassertSent()\n}\n"
)

// kindsRound opens round 1 over a change that adds a changelog entry and makes
// the identical edit to two event tests in sibling directories — the shape of
// the measured case — and records an empty mapping, so every axis may emit.
func kindsRound(t *testing.T) *Sources {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	runGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("orders/events_test.go", eventTest)
	write("refunds/events_test.go", eventTest)
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "--quiet", "-m", "the code before the change")
	base := runGit(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "checkout", "--quiet", "-b", "feature/"+runIssue)
	write("orders/events_test.go", change)
	write("refunds/events_test.go", change)
	write("changelogs/unreleased/locale.yml", "title: The event carries the locale\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "--quiet", "-m", "pass the locale")
	head := runGit(t, dir, "rev-parse", "HEAD")

	layout := state.New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsureProfile("shop", kindsProfile))
	cfg, err := config.Resolve(config.Sources{Flags: map[string]any{"profile": "shop"}})
	require.NoError(t, err)
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte("Every event carries the locale.\n"), 0o600))
	client := gh.WithRunner(func(args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "reviewThreads") {
			return `{"data":{"repository":{"pullRequest":{"reviewThreads":{` +
				`"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}}}}`, nil
		}
		return `{"data":{"repository":{"pullRequest":{"number":7,"title":"pass the locale","body":"",` +
			`"headRefName":"feature/` + runIssue + `","headRefOid":"` + head + `",` +
			`"baseRefName":"main","baseRefOid":"` + base + `"}}}}`, nil
	})
	_, err = brief.Run(&brief.Sources{
		Layout: layout, GH: client, Config: cfg, Owner: runOwner, Repo: runRepo, PR: runPR,
		RepoDir: dir, IssueFlag: runIssue, Intent: intent.Source{File: issue},
	})
	require.NoError(t, err)
	src := &Sources{Layout: layout, GH: client, Config: cfg, Owner: runOwner, Repo: runRepo, PR: runPR, RepoDir: dir}
	storeMapping(t, src, head, []*mapping.Pair{})
	return src
}

// unitAt is the round's unit on path.
func unitAt(t *testing.T, src *Sources, path string) unit.Unit {
	t.Helper()
	units := unitsOfRound(t, src)
	for i := range units {
		if units[i].Path == path {
			return units[i]
		}
	}
	require.Failf(t, "no unit", "the round formed no unit on %s", path)
	return unit.Unit{}
}

// §4.6.7: a unit every path of which matches a kind's globs is read only by the
// roles the kind lists. `cr review` emits the intent role's prompt over the
// changelog and no other, records every other active role's cell there as `na`
// with a reason naming the kind, and reports those cells recorded, so §10.2.2
// counts them.
func TestAUnitsKindDecidesWhichRolesReadIt(t *testing.T) {
	src := kindsRound(t)
	changelog := unitAt(t, src, "changelogs/unreleased/locale.yml").ID

	fan, err := Run(src)
	require.NoError(t, err)

	emitted := make([]string, 0)
	for i := range fan.Prompts {
		if fan.Prompts[i].Unit == changelog {
			emitted = append(emitted, fan.Prompts[i].Role)
		}
	}
	assert.Equal(t, []string{"intent-coverage"}, emitted, "§4.6.7: the kind's roles alone read the unit")

	cells, err := state.ReadStamped[coverage.Cell](src.Layout, runOwner, runRepo, runPR, state.FileCoverage, 1)
	require.NoError(t, err)
	na := make([]string, 0)
	for i := range cells {
		if cells[i].Unit != changelog {
			continue
		}
		assert.Equal(t, coverage.ResultNA, cells[i].Result)
		assert.Contains(t, cells[i].Reason, "unit kind changelog", "§4.6.7: the reason names the kind")
		assert.NotEmpty(t, cells[i].UnitHash, "the cell carries the unit hash §10.2.2 compares")
		na = append(na, cells[i].Role)
	}
	slices.Sort(na)
	assert.Equal(t, []string{"convention", "correctness", "test-adequacy"}, na)
	for _, cell := range fan.Expected {
		if cell.Unit == changelog && cell.Role != "intent-coverage" {
			assert.True(t, cell.Recorded, "§4.6.7: the kind's cell counts toward §10.2.2 as any other")
		}
	}

	src.All = true
	again, err := Run(src)
	require.NoError(t, err)
	for i := range again.Prompts {
		if again.Prompts[i].Unit == changelog {
			assert.Equal(t, "intent-coverage", again.Prompts[i].Role, "`--all` emits nothing the kind withholds")
		}
	}
}

