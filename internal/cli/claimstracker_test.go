package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/intent"
)

// `cr claims record` reads the issue for the key its round recorded, so a
// tracker that refuses is answered with the one flag this command has.
//
// Release QA on deligoez/cr-qa#1 met the fault: run without `--intent-file`,
// the command started the tracker, the tracker answered 404, and the failure
// told the reader to pass `--issue <KEY>` — a flag `cr brief` has and this
// command refuses. The error and the hint are asserted whole, as the document
// §12.4 prints them, so a sentence naming `--issue` anywhere fails the test.
func TestClaimsRecordNamesOnlyTheFlagItHasWhenTheTrackerRefuses(t *testing.T) {
	claimedHome(t)
	tracker := filepath.Join(t.TempDir(), "tracker")
	require.NoError(t, os.WriteFile(tracker,
		[]byte("#!/bin/sh\necho 'HTTP 404: Not Found' >&2\nexit 1\n"), 0o700))
	t.Setenv("CR_INTENT_CMD", `["`+tracker+`","issue","view","{key}"]`)
	file := aClaimFile(t,
		`{"id":"`+claimsIssue+`#c1","text":"Back off.","source":"acceptance",`+
			`"span":"backs off exponentially"}`,
	)

	args := []string{"claims", "record", claimsPR, file, "--repo", claimsSlug}
	err := runClaimsRecord(t, args[2:]...)

	var refused *intent.CommandError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, ExitFile, exitCodeFor(err), "§3.1.3: a refusing tracker exits 3")

	var stderr bytes.Buffer
	require.NoError(t, reportFailure(&bytes.Buffer{}, &stderr, args, err))
	var document failure
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &document))
	assert.Equal(t, failure{
		Error: tracker + " issue view " + claimsIssue + ": exit status 1: HTTP 404: Not Found; " +
			"§3.1.4's `--intent-file <path>` supplies the issue text without running this command at all",
		Hint: "the tracker command failed; run it yourself, or pass `--intent-file` " +
			"to read the issue from a file per §3.1.4",
	}, document, "§12.4: the next step names --intent-file, which the command accepts, and never --issue")
}
