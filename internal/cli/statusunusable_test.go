package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// A read-only command meeting a stored findings.ndjson line it cannot decode
// refuses it the way §9.3.4's sweep does since brief-invalidate-error-context:
// the line is cr's own state under ~/.cr edited by hand, §11.2 codes it 3 with
// state.UnusableHint, and the refusal names the file and the line to open.
//
// Measured before command-level-tests-and-comments: `cr status` over such a
// line exited 2 with the usage hint, telling the user to retype a command line
// that was right.
//
// The bad line is the third, behind a blank one, so a count that skipped blank
// lines would name the wrong line.
func TestStatusNamesTheStoredFindingsLineItCannotUse(t *testing.T) {
	cause := func(body string) string {
		t.Helper()
		var into *finding.Finding
		err := json.Unmarshal([]byte(body), &into)
		require.Error(t, err)
		return err.Error()
	}
	notJSON := func(body string) string {
		t.Helper()
		var fields map[string]json.RawMessage
		err := json.Unmarshal([]byte(body), &fields)
		require.Error(t, err)
		return err.Error()
	}
	for name, bad := range map[string]struct {
		line     string
		expected func(string) string
	}{
		"a state no version names":   {`{"id":"f2","state":"bogus","round":1}`, cause},
		"an id that is not a string": {`{"id":7,"state":"queued","round":1}`, cause},
		"a line that is not JSON":    {`{"id":"f2",`, notJSON},
	} {
		t.Run(name, func(t *testing.T) {
			statusHome(t)
			layout := state.New(os.Getenv(state.HomeEnv))
			meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
			require.NoError(t, err)
			stored := `{"id":"f1","state":"draft","head":"` + meta.Head + `","round":1}` + "\n\n" +
				bad.line + "\n"
			held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
			require.NoError(t, err)
			require.NoError(t, held.Write(state.FileFindings, []byte(stored)))
			require.NoError(t, held.Unlock())

			_, err = runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)

			require.Error(t, err)
			path := layout.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
			assert.Equal(t,
				path+" line 3: cannot use findings.ndjson: "+bad.expected(bad.line), err.Error())
			assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2 codes a file cr cannot use 3")
			assert.Equal(t, state.UnusableHint, hintFor(err))
		})
	}
}

// The same refusal from a read of the whole file rather than of one round.
//
// `cr record` numbers ids over every round's records, so it decodes an earlier
// round's line that a round-scoped read skips. The bad line here is valid JSON
// in round 1 while the fixture stands in round 2, so the round-scoped reads
// pass over it and only the whole-file read meets its state.
func TestRecordNamesAnEarlierRoundsStoredLineItCannotUse(t *testing.T) {
	layout := recordedHome(t)
	stored := `{"id":"f9","state":"bogus","round":1}` + "\n"
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(stored)))
	require.NoError(t, held.Unlock())

	_, err = runRecord(t, recordPR,
		writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1")), "--repo", recordSlug)

	require.Error(t, err)
	path := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
	assert.Equal(t,
		path+" line 1: cannot use findings.ndjson: "+(&finding.UnknownStateError{Value: "bogus"}).Error(),
		err.Error())
	assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2 codes a file cr cannot use 3")
	assert.Equal(t, state.UnusableHint, hintFor(err))
}
