package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// The repository `cr rules suggest` is exercised against. It is scanned whole,
// so it has no pull request of its own: every fixture below names the pull
// requests it wants.
const (
	harvestOwner = "acme"
	harvestRepo  = "api"
	harvestSlug  = harvestOwner + "/" + harvestRepo
	harvestHead  = "7c6b5a4938271605f4e3d2c1b0a998877665544"
)

// aPostedComment is one comment posted from one round: the record that reached
// `posted`, and the block its draft carries for it.
type aPostedComment struct {
	pr, round int
	id, class string
	body      string
	// at is the record's §9.1 state, and §9.1's `posted` when it is left
	// zero. A fixture sets it only to seed a block the author never saw.
	at finding.State
}

// harvestedHome puts a state root behind CR_HOME holding the comments given,
// each as §9.1 leaves it and as §7.1 drafted it.
//
// The two halves are written together because the harvest reads them together:
// findings.ndjson says which records reached `posted` and what class each
// carried, and the round's draft.md says what the author received. A fixture
// that wrote one without the other would seed a comment that never happened.
func harvestedHome(t *testing.T, comments ...aPostedComment) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())

	for _, comment := range comments {
		require.NoError(t, layout.EnsurePR(harvestOwner, harvestRepo, comment.pr))
		held, err := layout.LockPR(harvestOwner, harvestRepo, comment.pr)
		require.NoError(t, err)
		at := comment.at
		if at == (finding.State{}) {
			at = finding.StatePosted
		}
		record := aStoredRecord(comment.id, at)
		record.Class = comment.class
		// §2.3.3 stamps head and round, so the round the comment was
		// posted from is written the way cr writes it rather than set
		// on the record here — which is the pair §9.3.5 scopes by and
		// the pair the scan has to cross.
		require.NoError(t, state.AppendStamped(held, state.FileFindings,
			state.Stamp{Head: harvestHead, Round: comment.round},
			[]*finding.Finding{record}))
		require.NoError(t, held.WriteRound(comment.round, state.FileDraft,
			[]byte(draftedBlock(t, layout, &comment))))
		require.NoError(t, held.Unlock())
	}
	return layout
}

// draftedBlock is the round's draft with one more §7.1 block appended.
//
// §7.1 renders every queued record of a round into one file, and this fixture
// adds them one comment at a time, so what is already there is read back and
// kept: a helper that wrote each block over the last would seed a round whose
// draft holds one comment however many were posted from it.
func draftedBlock(t *testing.T, l state.Layout, comment *aPostedComment) string {
	t.Helper()
	existing, err := l.ReadRound(
		harvestOwner, harvestRepo, comment.pr, comment.round, state.FileDraft)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		require.NoError(t, err)
	}
	return string(existing) +
		`<!-- cr:record id="` + comment.id + `" kind="finding" ` +
		`path="internal/api/handler.go" start_line="42" line="44" ` +
		`severity="high" grade="cited" disposition="" -->` + "\n\n" +
		comment.body + "\n\n"
}

// suggested runs `cr rules suggest` against whatever CR_HOME points at and
// returns the document it printed.
func suggested(t *testing.T, args ...string) rulesSuggestResult {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"rules", "suggest", "--repo", harvestSlug}, args...))
	require.NoError(t, cmd.Execute())
	var printed rulesSuggestResult
	require.NoError(t, json.Unmarshal(out.Bytes(), &printed))
	return printed
}

// recordsOf is a candidate's comments by record id and the round each was
// posted from, which is what says the scan crossed a round boundary.
func recordsOf(candidate rule.Candidate) []string {
	rendered := make([]string, 0, len(candidate.Comments))
	for _, comment := range candidate.Comments {
		rendered = append(rendered, comment.Record+"@"+itoa(comment.Round))
	}
	return rendered
}

// itoa renders a small round number without pulling strconv into a test that
// is about grouping.
func itoa(n int) string { return string(rune('0' + n)) }

// §2.6.3.1 and §2.6.3.2 through the command: three matching comments across two
// rounds are one candidate, and the comments that formed it are reported with
// it.
//
// The rounds are what make this a harvest rather than a count. §9.3.5 scopes
// every other reader to the current round, and a scan that inherited that
// scoping would see one comment in round 2 and report nothing — which is the
// failure a single-round fixture could not tell from a working scan.
//
// The fourth comment is the same words under a different class and the fifth a
// different wording under the same class, so the one candidate reported is a
// group the scan formed rather than everything it read.
func TestThreeMatchingCommentsAcrossTwoRoundsAreOneCandidate(t *testing.T) {
	harvestedHome(t,
		aPostedComment{pr: 7, round: 1, id: "f1", class: "dropped-error",
			body: "The error this call returns is dropped."},
		aPostedComment{pr: 7, round: 2, id: "f2", class: "dropped-error",
			body: "The error this   call returns is dropped.  "},
		aPostedComment{pr: 7, round: 2, id: "f3", class: "dropped-error",
			body: "The error this call returns is dropped."},
		aPostedComment{pr: 7, round: 1, id: "f4", class: "unchecked-cast",
			body: "The error this call returns is dropped."},
		aPostedComment{pr: 7, round: 2, id: "f5", class: "dropped-error",
			body: "This call's error is dropped."},
	)

	printed := suggested(t)

	assert.Equal(t, 3, printed.Min, "§2.6.3.2's default is 3")
	assert.Equal(t, 5, printed.Scanned)
	require.Len(t, printed.Candidates, 1,
		"§2.6.3.2: one group reached the threshold, and class and wording kept the rest apart")
	assert.Equal(t, "dropped-error", printed.Candidates[0].Class)
	assert.Equal(t, 3, printed.Candidates[0].Occurrences)
	assert.Equal(t, []string{"f1@1", "f2@2", "f3@2"}, recordsOf(printed.Candidates[0]),
		"§2.6.3.2 reports the comments that formed the candidate, across the rounds they came from")
}

// The scan reads posted comments and nothing else, across every pull request of
// the repository.
//
// A record that never reached `posted` is not a comment: §9.1 has `draft`,
// `duplicate` and `suppressed` all mean the author never saw it, and grouping
// them in would harvest a rule out of sentences nobody read. f4 carries the
// same class and the same body as the three that were posted and has a block of
// its own in the draft, so its state is the only thing keeping it out — a scan
// that read the drafts and not the records would count four.
//
// The three that were posted are on three different pull requests, because
// §2.6.3.1 scopes the scan to the repository: three comments spread over three
// pull requests are the same repetition as three on one, and a scan that
// stopped at a pull request boundary would report nothing here.
func TestTheScanReadsPostedCommentsAcrossEveryPullRequest(t *testing.T) {
	harvestedHome(t,
		aPostedComment{pr: 7, round: 1, id: "f1", class: "dropped-error", body: "Same words."},
		aPostedComment{pr: 9, round: 1, id: "f2", class: "dropped-error", body: "Same words."},
		aPostedComment{pr: 12, round: 1, id: "f3", class: "dropped-error", body: "Same words."},
		aPostedComment{pr: 7, round: 1, id: "f4", class: "dropped-error", body: "Same words.",
			at: finding.StateDraft},
	)

	printed := suggested(t)

	assert.Equal(t, 3, printed.Scanned,
		"§9.1: the record still in `draft` is not a comment the author received")
	require.Len(t, printed.Candidates, 1)
	assert.Equal(t, []string{"f1@1", "f2@1", "f3@1"}, recordsOf(printed.Candidates[0]),
		"§2.6.3.1 scopes the scan to the repository, so the three pull requests are one group")
}

// §2.6.3.3: the command reports and writes no rule file.
//
// The whole rules directory is fingerprinted rather than one expected path,
// because the requirement is that cr writes no rule file at all and a check
// naming the file it would have written could only refuse the one shape that
// was imagined.
func TestSuggestWritesNoRuleFile(t *testing.T) {
	layout := harvestedHome(t,
		aPostedComment{pr: 7, round: 1, id: "f1", class: "dropped-error", body: "Same words."},
		aPostedComment{pr: 7, round: 2, id: "f2", class: "dropped-error", body: "Same words."},
		aPostedComment{pr: 7, round: 3, id: "f3", class: "dropped-error", body: "Same words."},
	)
	before := rulesTree(t, layout)

	printed := suggested(t)

	require.Len(t, printed.Candidates, 1, "a candidate was found, so there was something to write")
	assert.Equal(t, before, rulesTree(t, layout),
		"§2.6.3.3: candidates are reported only; cr must not write a rule file by itself")
}

// rulesTree is every path under both rule directories, so a file written
// anywhere in either is visible whatever it is called.
func rulesTree(t *testing.T, l state.Layout) []string {
	t.Helper()
	found := make([]string, 0)
	for _, dir := range []string{l.RulesDir(), l.RepoRulesDir(harvestOwner, harvestRepo)} {
		require.NoError(t, filepath.WalkDir(dir,
			func(path string, _ os.DirEntry, err error) error {
				if os.IsNotExist(err) {
					return nil
				}
				if err != nil {
					return err
				}
				found = append(found, path)
				return nil
			}))
	}
	return found
}
