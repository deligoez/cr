package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// §9.3.4's sweep reads every stored findings.ndjson line once the head moved,
// and a line it cannot use is cr's own state under ~/.cr edited by hand. §11.2
// codes that 3, as it codes a corrupt context store and a triage ledger holding
// a sixth action, and the refusal names the file and the line to open.
//
// Measured before brief-invalidate-error-context: a stored line whose state was
// "bogus" failed `cr brief` naming neither, and exited 2 with the usage hint —
// telling the user to retype a command line that was right.
//
// The bad line is the third, behind a blank one, so a count that skipped blank
// lines would name the wrong line; and the open record on the first line shows
// the sweep writes nothing when a later line refuses.
func TestABriefOnAMovedHeadNamesTheStoredLineItCannotUse(t *testing.T) {
	cause := func(body string, into any) string {
		t.Helper()
		err := json.Unmarshal([]byte(body), into)
		require.Error(t, err)
		return err.Error()
	}
	var (
		text   string
		fields map[string]json.RawMessage
	)
	for name, bad := range map[string]struct {
		line     string
		expected string
	}{
		"a state no version names": {
			`{"id":"f2","state":"bogus","round":1}`,
			"cannot use the state field of findings.ndjson: " +
				(&finding.UnknownStateError{Value: "bogus"}).Error(),
		},
		"an id that is not a string": {
			`{"id":7,"state":"queued","round":1}`,
			"cannot use the id field of findings.ndjson: " + cause(`7`, &text),
		},
		// Measured before stale-sweep-refuses-null-id: a null decodes into
		// a string as "" with no error, so the brief exited 0, rewrote the
		// line to stale and journaled draft to stale for record "".
		"an id that is null": {
			`{"id":null,"state":"draft","round":1}`,
			"cannot use the id field of findings.ndjson: null is not a string",
		},
		// encoding/json binds both keys to one field, so no one value is
		// the state every read of the line decodes.
		"a state given twice under case folding": {
			`{"id":"f2","state":"posted","State":"draft","round":1}`,
			"cannot use findings.ndjson: state is given more than once",
		},
		"a line that is not JSON": {
			`{"id":"f2",`,
			"cannot use findings.ndjson: " + cause(`{"id":"f2",`, &fields),
		},
	} {
		t.Run(name, func(t *testing.T) {
			layout, recorded, _ := aRoundTheHeadOutran(t)
			stored := `{"id":"f1","state":"draft","head":"` + recorded + `","round":1}` + "\n\n" +
				bad.line + "\n"
			held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
			require.NoError(t, err)
			require.NoError(t, held.Write(state.FileFindings, []byte(stored)))
			require.NoError(t, held.Unlock())
			journalPath := layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileTransitions)
			journal, err := os.ReadFile(journalPath)
			require.NoError(t, err)
			issue := filepath.Join(t.TempDir(), "issue.txt")
			require.NoError(t, os.WriteFile(issue, []byte("Retry on 5xx.\n"), 0o600))

			_, err = runCLIPrinting(t, "brief", fixturePR, "--issue", fixtureIssue,
				"--intent-file", issue, "--repo", fixtureSlug)

			require.Error(t, err)
			path := layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
			assert.Equal(t, path+" line 3: "+bad.expected, err.Error())
			assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2 codes a file cr cannot use 3")
			assert.Equal(t, state.UnusableHint, hintFor(err))
			kept, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, stored, string(kept), "a refused sweep writes nothing")
			after, err := os.ReadFile(journalPath)
			require.NoError(t, err)
			assert.Equal(t, string(journal), string(after), "a refused sweep journals nothing")
		})
	}
}

// §9.3.4's sweep reads a stored line's keys the way every read of the line
// binds them. encoding/json matches a key to a field case-insensitively, so
// `cr status` reads `"State":"queued"` as a queued record, and a sweep looking
// the key up by its exact spelling would leave open a record every reader calls
// open. The rewritten line keeps the spellings it was stored under.
//
// Measured before stale-sweep-refuses-null-id: the brief exited 0 and left f2
// queued, with no journal line for it.
func TestABriefOnAMovedHeadSweepsARecordWhoseKeysAreSpelledInAnotherCase(t *testing.T) {
	layout, recorded, moved := aRoundTheHeadOutran(t)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(
		`{"id":"f1","state":"draft","head":"`+recorded+`","round":1}`+"\n\n"+
			`{"ID":"f2","State":"queued","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	issue := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte("Retry on 5xx.\n"), 0o600))

	_, err = runCLIPrinting(t, "brief", fixturePR, "--issue", fixtureIssue,
		"--intent-file", issue, "--repo", fixtureSlug)

	require.NoError(t, err)
	kept, err := os.ReadFile(layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings))
	require.NoError(t, err)
	assert.Equal(t,
		`{"head":"`+recorded+`","id":"f1","round":1,"state":"stale"}`+"\n"+
			`{"ID":"f2","State":"stale","round":1}`+"\n",
		string(kept))
	journal, err := state.ReadRecords[finding.Transition](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileTransitions)
	require.NoError(t, err)
	require.Len(t, journal, 2, "one line per record the sweep moved")
	for i, want := range []struct {
		record string
		from   finding.State
	}{{"f1", finding.StateDraft}, {"f2", finding.StateQueued}} {
		assert.Equal(t, want.record, journal[i].Record)
		require.NotNil(t, journal[i].From)
		assert.Equal(t, want.from, *journal[i].From)
		assert.Equal(t, finding.StateStale, journal[i].To)
		assert.Equal(t, moved, journal[i].Head)
	}
}
