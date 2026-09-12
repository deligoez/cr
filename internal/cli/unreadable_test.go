package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §11.2: an input file the caller named and cr cannot read is a file failure.
// Measured before unreadable-input-exit-code on the built binary, `cr record 1
// missing.ndjson` printed the bare *fs.PathError and exited 2.
func TestAnUnreadableInputExitsWithTheFileCode(t *testing.T) {
	recordedHome(t)
	missing := filepath.Join(t.TempDir(), "merged.ndjson")

	_, err := runRecord(t, recordPR, missing, "--repo", recordSlug)

	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Contains(t, err.Error(), missing)
	assert.Contains(t, hintFor(err), "`cr merge`")
}

// A state file the command requires inside a briefed round answers the same
// way, and names the command that writes it. Measured: `cr record` on a round
// with no mapping.ndjson exited 2.
func TestAMissingStateFileExitsWithTheFileCodeAndNamesItsWriter(t *testing.T) {
	layout := recordedHome(t)
	require.NoError(t, os.Remove(layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileMapping)))

	_, err := runRecord(t, recordPR, writeRecordFile(t, "merged.ndjson"), "--repo", recordSlug)

	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Contains(t, hintFor(err), "`cr map record`")
}

// A §2.3 write that cannot land exits 3 too. The pull-request directory is made
// read-only after the round is briefed, so every read succeeds and only the
// write refuses — measured before, `cr record` there exited 2.
func TestAWriteThatCannotLandExitsWithTheFileCode(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes through a read-only directory")
	}
	layout := recordedHome(t)
	dir := layout.PRDir(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := runRecord(t, recordPR,
		writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1")), "--repo", recordSlug)

	require.Error(t, err)
	var file *state.FileError
	require.ErrorAs(t, err, &file)
	assert.Equal(t, ExitFile, exitCodeFor(err))
}

// §7.3.1 closes the triage vocabulary at five names, and the ledger is a file
// under ~/.cr a user can edit. A sixth is that file unusable as written, which
// §11.2 codes 3 as it does a corrupt context store. Measured before: a ledger
// holding `retracted` exited 2.
func TestATriageLedgerWithASixthActionExitsWithTheFileCode(t *testing.T) {
	layout := statsHome(t)
	ledger := layout.RepoTriage(statsOwner, statsRepo)
	require.NoError(t, os.MkdirAll(filepath.Dir(ledger), 0o700))
	require.NoError(t, os.WriteFile(ledger,
		[]byte(`{"record":"f9","action":"retracted","class":"x","pr":7,"round":1}`+"\n"), 0o600))

	cmd := newRootCmd()
	cmd.SetOut(&discard{})
	cmd.SetErr(&discard{})
	cmd.SetArgs([]string{"stats", "--repo", statsSlug})
	err := cmd.Execute()

	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Contains(t, err.Error(), ledger)
	assert.Contains(t, err.Error(), `"retracted"`)
}

// §8.4.4's two refusals of posted.json are one answer: cr's own file, found and
// parsed, that cannot be used for the match. §11.2 codes it 3 beside a corrupt
// context store — not 2, because the command line is right, and not 1,
// because the payload is not the agent's input.
func TestAPostedPayloadCrCannotMatchExitsWithTheFileCode(t *testing.T) {
	for name, body := range map[string]string{
		"no comment":         `{"comments":[],"records":[]}`,
		"records mismatched": `{"comments":[{"path":"a.go","body":"b"}],"records":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			layout := recordedHome(t)
			held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
			require.NoError(t, err)
			require.NoError(t, held.WriteRound(recordRound, state.FilePosted, []byte(body)))
			require.NoError(t, held.Unlock())
			round, err := layout.ReadMeta(recordOwner, recordRepo, recordPRNum)
			require.NoError(t, err)

			_, err = postedPayload(layout, &round)

			require.Error(t, err)
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, state.UnusableHint, hintFor(err))
		})
	}
}

// discard is an io.Writer that keeps nothing.
type discard struct{}

func (*discard) Write(p []byte) (int, error) { return len(p), nil }
