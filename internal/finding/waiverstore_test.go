package finding

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// The pull request every waiver below is written from. §7.4.8 records it on a
// repository-wide waiver as provenance and §7.4.4 scopes a `not-here` one to
// it, so one number serves both and the tests can tell the two roles apart.
const (
	waiverOwner = "octocat"
	waiverRepo  = "hello"
	waiverPR    = 7
)

// waiverHome is a state root with §2.2's tree in place.
func waiverHome(t *testing.T) state.Layout {
	t.Helper()
	layout := state.New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsureRepo(waiverOwner, waiverRepo))
	return layout
}

// theProvenance is §7.4.8's four fields, reason included.
func theProvenance() WaiverProvenance {
	return WaiverProvenance{
		Round: 3, PR: waiverPR, Head: "0a1b2c3", Reason: "the legacy area is exempt",
	}
}

// waive writes the waiver one discarded record calls for, through the whole of
// Waive, so every test below exercises the scope derivation rather than a scope
// it chose for itself.
func waive(t *testing.T, layout state.Layout, record *Finding, prov WaiverProvenance) WaiverRecord {
	t.Helper()
	waiver, err := WaiverFor(record)
	require.NoError(t, err)
	recorded, err := Waive(layout, waiverOwner, waiverRepo, &waiver, prov)
	require.NoError(t, err)
	return recorded
}

// storedAt decodes one waiver file straight off disk.
//
// The file is read rather than the reader called, because what §7.4.4 fixes is
// which path the bytes land at: a reader asked for the repository's waivers
// would answer the same whichever file it had been pointed at.
func storedAt(t *testing.T, path string) []WaiverRecord {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)

	stored := make([]WaiverRecord, 0)
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record WaiverRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		stored = append(stored, record)
	}
	return stored
}

// waiverFiles are §7.4.4's two paths for the fixture repository.
func waiverFiles(layout state.Layout) (repository, pullRequest string) {
	return layout.WaiversFile(waiverOwner, waiverRepo),
		layout.PRFile(waiverOwner, waiverRepo, waiverPR, state.FileWaivers)
}

// §7.4.4 puts the two scopes in two files, and the scope follows the
// disposition: `wrong` is a fact about the class and goes to
// `~/.cr/waivers/<owner>/<repo>.ndjson`, `not-here` is a fact about this pull
// request and goes to that pull request's `waivers.ndjson`.
//
// Both records here carry one key, which is what makes the assertion about the
// files rather than about the data: nothing but the disposition separates them,
// so a routing that read anything else would put them in the same file. The
// cross-checks are the half with teeth — §7.4.3 forbids a `not-here` from
// silencing a finding outside its pull request, and a `not-here` line in the
// repository file is exactly that failure, invisible because its cost is a
// finding that stops being raised.
func TestTheTwoScopesOfSection744LandInTheTwoFilesItNames(t *testing.T) {
	layout := waiverHome(t)
	wrong, notHere := theSameDefectAtTheSameCode(t)

	wide := waive(t, layout, &wrong, theProvenance())
	here := waive(t, layout, &notHere, theProvenance())
	repository, pullRequest := waiverFiles(layout)

	assert.Equal(t, []WaiverRecord{wide}, storedAt(t, repository),
		"§7.4.4: a repository-wide waiver is written to ~/.cr/waivers/<owner>/<repo>.ndjson")
	assert.Equal(t, DispositionWrong, wide.Disposition)
	assert.Equal(t, "wr1", wide.ID)

	assert.Equal(t, []WaiverRecord{here}, storedAt(t, pullRequest),
		"§7.4.4: a pull-request-scoped waiver is written to that PR's waivers.ndjson per §2.3")
	assert.Equal(t, DispositionNotHere, here.Disposition)
	assert.Equal(t, "wp1", here.ID)

	// §7.1.6 re-ingests a draft's deletions every time the draft is
	// regenerated, so the same key arrives again in the ordinary course.
	t.Run("waiving one key twice writes one line", func(t *testing.T) {
		assert.Equal(t, wide, waive(t, layout, &wrong, theProvenance()))
		assert.Equal(t, here, waive(t, layout, &notHere, theProvenance()))
		assert.Len(t, storedAt(t, repository), 1)
		assert.Len(t, storedAt(t, pullRequest), 1)
	})
}

// §7.4.4: both files MUST be readable and removable, so neither disposition is
// a one-way door.
//
// Readable is asserted through the three readers §7.4.7 and §6.4.4 will use,
// and removable from each scope in turn, with the other file required to be
// untouched — a removal that emptied both would make the door swing the wrong
// way instead of not at all. The last case is the one that keeps the deletion
// honest: an id a removed waiver held is spent, so the next waiver takes a new
// number and a listing the reviewer already read cannot come to name something
// else.
func TestAWaiverIsReadableAndRemovableFromEitherScope(t *testing.T) {
	layout := waiverHome(t)
	wrong, notHere := theSameDefectAtTheSameCode(t)
	wide := waive(t, layout, &wrong, theProvenance())
	here := waive(t, layout, &notHere, theProvenance())

	stored, err := RepositoryWaivers(layout, waiverOwner, waiverRepo)
	require.NoError(t, err)
	assert.Equal(t, []WaiverRecord{wide}, stored)

	stored, err = PullRequestWaivers(layout, waiverOwner, waiverRepo, waiverPR)
	require.NoError(t, err)
	assert.Equal(t, []WaiverRecord{here}, stored)

	stored, err = ActiveWaivers(layout, waiverOwner, waiverRepo, waiverPR)
	require.NoError(t, err)
	assert.Equal(t, []WaiverRecord{wide, here}, stored,
		"§6.4.4 drops a finding matching an active waiver of either scope, so both files are read")

	removed, err := RemoveWaiver(layout, waiverOwner, waiverRepo, waiverPR, wide.ID)
	require.NoError(t, err)
	assert.Equal(t, wide, removed)
	assert.Empty(t, storedAt(t, layout.WaiversFile(waiverOwner, waiverRepo)))
	stored, err = PullRequestWaivers(layout, waiverOwner, waiverRepo, waiverPR)
	require.NoError(t, err)
	assert.Equal(t, []WaiverRecord{here}, stored,
		"§7.4.3: removing a repository-wide waiver must not reach a pull request's own")

	removed, err = RemoveWaiver(layout, waiverOwner, waiverRepo, waiverPR, here.ID)
	require.NoError(t, err)
	assert.Equal(t, here, removed)
	assert.Empty(t, storedAt(t, layout.PRFile(waiverOwner, waiverRepo, waiverPR, state.FileWaivers)))

	t.Run("an id that names no waiver is refused", func(t *testing.T) {
		_, err := RemoveWaiver(layout, waiverOwner, waiverRepo, waiverPR, wide.ID)
		require.ErrorAs(t, err, new(*UnknownWaiverError))
		assert.ErrorContains(t, err, wide.ID,
			"the refusal must name the id, so the reviewer can see which one missed")
		assert.ErrorContains(t, err, "cr waivers list",
			"every error names the next actionable step")

		_, err = RemoveWaiver(layout, waiverOwner, waiverRepo, waiverPR, "w1")
		require.ErrorContains(t, err, "wr<n>",
			"an id names its own file, so one under neither prefix names no file to open")

		_, err = RemoveWaiver(layout, waiverOwner, waiverRepo, 0, "wp1")
		require.ErrorContains(t, err, "--pr",
			"§7.4.4 stores a pull-request-scoped waiver in that pull request's directory")
	})

	// The first pull request of a repository is the boundary that refusal
	// sits next to, and the one a reviewer would meet on a new repository.
	t.Run("the first pull request is a pull request", func(t *testing.T) {
		first := theProvenance()
		first.PR = 1
		waiver, err := WaiverFor(&notHere)
		require.NoError(t, err)
		recorded, err := Waive(layout, waiverOwner, waiverRepo, &waiver, first)
		require.NoError(t, err)

		removed, err := RemoveWaiver(layout, waiverOwner, waiverRepo, 1, recorded.ID)
		require.NoError(t, err)
		assert.Equal(t, recorded, removed)
	})

	// Nothing cites a waiver by id, so an emptied file starting again at one
	// strands no record — which is the whole of why §7.4.7 deletes where
	// §3.6.6 marks, and it is asserted rather than left to the comment.
	t.Run("an emptied file numbers from one again", func(t *testing.T) {
		assert.Equal(t, "wr1", waive(t, layout, &wrong, theProvenance()).ID)
		assert.Equal(t, "wp1", waive(t, layout, &notHere, theProvenance()).ID)
	})

	// A gap left by a removal is not filled: the count runs to the highest
	// id the file holds, so an id in it can never name two waivers at once.
	t.Run("a waiver beside a gap takes a number no line holds", func(t *testing.T) {
		second := wrong
		second.Class = "unhandled-error"
		assert.Equal(t, "wr2", waive(t, layout, &second, theProvenance()).ID)

		_, err := RemoveWaiver(layout, waiverOwner, waiverRepo, waiverPR, "wr1")
		require.NoError(t, err)

		third := wrong
		third.Class = "reinvented-helper"
		assert.Equal(t, "wr3", waive(t, layout, &third, theProvenance()).ID)
	})
}

// §7.4.8: a waiver MUST record the round, the PR, the head, and the reason when
// one was given.
//
// The four are read off the stored line by JSON key rather than off the decoded
// struct, because "when one was given" is a statement about the wire: a reason
// nobody gave and a reason given as the empty string decode alike, and only the
// absent key says which happened. The three that are not optional are asserted
// from the other side too — a waiver that recorded no round, no pull request or
// no head would be a silence nobody could account for, so it is refused rather
// than written.
func TestAWaiverRecordsTheRoundThePullRequestTheHeadAndTheReason(t *testing.T) {
	layout := waiverHome(t)
	wrong, _ := theSameDefectAtTheSameCode(t)
	waive(t, layout, &wrong, theProvenance())

	body, err := os.ReadFile(layout.WaiversFile(waiverOwner, waiverRepo))
	require.NoError(t, err)
	var written map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(body))), &written))

	for field, value := range map[string]string{
		"round":  "3",
		"pr":     "7",
		"head":   `"0a1b2c3"`,
		"reason": `"the legacy area is exempt"`,
	} {
		assert.JSONEq(t, value, string(written[field]),
			"§7.4.8: a waiver records %s", field)
	}

	t.Run("the reason is absent when none was given", func(t *testing.T) {
		layout := waiverHome(t)
		silent := theProvenance()
		silent.Reason = ""
		waive(t, layout, &wrong, silent)

		body, err := os.ReadFile(layout.WaiversFile(waiverOwner, waiverRepo))
		require.NoError(t, err)
		var written map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(string(body))), &written))
		assert.NotContains(t, written, "reason",
			"§7.4.8 asks for the reason when one was given, so none written is none given")
		for _, field := range []string{"round", "pr", "head"} {
			assert.Contains(t, written, field)
		}
	})

	t.Run("provenance §7.4.8 requires is not optional", func(t *testing.T) {
		for name, incomplete := range map[string]func(*WaiverProvenance){
			"no round":        func(p *WaiverProvenance) { p.Round = 0 },
			"no pull request": func(p *WaiverProvenance) { p.PR = 0 },
			"no head":         func(p *WaiverProvenance) { p.Head = "  " },
		} {
			t.Run(name, func(t *testing.T) {
				layout := waiverHome(t)
				prov := theProvenance()
				incomplete(&prov)
				waiver, err := WaiverFor(&wrong)
				require.NoError(t, err)

				_, err = Waive(layout, waiverOwner, waiverRepo, &waiver, prov)
				require.Error(t, err)
				assert.Empty(t, storedAt(t, layout.WaiversFile(waiverOwner, waiverRepo)),
					"a waiver §7.4.8 would not accept must not reach the file either")
			})
		}
	})
}

// §7.4.6 drops a waived finding before drafting, and §6.4.4 does the dropping
// at merge. This is the lookup that pass rests on: given the waivers of both
// scopes, does one cover this record?
//
// The two directions are the whole of the claim. It matches across both files,
// so a `wrong` waived on another pull request still silences this one and a
// `not-here` silences only this one; and it stops matching the moment the
// anchored lines change, which §7.4.2 says is exactly when the judgement behind
// the waiver should be revisited.
func TestAnActiveWaiverCoversTheRecordItWasWrittenForUntilThatCodeChanges(t *testing.T) {
	layout := waiverHome(t)
	wrong, notHere := theSameDefectAtTheSameCode(t)
	notHere.Class = "unhandled-error"

	wide := waive(t, layout, &wrong, theProvenance())
	here := waive(t, layout, &notHere, theProvenance())

	active, err := ActiveWaivers(layout, waiverOwner, waiverRepo, waiverPR)
	require.NoError(t, err)

	covering, waived := WaivedBy(active, &wrong)
	assert.True(t, waived)
	assert.Equal(t, wide, covering, "the repository-wide waiver covers its own record")

	covering, waived = WaivedBy(active, &notHere)
	assert.True(t, waived)
	assert.Equal(t, here, covering, "the pull-request-scoped waiver covers its own record")

	moved := wrong
	moved.Anchor.StartLine, moved.Anchor.Line = 400, 402
	_, waived = WaivedBy(active, &moved)
	assert.True(t, waived, "§7.4.2: the same unchanged code stays waived when the file above it moves")

	rewritten := wrong
	rewritten.Anchor.ContentHash = hashOf(t, []string{"if ($discount > 0) {", "    $total -= $discount;", "}"})
	_, waived = WaivedBy(active, &rewritten)
	assert.False(t, waived, "§7.4.2: a waiver stops suppressing once the anchored code changes")

	_, waived = WaivedBy(nil, &wrong)
	assert.False(t, waived, "no waiver covers a record when none was ever written")
}

// §7.4.4's repository-wide file is not per-PR state, so §2.3.1's lock does not
// reach it — which is the argument for a lock of its own rather than against
// one. Every pull request of a repository writes this one file, and each takes
// a different §2.3.1 lock, so nothing those locks do keeps two triages from
// publishing over each other.
//
// What that would cost is not a crash. The loser's waiver is simply gone, the
// finding it silenced comes back next round, and nothing on disk says why — the
// failure §7.4.4 calls a one-way door, running the other way. So the assertion
// is that every waiver written survives, and that each took an id of its own.
func TestARepositoryWaiverSurvivesTriageOnAnotherPullRequest(t *testing.T) {
	layout := waiverHome(t)
	wrong, _ := theSameDefectAtTheSameCode(t)

	const triages = 8
	var running sync.WaitGroup
	for i := range triages {
		running.Add(1)
		go func() {
			defer running.Done()
			// One class and one pull request each, so the waivers are
			// distinct records rather than one record written eight
			// times, which the idempotence above would collapse.
			record := wrong
			record.Class = fmt.Sprintf("waived-class-%d", i)
			prov := theProvenance()
			prov.PR = i + 1
			waiver, err := WaiverFor(&record)
			assert.NoError(t, err)
			_, err = Waive(layout, waiverOwner, waiverRepo, &waiver, prov)
			assert.NoError(t, err)
		}()
	}
	running.Wait()

	stored := storedAt(t, layout.WaiversFile(waiverOwner, waiverRepo))
	require.Len(t, stored, triages,
		"every triage's waiver must survive, whichever pull request it was written from")

	classes := make(map[string]bool, triages)
	ids := make(map[string]bool, triages)
	for i := range stored {
		classes[stored[i].Class] = true
		ids[stored[i].ID] = true
	}
	assert.Len(t, classes, triages)
	assert.Len(t, ids, triages, "each waiver must take an id of its own")
}

// §7.4.4 names two files and Waive routes to them by scope alone. The third arm
// of that routing is unreachable through Waive — Scope returns one of exactly
// two values or an error — and it is asserted anyway, because what it defends
// against is a scope added later: a routing that fell into whichever arm came
// last would write a waiver of unknown reach into one of the two files, and the
// wider of them silences findings across a repository forever.
func TestAScopeNamingNeitherFileOfSection744IsRefused(t *testing.T) {
	layout := waiverHome(t)
	wrong, _ := theSameDefectAtTheSameCode(t)
	waiver, err := WaiverFor(&wrong)
	require.NoError(t, err)
	draft := WaiverRecord{Waiver: waiver, WaiverProvenance: theProvenance()}

	_, err = appendWaiver(layout, waiverOwner, waiverRepo, WaiverScope{}, &draft)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ScopeRepository.String())
	assert.Contains(t, err.Error(), ScopePullRequest.String())

	repository, _ := waiverFiles(layout)
	assert.Empty(t, storedAt(t, repository), "a waiver of no scope must reach neither file")
}
