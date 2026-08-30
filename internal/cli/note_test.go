package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// runNote runs `cr note` with args against a fresh state root and returns the
// root, whatever it printed, and whatever it refused.
func runNote(t *testing.T, args ...string) (root, printed string, err error) {
	t.Helper()
	root = filepath.Join(t.TempDir(), ".cr")
	t.Setenv(state.HomeEnv, root)

	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"note"}, args...))
	// Executed first and read after: a return statement evaluates its
	// operands left to right, so reading the buffer in it would read what
	// the command printed before it ran.
	err = cmd.Execute()
	return root, out.String(), err
}

// §3.6.1 through the command: the note lands in §2.2's context store, keyed by
// the issue and not by the pull request, carrying the id, the timestamp, the
// pull request, and the source.
func TestNoteCommandRecordsAgainstTheIssueKey(t *testing.T) {
	root, out, err := runNote(t,
		"CR-1", "the deadline moved to Friday", "--source", "chat", "--pr", "42")
	require.NoError(t, err)

	var printed struct {
		Note note.Note `json:"note"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &printed))
	assert.Equal(t, "CR-1#n1", printed.Note.ID)
	assert.Equal(t, note.SourceChat, printed.Note.Source)
	assert.Equal(t, 42, printed.Note.PR)
	assert.False(t, printed.Note.RecordedAt.IsZero(), "§3.6.1 requires a timestamp")

	stored, err := os.ReadFile(state.New(root).ContextFile("CR-1"))
	require.NoError(t, err)
	assert.Contains(t, string(stored), "the deadline moved to Friday")
	assert.Contains(t, string(stored), `"id":"CR-1#n1"`)
}

// §12.1's other shape for this command. A terminal reader gets the id and the
// source, not the document: §3.6.6 makes a note revocable by id, so the id is
// the one thing the run produced that the user does not already have, and
// §8.1.6 will disclose the source in every posted body resting on the note.
func TestATerminalNoteNamesTheIdAndTheSource(t *testing.T) {
	crHome(t)

	out := throughATerminal(t, "note", "CR-1", "the deadline moved", "--source", "chat", "--pr", "42")

	assert.Contains(t, out, "recorded ")
	assert.Contains(t, out, "\x1b[36mCR-1#n1\x1b[0m", "the id is accented, as every terminal rendering accents its answer")
	assert.Contains(t, out, " from chat")
}

// The round-11 finding unspecified-flag-requiredness: §3.6.1 shows `--source`
// in its synopsis and never says it is required, so cr says it. An absent value
// and one outside §3.6.3's set are both the invocation being wrong rather than
// data in a file being wrong, which §11.2 codes 2 and not 1 — and an unmapped
// error is already exactly that in exitCodeFor, so neither needs a mapping of
// its own. `--pr` goes the same way: §3.6.1 has the note record the pull
// request it came from, and cr forms no opinion about which one that was.
func TestNoteRefusesAnAbsentOrUnlistedSourceWithTheUsageCode(t *testing.T) {
	for name, args := range map[string][]string{
		"no source at all":  {"CR-1", "hearsay", "--pr", "42"},
		"an empty source":   {"CR-1", "hearsay", "--source", "", "--pr", "42"},
		"an unlisted value": {"CR-1", "hearsay", "--source", "gossip", "--pr", "42"},
		"no pull request":   {"CR-1", "hearsay", "--source", "chat"},
		"no text":           {"CR-1", "--source", "chat", "--pr", "42"},
		"no issue key":      {"--source", "chat", "--pr", "42"},
	} {
		t.Run(name, func(t *testing.T) {
			root, out, err := runNote(t, args...)
			require.Error(t, err)
			assert.Equal(t, ExitUsage, exitCodeFor(err),
				"§11.2 codes a malformed invocation 2")
			assert.Empty(t, out, "a refused run prints no note")
			assert.NoFileExists(t, state.New(root).ContextFile("CR-1"))
		})
	}
}

// The round-9 finding retracted-provenance-still-posts, as far as v0.1's
// machinery reaches today.
//
// A reviewer records a note, the round's record is built on it, and the
// reviewer then learns the note was wrong and retracts it — mid-round, with the
// record already recorded. §3.6.6 and the finding together require the
// retraction to bite here rather than in the next round: the dependent record
// is forced to a question, is reported, and cannot be posted as a finding.
// Deferring any of that to the next round is not a delay but a loss, because
// v0.1 ends at posting per §9 and the next round may never come for this pull
// request.
//
// What is asserted is the decision every one of those consequences reads.
// note.StandingOf is derived from the store at the moment it is asked rather
// than stamped onto the citing record, so there is no round for a retraction to
// be late for, and no consumer can hold a standing that predates it. Before the
// retraction the citation stands and may assert; immediately after it, in the
// same round and against the same store, it may not — and it has no provenance
// left for §8.1.6 to disclose either, which is the same fact rather than a
// second one.
//
// The record itself is not driven from here, because there is none to drive:
// `cr record`, `cr draft`, `cr post`, `cr status` and the round summary are all
// still to be written, and a draft written here to give this test something to
// fail against would test itself. Those assertions are named in this task's
// closure against the tasks that own them.
func TestRetractingANoteMidRoundStopsTheRecordRestingOnIt(t *testing.T) {
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())

	_, err := runIn(t, "note", "CR-9", "the retry limit is three", "--source", "chat", "--pr", "5")
	require.NoError(t, err)

	// The record recorded this round cites the note by id, which is all
	// §3.3.2 gives it and all §8.1.6 has to disclose.
	const cited = "CR-9#n1"

	before, err := note.Load(layout, "CR-9")
	require.NoError(t, err)
	require.Equal(t, note.StandingStands, note.StandingOf(before, cited))
	require.True(t, note.StandingOf(before, cited).Stands())

	out, err := runIn(t, "note", "--remove", cited)
	require.NoError(t, err)

	var printed struct {
		Note     note.Note     `json:"note"`
		Standing note.Standing `json:"standing"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &printed))
	assert.Equal(t, cited, printed.Note.ID)
	assert.Equal(t, note.StandingRetracted, printed.Standing)
	require.NotNil(t, printed.Note.RetractedAt, "§3.6.6's retraction is on the record")

	after, err := note.Load(layout, "CR-9")
	require.NoError(t, err)
	assert.Equal(t, note.StandingRetracted, note.StandingOf(after, cited))
	assert.False(t, note.StandingOf(after, cited).Stands(),
		"the record resting on it may no longer assert, in this round and not the next")

	require.Len(t, after, 1, "the note is retained, so the decision stays auditable")
	assert.Equal(t, "the retry limit is three", after[0].Text)
	assert.Equal(t, "CR-9#n2", note.NextID("CR-9", after),
		"and the id stays spent, so no later note can answer this record's citation")
}
