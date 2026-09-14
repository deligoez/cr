package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §12.4 through `cr claims record`: a rejected claim is named by its file, its
// line and its field, so the hint sends the reader to that field on that line
// rather than to a claim the message never names.
func TestAClaimRejectionHintsAtTheLineAndFieldItNames(t *testing.T) {
	claimedHome(t)
	file := aClaimFile(t,
		`{"id":"`+claimsIssue+`#c1","text":"Refund within a day.","source":"acceptance",`+
			`"span":"refunded within a day"}`)

	err := runClaimsRecord(t, claimsPR, file, "--repo", claimsSlug, "--intent-file", anIssueFile(t))

	require.Error(t, err)
	assert.Equal(t, file+" line 1: span does not occur in the issue text; "+
		"§3.3 draws a claim from a verbatim substring of its source", err.Error())
	assert.Equal(t, "correct the field the message names on the line it names, then record the file again",
		hintFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(err))
}

// `cr note` without `--pr` says the flag is required, rather than reporting a
// pull request 0 the caller never typed.
func TestANoteWithoutPRSaysTheFlagIsRequired(t *testing.T) {
	root, out, err := runNote(t, "CR-7", "the deadline moved", "--source", "chat")

	require.Error(t, err)
	assert.Equal(t, "--pr is required: §3.6.1 records the pull request a note came from, so name it, e.g. --pr 42",
		err.Error())
	assert.Equal(t, ExitUsage, exitCodeFor(err))
	assert.Equal(t, usageHint, hintFor(err))
	assert.Empty(t, out)
	assert.NoDirExists(t, root, "nothing was recorded")
}

// `cr note --remove` given a positional reports it as an unexpected argument:
// the command has no subcommands, so "unknown command" named the wrong fault.
func TestARetractionGivenAnArgumentReportsItAsUnexpected(t *testing.T) {
	root, out, err := runNote(t, "--remove", "CR-7#n1", "extra-arg")

	require.Error(t, err)
	assert.Equal(t, "unexpected argument \"extra-arg\": `cr note --remove <note-id>` takes "+
		"the note id as the flag's value and no argument", err.Error())
	assert.Equal(t, ExitUsage, exitCodeFor(err))
	assert.Empty(t, out)
	assert.NoDirExists(t, root, "nothing was retracted")
}
