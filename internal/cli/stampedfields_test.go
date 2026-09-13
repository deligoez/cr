package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §2.3.3 through the three recording commands cr cells record's test does not
// already drive: `cr claims record`, `cr map record` and `cr record` each
// refuse a line supplying head, and a line supplying round, with §6.1.4's exit
// code 1 naming the file, the line and the field, and leave the file they
// would have written as they found it.
//
// Before command-level-tests-and-comments only `cr cells record` was driven
// with a supplied head, and no command with a supplied round; the other three
// inherit state.DecodeStamped's refusal, which was tested inside mapping,
// intent and finding rather than through the command a caller runs.
func TestTheRecordingCommandsRefuseASuppliedHeadOrRound(t *testing.T) {
	type command struct {
		// home builds the round and returns the stored file the command
		// writes.
		home func(t *testing.T) (layout state.Layout, stored string)
		// run hands the command the file at path.
		run func(t *testing.T, path string) error
		// line is a valid line of the command's input, missing only the
		// supplied field.
		line map[string]any
	}
	commands := map[string]command{
		"claims record": {
			home: func(t *testing.T) (state.Layout, string) {
				t.Helper()
				layout := claimedHome(t)
				return layout, layout.PRFile(claimsOwner, claimsRepo, claimsPRNum, state.FileClaims)
			},
			run: func(t *testing.T, path string) error {
				t.Helper()
				return runClaimsRecord(t, claimsPR, path,
					"--repo", claimsSlug, "--intent-file", anIssueFile(t))
			},
			line: map[string]any{
				"id": claimsIssue + "#c1", "text": "Back off exponentially.",
				"source": "acceptance", "span": "backs off exponentially",
			},
		},
		"map record": {
			home: func(t *testing.T) (state.Layout, string) {
				t.Helper()
				layout := briefedForMapping(t)
				return layout, layout.PRFile(mapOwner, mapRepo, mapPR, state.FileMapping)
			},
			run: func(t *testing.T, path string) error {
				t.Helper()
				return runCLI(t, "map", "record", strconv.Itoa(mapPR), path, "--repo", mapSlug)
			},
			line: map[string]any{"claim": mapIssue + "#c1", "unit": "u1"},
		},
		"record": {
			home: func(t *testing.T) (state.Layout, string) {
				t.Helper()
				layout := recordedHome(t)
				return layout, layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
			},
			run: func(t *testing.T, path string) error {
				t.Helper()
				_, err := runRecord(t, recordPR, path, "--repo", recordSlug)
				return err
			},
			line: aRecord("f1", "u1"),
		},
	}
	supplied := map[string]any{"head": recordHead, "round": 2}

	for name, cmd := range commands {
		// The control: the same line without either field is accepted, so
		// the refusal below is the supplied field's and nothing else's.
		t.Run(name+" supplying neither", func(t *testing.T) {
			_, stored := cmd.home(t)
			require.NoError(t, cmd.run(t, writeRecordFile(t, filepath.Base(stored), cmd.line)))
		})
		for field, value := range supplied {
			t.Run(name+" supplying "+field, func(t *testing.T) {
				_, stored := cmd.home(t)
				before, err := os.ReadFile(stored)
				require.NoError(t, err)
				line := make(map[string]any, len(cmd.line)+1)
				for key, v := range cmd.line {
					line[key] = v
				}
				line[field] = value
				path := writeRecordFile(t, filepath.Base(stored), line)

				err = cmd.run(t, path)

				var reserved *state.ReservedFieldError
				require.ErrorAs(t, err, &reserved, "§6.1.4's refusal of a field cr writes")
				assert.Equal(t, &state.ReservedFieldError{File: path, Line: 1, Field: field}, reserved)
				assert.Equal(t, ExitValidation, exitCodeFor(err))
				after, err := os.ReadFile(stored)
				require.NoError(t, err)
				assert.Equal(t, string(before), string(after), "the refused file leaves the round as it found it")
			})
		}
	}
}
