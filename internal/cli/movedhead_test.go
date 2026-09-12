package cli

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The one door onto a round, and the one read of meta.json that is not it.
//
// §9.3.1 binds "every command that reads per-PR state", which is a set nobody
// can keep by hand: the way it goes wrong is a command written next month that
// does its own read and is therefore in the set without anybody noticing. So
// the set is closed from the other end. state.Layout.Briefed is the only way to
// obtain a round, it now demands a state.CurrentHead the compiler will not let
// a caller omit, and the callers of both it and the raw ReadMeta beneath it are
// read out of the source here rather than trusted.
//
// ReadMeta is the half that matters. Briefed cannot be called without answering
// §9.3.1, so the escape is not calling it — and the four production readers
// below are each outside §9.3.1's reach for a reason of their own: Briefed
// itself, `cr brief`'s two, which §9.3.2 names as the way forward, and
// note.issueKeyOf, whose command answers §9.3.1 through answerHonesty instead.
func TestTheRoundHasOneDoorAndMetaJSONHasFourReaders(t *testing.T) {
	doors := map[string][]string{
		"Briefed": {
			"internal/cli/head.go: briefedRound",
			"internal/review/run.go: Run",
		},
		"ReadMeta": {
			"internal/brief/brief.go: roundOf",
			"internal/brief/store.go: metaOf",
			"internal/note/answer.go: issueKeyOf",
			"internal/state/briefed.go: Briefed",
		},
	}
	callers := map[string][]string{}
	eachSourceFile(t, func(rel string, file *ast.File) {
		for _, decl := range file.Decls {
			within, isFunc := decl.(*ast.FuncDecl)
			if !isFunc {
				continue
			}
			ast.Inspect(within, func(n ast.Node) bool {
				call, isCall := n.(*ast.CallExpr)
				if isCall && doors[calledName(call)] != nil {
					name := calledName(call)
					callers[name] = append(callers[name],
						filepath.ToSlash(rel)+": "+within.Name.Name)
				}
				return true
			})
		}
	})

	for door, expected := range doors {
		found := callers[door]
		slices.Sort(found)
		assert.Equalf(t, expected, found,
			"§9.3.1 binds every command that reads per-PR state, and %s is reached from somewhere new",
			door)
	}
}

// Every place that holds a round says what it does about §9.3, and the
// alternatives are the section's own two.
//
// A round now carries the comparison, so the way §9.3 gets broken is no longer
// a command that forgets to compare — it is a command that compares and then
// ignores the answer. That has a shape a walk can see: a file that reaches
// briefedRound or Briefed and never names RefuseStale (§9.3.2's refusal) or
// Disclosure (§9.3.1's report). Neither answer is assumed correct here; what is
// asserted is that one of them was given, which is what a file added later
// cannot skip in silence.
// doors93 are the two files that are the door onto a round rather than callers
// of it.
var doors93 = map[string]bool{
	"internal/state/briefed.go": true,
	"internal/cli/head.go":      true,
}

func TestEveryFileHoldingARoundAnswersSection93(t *testing.T) {
	const (
		holds     = "briefedRound"
		alsoHolds = "Briefed"
		refuses   = "RefuseStale"
		discloses = "Disclosure"
	)
	answered, holders := 0, 0
	eachSourceFile(t, func(rel string, file *ast.File) {
		names := map[string]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			if ident, isIdent := n.(*ast.Ident); isIdent {
				names[ident.Name] = true
			}
			return true
		})
		// The two files that are the door rather than a caller of it:
		// state/briefed.go declares all four names, and cli/head.go is
		// the wrapper every command under internal/cli comes through.
		// Either answering §9.3 for itself would prove nothing about a
		// caller, and TestTheRoundHasOneDoorAndMetaJSONHasFourReaders
		// is what keeps the exemption from covering anything else.
		if doors93[filepath.ToSlash(rel)] {
			return
		}
		if !names[holds] && !names[alsoHolds] {
			return
		}
		holders++
		if assert.Truef(t, names[refuses] || names[discloses],
			"%s holds a round and answers §9.3 neither way: refuse under §9.3.2 or disclose under §9.3.1",
			rel) {
			answered++
		}
	})

	require.Greater(t, holders, 5,
		"only %d files were found to hold a round, so this guard measured almost nothing", holders)
	assert.Equal(t, holders, answered)
}

// §11.1 names the stale-round report among the seven `--quiet` may never
// suppress, and finding.HonestyDisclosure is the shape the writer holding that
// exemption consumes. quiet-honesty-exemptions builds that writer; implementing
// the contract now is what lets it exempt this report by consuming one
// interface rather than by learning a seventh special case.
//
// The refusal is required to carry the same sentence, because §11.1 and §9.3.2
// are one fact told twice: a reader who is refused and a reader who is merely
// told must not be given two different accounts of where the head is.
func TestTheStaleRoundReportIsAnHonestyDisclosureTheRefusalRepeats(t *testing.T) {
	moved := state.Round{
		Meta:    state.Meta{Owner: "acme", Repo: "web", PR: 42, Round: 3, Head: "0f1e2d3"},
		Current: "9a8b7c6",
	}
	var quietProof finding.HonestyDisclosure = &moved
	assert.Equal(t,
		"§9.3.1: round 3 was opened at head 0f1e2d3 and the pull request's current head is 9a8b7c6",
		quietProof.Disclosure())

	refused := moved.RefuseStale()
	require.Error(t, refused)
	assert.True(t, strings.HasPrefix(refused.Error(), quietProof.Disclosure()),
		"§9.3.2's refusal and §11.1's report are one fact, so the refusal opens with it")
	assert.Equal(t, ExitState, exitCodeFor(refused), "§11.2 codes a state conflict 4")
	assert.Equal(t, ExitState, exitCodeFor(&state.NoCurrentHeadError{}),
		"§9.3.1 admits no command that skips the comparison")
}

// movedHeadRuns is the argv every command in the tree is exercised with after
// the head moved, keyed by the command as it is typed and by what §9.3 owes the
// caller.
//
// cr cannot invent a command's arguments, so the argv is a table; what keeps it
// from freezing at today's surface is that the guard below compares its keys
// against the tree. A command added later has no row, fails here, and its
// author has to say which of the four answers §9.3 gives it.
//
// The file arguments are paths that are never opened. Every refusal §9.3.2
// makes happens on the round, which each of these commands reads before it
// reads anything the caller named — so a run that got as far as the file would
// already have failed this guard.
func movedHeadRuns(dir string) map[string]section93 {
	file := func(name string) string { return filepath.Join(dir, name) }
	return map[string]section93{
		// §9.3.2's writers: every one of them writes per-PR state.
		"record":         refusesTheWrite("record", fixturePR, file("merged.ndjson")),
		"claims record":  refusesTheWrite("claims", "record", fixturePR, file("claims.ndjson")),
		"cells record":   refusesTheWrite("cells", "record", fixturePR, file("cells.ndjson")),
		"map record":     refusesTheWrite("map", "record", fixturePR, file("mapping.ndjson")),
		"draft":          refusesTheWrite("draft", fixturePR),
		"post":           refusesTheWrite("post", fixturePR),
		"review":         refusesTheWrite("review", fixturePR),
		"sandbox create": refusesTheWrite("sandbox", "create", fixturePR),
		"test":           refusesTheWrite("test", fixturePR),
		"probe run": refusesTheWrite("probe", "run", fixturePR,
			"--kind", "mutation", "--patch", file("mutation.patch")),

		// §9.3.1's readers: neither writes per-PR state, so both run
		// and both say where the head is.
		"rules check": disclosesTheMove("rules", "check", fixturePR),
		"answer": disclosesTheMove("answer", fixturePR, "f3", "the retry is deliberate",
			"--source", "chat"),

		// §9.3.2's way forward, and the commands that read no round at
		// all.
		"brief":           opensTheRound("brief", fixturePR, "--issue", fixtureIssue, "--intent-file", file("issue.txt")),
		"init":            readsNoRound("init"),
		"config":          readsNoRound("config"),
		"note":            readsNoRound("note", fixtureIssue, "a fact", "--source", "chat", "--pr", fixturePR),
		"context":         readsNoRound("context", fixtureIssue),
		"sandbox destroy": readsNoRound("sandbox", "destroy", fixturePR),
		// `cr rules suggest` is repository-scoped: §2.6.3.1 scans
		// every recorded round of every pull request, so there is no
		// one round whose head §9.3.1 could compare against a current
		// one, and no per-PR state for §9.3.2 to refuse the write of.
		"rules suggest": readsNoRound("rules", "suggest"),
	}
}

// section93 is one command's invocation and the answer §9.3 owes its caller
// once the head has moved.
type section93 struct {
	// argv is the command line, without `--repo`, which the guard adds.
	argv []string
	// owed is what the run must produce.
	owed owing
}

// owing is the four answers §9.3 admits, one of which every command in the tree
// has to be given.
type owing int

const (
	// owesRefusal is §9.3.2: the command writes per-PR state, so it stops
	// with §11.2's code 4 naming both heads.
	owesRefusal owing = iota
	// owesDisclosure is §9.3.1 alone: the command reads per-PR state,
	// writes none, and reports both heads.
	owesDisclosure
	// owesANewRound is `cr brief`, which §9.3.2 names as the only way
	// forward and §9.3.3 has open the round the new head belongs to.
	owesANewRound
	// owesNothing is a command that reads no round, so §9.3 has nothing to
	// say about it.
	owesNothing
	// owesNothingYet is a command whose behaviour a later task owns.
	owesNothingYet
)

func refusesTheWrite(argv ...string) section93 { return section93{argv: argv, owed: owesRefusal} }
func disclosesTheMove(argv ...string) section93 {
	return section93{argv: argv, owed: owesDisclosure}
}
func opensTheRound(argv ...string) section93 { return section93{argv: argv, owed: owesANewRound} }
func readsNoRound(argv ...string) section93  { return section93{argv: argv, owed: owesNothing} }

// §9.3 over the whole command surface, with the head really moved: every
// command that writes per-PR state refuses with exit code 4 naming both heads,
// every command that only reads it reports both and runs, and `cr brief` opens
// the round the new head belongs to.
//
// The head is moved the way a force-push moves it — a second commit on the head
// branch, with `gh` answering that commit and meta.json still naming the first
// — so what is measured is two real revisions disagreeing rather than a flag
// saying they do.
func TestAMovedHeadRefusesEveryWriterAndIsDisclosedToEveryReader(t *testing.T) {
	layout, recorded, moved := aRoundTheHeadOutran(t)
	dir := t.TempDir()
	for _, name := range []string{
		"merged.ndjson", "claims.ndjson", "cells.ndjson", "mapping.ndjson",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "issue.txt"), []byte("Retry on 5xx.\n"), 0o600))
	// §5.3.1's mutation is read and parsed before the round is, so this
	// one is a real unified diff: a patch refused for holding no hunk
	// would never reach the refusal this guard is about.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mutation.patch"), []byte(
		"--- a/lib.go\n+++ b/lib.go\n@@ -4 +4 @@\n-\tpanic(\"one\")\n+\tpanic(\"two\")\n"), 0o600))

	runs := movedHeadRuns(dir)
	for _, name := range leafCommands(t) {
		found, _, err := newRootCmd().Find(strings.Fields(name))
		require.NoError(t, err)
		if _, stub := found.Annotations[stubAnnotation]; stub {
			runs[name] = section93{owed: owesNothingYet}
		}
	}
	require.ElementsMatch(t, leafCommands(t), slices.Collect(maps.Keys(runs)),
		"§9.3 binds the whole surface, so a command in the tree needs an answer here")

	// `cr brief` runs last, because §9.3.3 has it move the round onto the
	// new head: every other run has to meet the round the head outran, and
	// one that ran after this would meet a round that is current again.
	for _, name := range append(withoutBrief(runs), "brief") {
		run := runs[name]
		if run.owed == owesNothingYet {
			continue
		}
		t.Run(name, func(t *testing.T) {
			assertSection93(t, layout, recorded, moved, run)
		})
	}
}

// withoutBrief is the table's commands in a fixed order, with `cr brief` held
// back for the caller to append.
func withoutBrief(runs map[string]section93) []string {
	ordered := make([]string, 0, len(runs))
	for _, name := range slices.Sorted(maps.Keys(runs)) {
		if name != "brief" {
			ordered = append(ordered, name)
		}
	}
	return ordered
}

// assertSection93 runs one command and holds it to what §9.3 owes its caller.
func assertSection93(t *testing.T, layout state.Layout, recorded, moved string, run section93) {
	t.Helper()
	printed, err := runCLIPrinting(t, append(slices.Clone(run.argv), "--repo", fixtureSlug)...)

	switch run.owed {
	case owesRefusal:
		require.Error(t, err, "§9.3.2: this command writes per-PR state and the head moved")
		var stale *state.StaleRoundError
		require.ErrorAs(t, err, &stale)
		assert.Equal(t, ExitState, exitCodeFor(err), "§11.2 codes a state conflict 4")
		assert.Contains(t, err.Error(), recorded, "§9.3.2 names the recorded head")
		assert.Contains(t, err.Error(), moved, "§9.3.2 names the current head")
		assert.Contains(t, err.Error(), "cr brief "+fixturePR,
			"§9.3.2: `cr brief` is the only way forward, so the refusal names it")
	case owesDisclosure:
		require.NoError(t, err, "§9.3.1 reports the move; it is §9.3.2 that refuses, and this writes nothing")
		assert.Contains(t, honestyOf(t, printed), recorded)
		assert.Contains(t, honestyOf(t, printed), moved)
	case owesANewRound:
		require.NoError(t, err, "§9.3.2: `cr brief` is the only way forward, so it has to work")
		opened, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
		require.NoError(t, err)
		assert.Equal(t, 2, opened.Round, "§9.3.3 increments the round index when the head differs")
		assert.Equal(t, moved, opened.Head, "§9.3.3 then stores the new head")
	case owesNothing:
		require.NoError(t, err, "this command reads no round, so §9.3 refuses it nothing")
	case owesNothingYet:
		t.Fatal("a stub is skipped by the caller")
	}
}

// honestyOf reads the `honesty` array out of a command's JSON document, which
// is the channel §11.1 exempts from `--quiet`.
func honestyOf(t *testing.T, printed string) string {
	t.Helper()
	var document struct {
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &document))
	require.NotEmpty(t, document.Honesty,
		"§9.3.1 has the command report both heads, and it reported nothing")
	return strings.Join(document.Honesty, "\n")
}

// §9.3.2 exempts `cr post --reconcile` from the refusal by name, because it
// anchors nothing and only matches §8.4.3's payload hash against the reviews
// the pull request already has.
//
// The flag's behaviour is not built, and the exemption is nonetheless
// observable: the refusal for an unbuilt flag is raised before any state is
// read, so a moved head produces §11.2's code 2 naming the flag rather than
// code 4 naming the heads. That ordering is the exemption, and this is what
// holds it in place while the behaviour is written — a `--reconcile` that
// arrived on the other side of the round read would fail here.
func TestPostReconcileIsExemptFromTheStaleHeadRefusal(t *testing.T) {
	aRoundTheHeadOutran(t)

	err := runCLI(t, "post", fixturePR, "--repo", fixtureSlug, "--reconcile")

	require.Error(t, err)
	var stale *state.StaleRoundError
	assert.NotErrorAs(t, err, &stale,
		"§9.3.2 exempts `cr post --reconcile`, so the stale head is not what stopped it")
	assert.Equal(t, ExitUsage, exitCodeFor(err))
	assert.Contains(t, err.Error(), "--reconcile")
}

// aRoundTheHeadOutran is a pull request whose head really moved: two commits on
// the head branch, a round recorded at the first, and a `gh` answering with the
// second.
//
// The move is two revisions rather than a flag. §9.3.1 compares meta.json's
// recorded head against the head GitHub reports, and gh/pr.go argues the second
// is GitHub's `headRefOid` — so a fixture that faked the disagreement would test
// the comparison and not what it compares. Both commits are real, which is also
// what lets `cr brief` open the round the new head belongs to at the end of the
// run.
//
// It returns the state root, the recorded head, and the current one.
func aRoundTheHeadOutran(t *testing.T) (layout state.Layout, recorded, moved string) {
	t.Helper()
	dir := t.TempDir()
	write := func(body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.go"), []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("package lib\n\nfunc Load() {}\n")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("package lib\n\nfunc Load() {\n\tpanic(\"one\")\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the change the round was opened on")
	recorded = strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	write("package lib\n\nfunc Load() {\n\tpanic(\"two\")\n}\n")
	mustGit(t, dir, "commit", "--quiet", "-am", "the force-push the round did not see")
	moved = strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))
	require.NotEqual(t, recorded, moved, "the fixture has to move the head to measure anything")

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), moved, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout = state.New(crHome(t))
	require.NoError(t, layout.Init())
	// The seam and the shim answer the same head: `cr review` reads the
	// current head through the client on its own sources, and every other
	// command reads it through internal/cli's. Both are GitHub's
	// `headRefOid` in production, and a fixture in which they disagreed
	// would leave half the surface measuring the wrong thing.
	movedHead(t, moved)

	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 1, Head: recorded,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":3,"end":5}],`+
			`"head":"`+recorded+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	return layout, recorded, moved
}

// runCLIPrinting runs one invocation against whatever CR_HOME points at and
// returns both what it printed and what it refused.
func runCLIPrinting(t *testing.T, args ...string) (printed string, err error) {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	// Executed first and read after: a return statement evaluates its
	// operands left to right, so reading the buffer in it would read what
	// the command printed before it ran.
	err = cmd.Execute()
	return out.String(), err
}
