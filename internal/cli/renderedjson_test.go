package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// renderedOf is the round's rendered.json, as `cr draft` wrote it and as the
// next run of the command reads it back.
func renderedOf(t *testing.T, layout state.Layout) string {
	t.Helper()
	return layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileRendered)
}

// M-1.3: a rounds/<n>/rendered.json that does not decode is cr's own state cr
// cannot use, which §11.2 codes 3 with state.UnusableHint — never the usage
// floor of 2.
//
// The file is written by `cr draft` and read by the next `cr draft` of the
// round, so no invocation can be retyped to get past it and the usage hint
// would send the reader to `cr draft --help` for a file they have to repair.
// It is the class D-S11-1 found for meta.json, fixed there by 98df0c8.
//
// Measured against the unfixed tree on 2026-09-18: DecodeRendered wrapped the
// json error in a bare fmt.Errorf, no row of exit.go claimed it, and this test
// read exit code 2 with "§11.2 codes a malformed invocation 2; check the
// command line against `cr <command> --help`".
func TestACorruptRenderedJSONExitsWithTheFileCode(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f1", finding.StateDraft))
	redraft(t)
	require.NoError(t, os.WriteFile(renderedOf(t, layout), []byte("not json\n"), 0o600))

	_, err := runDraft(t, draftPR, "--repo", draftSlug)

	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err),
		"§11.2 codes a file of cr's own state that does not decode 3")
	assert.Equal(t, state.UnusableHint, hintFor(err),
		"and the step is repairing that file, never retyping the command line")
	assert.Contains(t, err.Error(), state.FileRendered,
		"the message names the file the reviewer has to repair")
}

// The negative direction: the rendered.json `cr draft` itself wrote still
// drafts, so the refusal above is the decode failing and not the read.
func TestARenderedJSONThatDecodesStillDrafts(t *testing.T) {
	layout := draftedHome(t, aStoredRecord("f1", finding.StateDraft))
	redraft(t)
	body, err := os.ReadFile(renderedOf(t, layout))
	require.NoError(t, err)
	require.Contains(t, string(body), "f1", "the round's own rendered.json keys on the record id")

	_, err = runDraft(t, draftPR, "--repo", draftSlug)

	require.NoError(t, err)
	assert.Equal(t, []string{"f1"}, markersIn(readDraft(t, layout)),
		"the record is drafted again from the entries the file holds")
}
