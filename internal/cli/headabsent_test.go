package cli

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// fetchHint is the step a head the clone does not hold is met with.
const fetchHint = "run `git fetch` in the repository under review so it holds the commit the message names " +
	"(`git fetch origin pull/<pr>/head` for a pull request from a fork), then run the command again"

// forcePushedClone is QA W3's push-03: a round recorded at a head that a
// force-push then took off the head branch, read from a fresh clone made after
// the push, which therefore never received the recorded head.
//
// The origin keeps the old commit as an unreachable object, so the absence is
// the clone's: `git clone --no-local` transfers only what a ref reaches. It
// returns the state layout, the recorded head, and the head the push left.
func forcePushedClone(t *testing.T) (layout state.Layout, recorded, moved string) {
	t.Helper()
	origin := t.TempDir()
	write := func(body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(origin, "lib.go"), []byte(body), 0o600))
	}
	mustGit(t, origin, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("package lib\n\nfunc Load() {}\n")
	mustGit(t, origin, "add", "lib.go")
	mustGit(t, origin, "commit", "--quiet", "-m", "the base")
	mustGit(t, origin, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("package lib\n\nfunc Load() {\n\tpanic(\"one\")\n}\n")
	mustGit(t, origin, "commit", "--quiet", "-am", "the change the round was opened on")
	recorded = strings.TrimSpace(mustGit(t, origin, "rev-parse", fixtureHeadBranch))
	mustGit(t, origin, "checkout", "--quiet", "-B", fixtureHeadBranch, "main")
	write("package lib\n\nfunc Load() {\n\tpanic(\"two\")\n}\n")
	mustGit(t, origin, "commit", "--quiet", "-am", "the history the force-push left")
	moved = strings.TrimSpace(mustGit(t, origin, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, origin, "rev-parse", "main"))

	clone := filepath.Join(t.TempDir(), "fresh")
	mustGit(t, t.TempDir(), "clone", "--quiet", "--no-local", origin, clone)
	_, err := gitIn(t, clone, "cat-file", "-e", recorded+"^{commit}")
	require.Error(t, err, "the fixture is only worth anything if the clone lacks the recorded head")
	mustGit(t, clone, "cat-file", "-e", moved+"^{commit}")

	restore := repoDir
	repoDir = func() (string, error) { return clone, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), moved, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout = state.New(crHome(t))
	require.NoError(t, layout.Init())
	movedHead(t, moved)
	recordRoundAt(t, layout, recorded)
	return layout, recorded, moved
}

// recordRoundAt writes round 1 of the fixture pull request at head, with one
// unit, the way `cr brief` would have left it.
func recordRoundAt(t *testing.T, layout state.Layout, head string) {
	t.Helper()
	require.NoError(t, layout.EnsurePR(fixtureOwner, fixtureProject, fixturePRNumber))
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber,
		IssueKey: fixtureIssue, Round: 1, Head: head,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(
		`{"id":"u1","path":"lib.go","side":"RIGHT","hunk_ranges":[{"start":3,"end":5}],`+
			`"head":"`+head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
}

// §9.3.1 over a clone that never held the recorded head: `cr status` reports
// both heads and names `cr brief`, and reads nothing at the recorded head, so
// the report is not lost to git's refusal of a tree the clone lacks (QA
// D-W3-1, where it exited 3 with `ls-tree … not a tree object`).
//
// What the report leaves out is said rather than zeroed: the file counts are
// absent from the document, not printed as nothing excluded, and the honesty
// channel names the halves and the counts it could not read.
func TestStatusReportsAMovedHeadTheCloneNeverHeld(t *testing.T) {
	_, recorded, moved := forcePushedClone(t)

	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err, "§9.3.1 reports the move, and §9.3.2 refuses only writers")

	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(printed), &document))
	assert.NotContains(t, document, "files", "the counts are read at the recorded head, so none is claimed")
	var report struct {
		Completeness struct {
			Complete bool     `json:"complete"`
			Reasons  []string `json:"reasons"`
		} `json:"completeness"`
		Honesty []string `json:"honesty"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	assert.False(t, report.Completeness.Complete)
	require.NotEmpty(t, report.Completeness.Reasons)
	assert.Equal(t, "§10.2.1: the pull request's head is no longer the head this round was recorded against",
		report.Completeness.Reasons[0])
	require.GreaterOrEqual(t, len(report.Honesty), 2)
	assert.Equal(t, "§9.3.1: round 1 was opened at head "+recorded+" and the pull request's current head is "+
		moved+"; run `cr brief "+fixturePR+" --repo "+fixtureSlug+"` to open the round the current head belongs to",
		report.Honesty[0])
	assert.Equal(t, "§9.3.1: the reinvention half of §4.3.1, the symbol half of §4.4.1, and the file counts of "+
		"§3.4.2 and §3.4.7 are not reported for round 1, because each is read at head "+recorded+
		", which is no longer the pull request's head", report.Honesty[1])
}

// unfetchedClone is QA W3's setup note: a clone made before the head was
// pushed, so GitHub reports a head the clone has never fetched. It returns the
// clone and that head.
func unfetchedClone(t *testing.T) (clone, head string) {
	t.Helper()
	origin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(origin, "lib.go"), []byte("package lib\n\nfunc Load() {}\n"), 0o600))
	mustGit(t, origin, "-c", "init.defaultBranch=main", "init", "--quiet")
	mustGit(t, origin, "add", "lib.go")
	mustGit(t, origin, "commit", "--quiet", "-m", "the base")
	clone = filepath.Join(t.TempDir(), "early")
	mustGit(t, t.TempDir(), "clone", "--quiet", "--no-local", origin, clone)
	mustGit(t, origin, "checkout", "--quiet", "-b", fixtureHeadBranch)
	require.NoError(t, os.WriteFile(filepath.Join(origin, "lib.go"),
		[]byte("package lib\n\nfunc Load() {\n\tpanic(\"one\")\n}\n"), 0o600))
	mustGit(t, origin, "commit", "--quiet", "-am", "the head pushed after the clone")
	head = strings.TrimSpace(mustGit(t, origin, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, origin, "rev-parse", "main"))
	_, err := gitIn(t, clone, "cat-file", "-e", head+"^{commit}")
	require.Error(t, err, "the fixture is only worth anything if the clone lacks the head")

	restore := repoDir
	repoDir = func() (string, error) { return clone, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return clone, head
}

// A head the clone never fetched is a fetch, for `cr brief` and for `cr status`
// alike: exit 3 naming the commit and the repository, and a hint naming
// `git fetch`, rather than git's refusal of a merge base or a tree with a hint
// to run git by hand. `cr brief` records nothing on the way.
func TestAHeadTheCloneNeverFetchedIsAFetch(t *testing.T) {
	clone, head := unfetchedClone(t)
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": panic once.\n"), 0o600))
	said := "the repository at " + clone + " does not hold commit " + head

	_, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	var missing *git.MissingCommitError
	require.ErrorAs(t, err, &missing, "cr brief")
	assert.Equal(t, said, err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, fetchHint, hintFor(err))
	_, err = layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.Error(t, err, "a brief that could not read the head opens no round")

	recordRoundAt(t, layout, head)
	_, err = runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.ErrorAs(t, err, &missing, "cr status")
	assert.Equal(t, said, err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, fetchHint, hintFor(err))
}

// gitHint is the step a git failure of any other kind is met with.
const gitHint = "the git command the message names failed; run it yourself to see what it reports"

// headRun is how one command in the tree meets the head and the merge base of
// the repository under review: the invocation that reaches its git read and the
// round that read needs. Which commits' absence that read reaches is the
// command's row in headReads, the table headNotFetched asks from. A command
// that reads neither commit carries the reason instead.
type headRun struct {
	// argv is the command line, `--repo` included.
	argv []string
	// drafted is the record the round is recorded and drafted with, and nil
	// for a queued question on u1's added line 12 of money.go.
	drafted map[string]any
	// prepare readies what the run needs beyond the drafted round, given the
	// state root and the round's draft.md, and is nil when it needs nothing.
	prepare func(t *testing.T, layout state.Layout, drafted string)
	// exempt is why the command reads neither the head nor the merge base
	// from the clone, and is empty for a command routed through
	// headNotFetched.
	exempt string
}

// headRuns classifies every command in the tree, keyed as the command is
// typed. A routed command's argv reaches a git read of each commit headReads
// names for it on draftedCloneLacking's round; an exempt command's argv is run
// over a clone lacking the head to hold the exemption to what the command does.
//
// The records a LEFT anchor carries are what take `cr record`, `cr merge` and
// `cr draft` to the merge base: §6.1.2 reads a LEFT anchor at it, and a RIGHT
// one at the head alone.
func headRuns(t *testing.T) map[string]headRun {
	t.Helper()
	dir := t.TempDir()
	file := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		return path
	}
	left := onUnit("u2", "order.go", "LEFT", 11, 12)
	left["id"] = "f2"
	line, err := json.Marshal(left)
	require.NoError(t, err)
	merged := file("merged.ndjson", string(line)+"\n")
	perRole := file("review-correctness.ndjson", string(line)+"\n")
	empty := file("empty.ndjson", "")
	patch := file("mutation.patch", "--- a/money.go\n+++ b/money.go\n@@ -12 +12 @@\n-// money added a\n+// money mutated a\n")
	issue := briefIssue(t)
	routed := func(argv ...string) headRun {
		return headRun{argv: append(argv, "--repo", fixtureSlug)}
	}
	exempt := func(why string, argv ...string) headRun {
		return headRun{argv: argv, exempt: why}
	}

	runs := map[string]headRun{
		"brief":  routed("brief", fixturePR, "--issue", fixtureIssue, "--intent-file", issue),
		"status": routed("status", fixturePR),
		// The intent pass is the one a round without a mapping may emit.
		"review":         routed("review", fixturePR, "--axis", "intent"),
		"post":           routed("post", fixturePR),
		"record":         routed("record", fixturePR, merged),
		"merge":          routed("merge", perRole, "-o", filepath.Join(dir, "merged-out.ndjson"), "--pr", fixturePR),
		"rules check":    routed("rules", "check", fixturePR),
		"sandbox create": routed("sandbox", "create", fixturePR),
		"test":           routed("test", fixturePR),
		"probe run":      routed("probe", "run", fixturePR, "--kind", "mutation", "--patch", patch),

		"init":   exempt("writes the state root and the shipped profiles, and reads no repository", "init"),
		"config": exempt("prints the configuration layers, and reads the clone for its remotes alone", "config", "--repo", fixtureSlug),
		"note": exempt("stores a fact in the context store under the state root", "note", fixtureIssue, "a fact",
			"--source", "chat", "--pr", fixturePR, "--repo", fixtureSlug),
		"context": exempt("prints the context store under the state root", "context", fixtureIssue),
		"answer": exempt("files a note against a record the round holds, and reads no revision",
			"answer", fixturePR, "f1", "the retry is deliberate", "--source", "chat", "--repo", fixtureSlug),
		"claims record": exempt("validates claims against the issue text, and reads no revision",
			"claims", "record", fixturePR, empty, "--intent-file", issue, "--repo", fixtureSlug),
		"claims set-aside": exempt("stamps intent-gaps.ndjson from the context store, and reads no revision",
			"claims", "set-aside", fixturePR, fixtureIssue+"#c1", "--note", fixtureIssue+"#n1", "--repo", fixtureSlug),
		"cells record": exempt("validates cells against the round's units under the state root, and reads no revision",
			"cells", "record", fixturePR, empty, "--repo", fixtureSlug),
		"map record": exempt("validates the mapping against the round's units under the state root, and reads no revision",
			"map", "record", fixturePR, empty, "--repo", fixtureSlug),
		"stats": exempt("counts the repository's triage.ndjson under the state root", "stats", "--repo", fixtureSlug),
		"rules list": exempt("reads the rule corpus and ledger, and stats marker files in the checkout rather than a revision",
			"rules", "list", "--dead", "--repo", fixtureSlug),
		"rules suggest": exempt("scans posted comments under the state root", "rules", "suggest", "--repo", fixtureSlug),
		"waivers list": exempt("reads the waiver files under the state root",
			"waivers", "list", "--repo", fixtureSlug, "--pr", fixturePR),
		"waivers remove": exempt("rewrites a waiver file under the state root",
			"waivers", "remove", "wp1", "--repo", fixtureSlug, "--pr", fixturePR),
		"sandbox destroy": exempt("removes the worktree and its registration by path, and checks out no revision",
			"sandbox", "destroy", fixturePR, "--repo", fixtureSlug),
	}

	// §6.1.2 reads a LEFT anchor at the merge base: `cr record` and `cr
	// merge` are handed one, and `cr draft` meets one moved by its marker.
	draft := routed("draft", fixturePR)
	draft.drafted = onUnit("u2", "order.go", "LEFT", 11, 12)
	draft.prepare = func(t *testing.T, _ state.Layout, drafted string) {
		t.Helper()
		editF1Marker(t, drafted, `start_line="11" line="12"`, `start_line="12" line="12"`)
	}
	runs["draft"] = draft
	// §5.2.1 runs the profile's test command, so the round names a profile
	// that has one; the run fails before it would start.
	for _, name := range []string{"test", "probe run"} {
		run := runs[name]
		run.prepare = testProfiled
		runs[name] = run
	}
	return runs
}

// testProfiled has the round name a profile whose tests.cmd is set.
func testProfiled(t *testing.T, layout state.Layout, _ string) {
	t.Helper()
	require.NoError(t, layout.EnsureProfile("qa", `{"id":"qa","match":{"files":[],"globs":[]},`+
		`"axes":{"test":true},"tests":{"cmd":["/usr/bin/true"],"globs":["*_test.go"]}}`))
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.ProfileID = "qa"
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())
}

// routedHeadRuns are headRuns' commands routed through headNotFetched, in a
// fixed order.
func routedHeadRuns(t *testing.T) []string {
	t.Helper()
	runs := headRuns(t)
	var names []string
	for _, name := range slices.Sorted(maps.Keys(runs)) {
		if runs[name].exempt == "" {
			names = append(names, name)
		}
	}
	return names
}

// draftedFor is threeUnitHome with run's record recorded and drafted, and run
// prepared.
func draftedFor(t *testing.T, run *headRun) {
	t.Helper()
	record := run.drafted
	if record == nil {
		record = onUnit("u1", "money.go", "RIGHT", 12, 12)
	}
	layout, drafted := unitDrafted(t, record)
	if run.prepare != nil {
		run.prepare(t, layout, drafted)
	}
}

// draftedCloneLacking is draftedFor's round, read from a no-local clone of its
// repository that lacks one commit GitHub reports: the pull request's head, or
// a base that moved after the clone was made. It returns the clone and the
// commit it lacks.
func draftedCloneLacking(t *testing.T, lacking string, run *headRun) (clone, missing string) {
	t.Helper()
	draftedFor(t, run)
	full, err := repoDir()
	require.NoError(t, err)
	head := strings.TrimSpace(mustGit(t, full, "rev-parse", fixtureHeadBranch))
	branch := "main"
	missing = head
	if lacking == readsBase {
		branch = fixtureHeadBranch
		tree := strings.TrimSpace(mustGit(t, full, "rev-parse", "main^{tree}"))
		missing = strings.TrimSpace(mustGit(t, full, "commit-tree", "-p", "main", "-m", "the base moved on", tree))
		mustGit(t, full, "update-ref", "refs/heads/main", missing)
		t.Setenv("PATH", ghShim(t, t.TempDir(), head, missing)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	clone = filepath.Join(t.TempDir(), "clone")
	mustGit(t, t.TempDir(), "clone", "--quiet", "--no-local", "--single-branch", "--branch", branch, full, clone)
	_, err = gitIn(t, clone, "cat-file", "-e", missing+"^{commit}")
	require.Error(t, err, "the fixture is only worth anything if the clone lacks the commit")

	restore := repoDir
	repoDir = func() (string, error) { return clone, nil }
	t.Cleanup(func() { repoDir = restore })
	return clone, missing
}

// Every command in the tree is classified: routed through headNotFetched with
// the commits its read reaches named in headReads, or exempt with the reason it
// reads neither. A command added later fails here by name until it is given one
// of the two.
//
// An exemption is held to what the command does: each exempt command runs over
// a clone lacking the head, and a git failure there is a read the exemption
// denies. Every read of the merge base is taken with the head, so a clone
// lacking the head fails a read of either commit.
func TestEveryCommandIsClassifiedAsAHeadReaderOrExempt(t *testing.T) {
	runs := headRuns(t)
	tree := leafCommands(t)
	for _, name := range tree {
		run, classified := runs[name]
		if !classified {
			t.Errorf("cr %s is in the command tree and not in headRuns: route its git failures through "+
				"headNotFetched and name the commits its read reaches in headReads, or exempt it with the "+
				"reason it reads neither the head nor the merge base", name)
			continue
		}
		if (run.exempt == "") == (len(headReads[name]) == 0) {
			t.Errorf("cr %s has both a row in headReads and a reason it is exempt, or neither", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(runs)) {
		if !slices.Contains(tree, name) {
			t.Errorf("headRuns classifies cr %s, which the command tree does not have", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(headReads)) {
		if !slices.Contains(tree, name) {
			t.Errorf("headReads names the reads of cr %s, which the command tree does not have", name)
		}
		if reads := headReads[name]; !slices.Equal(reads, []string{readsHead}) &&
			!slices.Equal(reads, []string{readsHead, readsBase}) {
			t.Errorf("headReads names %q for cr %s: every read of the merge base is taken with the head, "+
				"so a row is the head alone or the head and then the base", reads, name)
		}
	}

	draftedCloneLacking(t, "head", &headRun{})
	for _, name := range slices.Sorted(maps.Keys(runs)) {
		if runs[name].exempt == "" {
			continue
		}
		_, err := runCLIPrinting(t, runs[name].argv...)
		var failed *git.CommandError
		var missing *git.MissingCommitError
		assert.False(t, errors.As(err, &failed) || errors.As(err, &missing),
			"cr %s is exempt because it %s, and a git read failed over a clone lacking the head: %v",
			name, runs[name].exempt, err)
	}
}

// One predicate for every command reading the clone's head: each routed
// command, on a clone that lacks the pull request's head, or the base where its
// read reaches the merge base, gives the missing commit, code 3 and the `git
// fetch` hint, rather than git's refusal of a tree or a merge base with a hint
// to run git by hand.
func TestEveryCommandReadingTheHeadGivesTheFetchHint(t *testing.T) {
	for _, name := range routedHeadRuns(t) {
		for _, lacking := range headReads[name] {
			t.Run(lacking+"/"+name, func(t *testing.T) {
				run := headRuns(t)[name]
				clone, absent := draftedCloneLacking(t, lacking, &run)

				_, err := runCLIPrinting(t, run.argv...)
				var missing *git.MissingCommitError
				require.ErrorAs(t, err, &missing)
				assert.Equal(t, git.MissingCommitError{Dir: clone, Commit: absent}, *missing)
				assert.Equal(t, "the repository at "+clone+" does not hold commit "+absent, err.Error())
				assert.Equal(t, ExitFile, exitCodeFor(err))
				assert.Equal(t, fetchHint, hintFor(err))
			})
		}
	}
}

// The same commands name a `--repo` no remote of the clone points at, in the
// hint of the missing commit they now classify alike.
func TestEveryCommandReadingTheHeadNamesARepoNoRemotePointsAt(t *testing.T) {
	hint := "`--repo` names octocat/hello, and no remote of the repository under review points at it " +
		"(origin points at someone/elsewhere), so a `git fetch` from its remotes may not bring the commit " +
		"the message names; check that `--repo` names the pull request's repository; if it does, fetch " +
		"the commit from it with `git fetch https://github.com/octocat/hello pull/7/head` and run the " +
		"command again, or run the command in a clone of octocat/hello"
	for _, name := range routedHeadRuns(t) {
		t.Run(name, func(t *testing.T) {
			run := headRuns(t)[name]
			clone, absent := draftedCloneLacking(t, "head", &run)
			mustGit(t, clone, "remote", "set-url", "origin", "git@github.com:someone/elsewhere.git")

			_, err := runCLIPrinting(t, run.argv...)
			var mismatch *RemoteMismatchError
			require.ErrorAs(t, err, &mismatch)
			assert.Equal(t, "the repository at "+clone+" does not hold commit "+absent, err.Error())
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, hint, hintFor(err))
		})
	}
}

// The other direction: a git failure while the clone holds both the head and
// the base is not a fetch. The head commit's tree is taken out of the object
// store, so every read of the head's content fails, and each routed command
// keeps git's own error and hint.
func TestAGitFailureOfAnotherKindKeepsItsOwnHint(t *testing.T) {
	for _, name := range routedHeadRuns(t) {
		t.Run(name, func(t *testing.T) {
			run := headRuns(t)[name]
			draftedFor(t, &run)
			full, err := repoDir()
			require.NoError(t, err)
			tree := strings.TrimSpace(mustGit(t, full, "rev-parse", fixtureHeadBranch+"^{tree}"))
			require.NoError(t, os.Remove(filepath.Join(full, ".git", "objects", tree[:2], tree[2:])))
			mustGit(t, full, "cat-file", "-e", fixtureHeadBranch+"^{commit}")
			mustGit(t, full, "cat-file", "-e", "main^{commit}")

			_, err = runCLIPrinting(t, run.argv...)
			var failed *git.CommandError
			require.ErrorAs(t, err, &failed)
			var missing *git.MissingCommitError
			assert.False(t, errors.As(err, &missing), "the clone holds both commits")
			assert.Equal(t, failed.Error(), err.Error())
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, gitHint, hintFor(err))
		})
	}
}

// A command headReads gives the head alone reads nothing a moved base fails:
// over a clone that holds the head and lacks the base GitHub reports,
// `cr sandbox create`, `cr test` and `cr probe run` each succeed, so the table
// leaves out no read of the merge base. Those three are the only such rows, so
// a base reader whose row lost the base fails here rather than being held to
// the head alone by every test that reads the table.
func TestACommandReadingTheHeadAloneReadsNothingAMovedBaseFails(t *testing.T) {
	var headOnly []string
	for _, name := range routedHeadRuns(t) {
		if !slices.Contains(headReads[name], readsBase) {
			headOnly = append(headOnly, name)
		}
	}
	require.Equal(t, []string{"probe run", "sandbox create", "test"}, headOnly)
	for _, name := range headOnly {
		t.Run(name, func(t *testing.T) {
			run := headRuns(t)[name]
			draftedCloneLacking(t, readsBase, &run)

			_, err := runCLIPrinting(t, run.argv...)
			assert.NoError(t, err, "cr %s reads the head alone, and failed over a clone lacking the base", name)
		})
	}
}

// A git failure of another kind, while the clone holds the head and lacks a
// base that moved, is a fetch of the base only for a command that reads the
// merge base. The head commit's tree is taken out of the object store, so every
// read of the head's content fails: `cr rules check` and every other command
// reading the merge base names the base, code 3 and the `git fetch` hint, and
// `cr sandbox create`, `cr test` and `cr probe run`, which read the head alone,
// keep git's own error and hint.
func TestAGitFailureBesideAMovedBaseIsAFetchOnlyForItsReaders(t *testing.T) {
	for _, name := range routedHeadRuns(t) {
		t.Run(name, func(t *testing.T) {
			run := headRuns(t)[name]
			draftedFor(t, &run)
			full, err := repoDir()
			require.NoError(t, err)
			head := strings.TrimSpace(mustGit(t, full, "rev-parse", fixtureHeadBranch))
			tree := strings.TrimSpace(mustGit(t, full, "rev-parse", fixtureHeadBranch+"^{tree}"))
			base := strings.TrimSpace(mustGit(t, full, "commit-tree", "-p", "main", "-m", "the base moved on",
				strings.TrimSpace(mustGit(t, full, "rev-parse", "main^{tree}"))))
			for _, object := range []string{tree, base} {
				require.NoError(t, os.Remove(filepath.Join(full, ".git", "objects", object[:2], object[2:])))
			}
			t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))
			mustGit(t, full, "cat-file", "-e", head+"^{commit}")
			_, err = gitIn(t, full, "cat-file", "-e", base+"^{commit}")
			require.Error(t, err, "the fixture is only worth anything if the clone lacks the base")

			_, err = runCLIPrinting(t, run.argv...)
			var missing *git.MissingCommitError
			if slices.Contains(headReads[name], readsBase) {
				require.ErrorAs(t, err, &missing)
				assert.Equal(t, git.MissingCommitError{Dir: full, Commit: base}, *missing)
				assert.Equal(t, "the repository at "+full+" does not hold commit "+base, err.Error())
				assert.Equal(t, ExitFile, exitCodeFor(err))
				assert.Equal(t, fetchHint, hintFor(err))
				return
			}
			var failed *git.CommandError
			require.ErrorAs(t, err, &failed)
			assert.False(t, errors.As(err, &missing), "cr %s does not read the base", name)
			assert.Equal(t, failed.Error(), err.Error())
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, gitHint, hintFor(err))
		})
	}
}
