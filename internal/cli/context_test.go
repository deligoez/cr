package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// runIn runs one cr command against whatever CR_HOME already points at, and
// returns what it printed and what it refused.
//
// It takes the whole command line rather than prepending one, because these
// tests record with `cr note` and `cr answer` and then read with `cr context`:
// the store one command writes has to be the store the next one finds, so all
// three run against one state root.
func runIn(t *testing.T, args ...string) (printed string, err error) {
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

// runContext runs `cr context` with args.
func runContext(t *testing.T, args ...string) (printed string, err error) {
	t.Helper()
	return runIn(t, append([]string{"context"}, args...)...)
}

// §3.6.5 asks for the notes *with provenance*, and this is what that has to
// mean: everything the store holds about a note reaches the reader.
//
// The two notes between them carry every field §3.6.1 and §3.6.2 name — a plain
// note recorded from one pull request, and an answer recorded from another,
// which is the only note that carries a record. The printed document is then
// compared against the store itself rather than against a list written here, so
// a field added to a note later is a field this test requires to be printed,
// and the fields are named individually as well, so an equality between two
// empty lists cannot pass for one.
func TestContextPrintsEveryFieldTheStoreHolds(t *testing.T) {
	layout := briefedHome(t, "CR-7")

	_, err := runIn(t, "note", "CR-7", "the deadline moved to Friday", "--source", "chat", "--pr", "9")
	require.NoError(t, err)
	_, err = runIn(t, "answer", answeredPR, "f3", "the retry is deliberate",
		"--source", "thread", "--repo", answeredSlug)
	require.NoError(t, err)

	out, err := runContext(t, "CR-7")
	require.NoError(t, err)

	var printed struct {
		IssueKey string      `json:"issue_key"`
		Notes    []note.Note `json:"notes"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &printed))
	assert.Equal(t, "CR-7", printed.IssueKey)

	stored, err := note.Load(layout, "CR-7")
	require.NoError(t, err)
	require.Len(t, stored, 2)
	assert.Equal(t, stored, printed.Notes, "§3.6.5: the store holds more than the reader was given")

	assert.Equal(t, "CR-7#n1", printed.Notes[0].ID)
	assert.Equal(t, "the deadline moved to Friday", printed.Notes[0].Text)
	assert.Equal(t, note.SourceChat, printed.Notes[0].Source)
	assert.Equal(t, 9, printed.Notes[0].PR)
	assert.False(t, printed.Notes[0].RecordedAt.IsZero(), "§3.6.1 requires a timestamp")
	assert.Empty(t, printed.Notes[0].Record, "a note that answers no record names none")

	assert.Equal(t, "CR-7#n2", printed.Notes[1].ID)
	assert.Equal(t, "f3", printed.Notes[1].Record, "§3.6.2's answer names the record it was given to")
	assert.Equal(t, answeredPRNum, printed.Notes[1].PR)
}

// §12.1's other shape for this command. A terminal reader gets a line of
// provenance and a line of text for each note, and both are asserted: the text
// is printed here where `cr note` withholds it, because this reader did not
// write these notes and may never have seen them, which is what §3.6.5 exists
// for. The provenance line names the source §8.1.6 discloses, the pull request
// the fact came from, when it was recorded, and — on §3.6.2's answer alone —
// the record it was given to.
func TestATerminalContextNamesEachNotesProvenance(t *testing.T) {
	briefedHome(t, "CR-7")

	_, err := runIn(t, "note", "CR-7", "the deadline moved to Friday", "--source", "chat", "--pr", "9")
	require.NoError(t, err)
	_, err = runIn(t, "answer", answeredPR, "f3", "the retry is deliberate",
		"--source", "thread", "--repo", answeredSlug)
	require.NoError(t, err)

	out := throughATerminal(t, "context", "CR-7")

	assert.Contains(t, out, "\x1b[36mCR-7\x1b[0m: 2 note(s)")
	assert.Contains(t, out, "\x1b[36mCR-7#n1\x1b[0m from chat on pr 9 at ")
	assert.Contains(t, out, "\n    the deadline moved to Friday\n")
	assert.Contains(t, out, "\x1b[36mCR-7#n2\x1b[0m from thread on pr 42 at ")
	assert.Contains(t, out, ", answering f3")
	assert.Contains(t, out, "\n    the retry is deliberate\n")
	assert.NotContains(t, out, "answering\n", "a note answering no record names none")
}

// brief writes the §2.3 metadata `cr brief` leaves behind for one pull request,
// so a later command can resolve the issue key it belongs to. Two pull requests
// briefed under one key is the arrangement §3.6.4's second half is about.
func briefedPR(t *testing.T, layout state.Layout, pr int, issueKey string) {
	t.Helper()
	require.NoError(t, layout.EnsurePR(answeredOwner, answeredRepo, pr))

	held, err := layout.LockPR(answeredOwner, answeredRepo, pr)
	require.NoError(t, err)
	recorded := state.Meta{Owner: answeredOwner, Repo: answeredRepo, PR: pr, IssueKey: issueKey}
	require.NoError(t, held.WriteMeta(&recorded))
	require.NoError(t, held.Unlock())
}

// §3.6.4's second half: a note recorded against one pull request is loaded for
// every subsequent pull request that resolves to the same issue key.
//
// Two pull requests are briefed under CR-7. The fact is recorded from #1 and
// never from #2, and it is then read back twice from #2's side. `cr answer 2`
// allocates CR-7#n2, which it could only have done having loaded #1's note —
// §3.6.1 numbers a note by counting the ones the store already holds, so a
// store scoped to the pull request would have given it CR-7#n1 and overwritten
// nothing anybody could see. `cr context` then prints both, and #1's note is
// still there carrying the pull request it came from, which is the other half
// of the claim: the PR field is provenance and is not read as a scope.
//
// §9.3.5 exempts this store from round scoping outright, so neither read is
// narrowed by a round either. The round half of §3.6.4 has no round machinery
// to drive yet and belongs to notes-auto-loaded; what is fixed here is the
// store's shape, which is what that task will load through.
func TestANoteRecordedAgainstOnePullRequestLoadsForAnother(t *testing.T) {
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	briefedPR(t, layout, 1, "CR-7")
	briefedPR(t, layout, 2, "CR-7")

	_, err := runIn(t, "note", "CR-7", "the deadline moved to Friday", "--source", "chat", "--pr", "1")
	require.NoError(t, err)

	answered, err := runIn(t, "answer", "2", "f3", "the retry is deliberate",
		"--source", "thread", "--repo", answeredSlug)
	require.NoError(t, err)
	assert.Contains(t, answered, `"id": "CR-7#n2"`,
		"§3.6.4: the answer on #2 numbered over #1's note, so the store loaded across pull requests")

	out, err := runContext(t, "CR-7")
	require.NoError(t, err)

	var printed struct {
		Notes []note.Note `json:"notes"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &printed))
	require.Len(t, printed.Notes, 2, "§3.6.4: a note is not scoped to the pull request it came from")
	assert.Equal(t, "the deadline moved to Friday", printed.Notes[0].Text)
	assert.Equal(t, 1, printed.Notes[0].PR, "the pull request is provenance, and it survives the read")
	assert.Equal(t, 2, printed.Notes[1].PR)
}

// An issue key nobody has recorded against holds no notes, and asking for them
// is not a failure. §3.6's store is created by the first note appended to it, so
// a key whose notes are still to come and a key mistyped at the command line are
// the same absent file: cr has nothing to tell them apart with, and reports what
// it found rather than guessing which one this was.
//
// §12.3 is the other half — an empty collection prints as [] and never as null,
// which is what the agent reading the pipe parses.
func TestContextOnAKeyNobodyRecordedAgainstIsNoNotes(t *testing.T) {
	crHome(t)

	out, err := runContext(t, "CR-404")
	require.NoError(t, err)
	assert.Contains(t, out, `"issue_key": "CR-404"`)
	assert.Contains(t, out, `"notes": []`, "§12.3: an empty collection is [] and never null")

	assert.Contains(t, throughATerminal(t, "context", "CR-404"),
		"no notes recorded against \x1b[36mCR-404\x1b[0m")
}

// §3.6.4 as a command surface that cannot narrow, which is the half of it that
// can regress. A note loads for every subsequent round and every subsequent
// pull request resolving to the same key, so there is nothing here to filter by
// — and a flag that filtered would be a way of not seeing a fact somebody
// recorded, which is exactly the loss §3.6 exists to prevent.
//
// The command therefore declares no flag of its own and takes the key alone.
// The flags are read off the command rather than exercised one by one, so a
// `--pr`, a `--round`, or a `--since` added later fails here whatever it is
// called.
func TestContextIsAddressedByTheIssueKeyAlone(t *testing.T) {
	crHome(t)

	assert.False(t, newContextCmd(&writer{}).Flags().HasFlags(),
		"§3.6.4: nothing may narrow which notes an issue key loads")

	for name, args := range map[string][]string{
		"no issue key":     {},
		"a second key":     {"CR-7", "CR-8"},
		"a pull request":   {"CR-7", "--pr", "1"},
		"a round to scope": {"CR-7", "--round", "2"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := runContext(t, args...)
			require.Error(t, err)
			assert.Equal(t, ExitUsage, exitCodeFor(err), "§11.2 codes a malformed invocation 2")
			assert.Empty(t, out, "a refused run prints no notes")
		})
	}
}

// §3.6.5 prints the notes *with provenance*, and §3.6.6's retraction is the
// most consequential provenance a note can carry: it is the fact that nothing
// may be asserted on this one. A store that printed a retracted note exactly
// as it prints a standing one would leave a reader weighing hearsay somebody
// has already withdrawn, and the retraction is what tells them not to.
//
// The retained note is the other half. §3.6.6 revokes rather than erases, so
// the withdrawn fact is still readable — which is how the reader recognises the
// records that rested on it — and it is still numbered, so no later note can
// take its id and answer a citation that was never about it.
func TestContextNamesARetractionInTheProvenance(t *testing.T) {
	briefedHome(t, "CR-7")

	_, err := runIn(t, "note", "CR-7", "the deadline moved to Friday", "--source", "chat", "--pr", "9")
	require.NoError(t, err)
	_, err = runIn(t, "note", "CR-7", "the retry limit is three", "--source", "meeting", "--pr", "9")
	require.NoError(t, err)
	_, err = runIn(t, "note", "--remove", "CR-7#n1")
	require.NoError(t, err)

	out, err := runContext(t, "CR-7")
	require.NoError(t, err)

	var printed struct {
		Notes []note.Note `json:"notes"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &printed))
	require.Len(t, printed.Notes, 2, "§3.6.6 revokes a note; it does not erase one")
	assert.Equal(t, "the deadline moved to Friday", printed.Notes[0].Text)
	require.NotNil(t, printed.Notes[0].RetractedAt)
	assert.Nil(t, printed.Notes[1].RetractedAt, "retracting one note retracts one note")

	terminal := throughATerminal(t, "context", "CR-7")
	assert.Contains(t, terminal, "\x1b[36mCR-7#n1\x1b[0m from chat on pr 9 at ")
	assert.Contains(t, terminal, ", retracted at ")
	assert.Contains(t, terminal, "\n    the deadline moved to Friday\n",
		"the withdrawn fact is still readable, which is how its records are recognised")
	assert.Equal(t, 1, strings.Count(terminal, "retracted at "),
		"the note that still stands says nothing about a retraction")
}
