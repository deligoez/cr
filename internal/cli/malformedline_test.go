package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// malformedLineHint is the step exit.go's row gives a caller-supplied line
// that does not decode, spelled out here so a row that lost it, or an error
// that fell through to the usage hint, is caught by value.
const malformedLineHint = "correct the line the message names so it is one JSON object whose fields carry " +
	"the types the command's record schema gives them"

// heldBytes reads a file the command would have written, reading an absent one
// as empty, so a refusal can be shown to have left it as it was.
func heldBytes(t *testing.T, path string) string {
	t.Helper()
	held, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	require.NoError(t, err)
	return string(held)
}

// §11.2 through the five commands that read an NDJSON file a caller hands
// them: a line that is not JSON, and a line giving a field the wrong type, are
// input data, which §11.2 codes 1. Before this, state.DecodeStamped refused
// both in a plain error no exit.go row claimed, so each command exited 2 with
// the hint to check a command line that was right.
//
// Each refusal names the file and the line, counting the blank line above it,
// carries the decode's own error, takes the row's hint, and leaves the file the
// command writes as it found it. The stored-file half stays §11.2's 3 and is
// held by statusunusable_test.go and unreadable_test.go.
func TestAMalformedInputLineExitsWithTheValidationCode(t *testing.T) {
	type command struct {
		// home builds the round and returns the file the command writes.
		home func(t *testing.T) (written string)
		// run hands the command the file at path, and home's file where
		// the command takes it as an argument.
		run func(t *testing.T, path, written string) error
		// name is the input file's name, which `cr merge` binds to a role.
		name string
		// typed is a field of the command's record that takes a string.
		typed string
	}
	commands := map[string]command{
		"record": {
			home: func(t *testing.T) string {
				t.Helper()
				return recordedHome(t).PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
			},
			run: func(t *testing.T, path, _ string) error {
				t.Helper()
				_, err := runRecord(t, recordPR, path, "--repo", recordSlug)
				return err
			},
			name: "merged.ndjson", typed: "id",
		},
		"merge": {
			home: func(t *testing.T) string {
				t.Helper()
				recordedHome(t)
				return filepath.Join(t.TempDir(), "merged.ndjson")
			},
			run: func(t *testing.T, path, written string) error {
				t.Helper()
				// -o is the file home returned, so its absence afterwards
				// shows the merge wrote nothing.
				_, err := runMergeCLI(t, written, path)
				return err
			},
			name: finding.FanOutFile("correctness"), typed: "id",
		},
		"claims record": {
			home: func(t *testing.T) string {
				t.Helper()
				return claimedHome(t).PRFile(claimsOwner, claimsRepo, claimsPRNum, state.FileClaims)
			},
			run: func(t *testing.T, path, _ string) error {
				t.Helper()
				return runClaimsRecord(t, claimsPR, path, "--repo", claimsSlug, "--intent-file", anIssueFile(t))
			},
			name: "claims.ndjson", typed: "id",
		},
		"map record": {
			home: func(t *testing.T) string {
				t.Helper()
				return briefedForMapping(t).PRFile(mapOwner, mapRepo, mapPR, state.FileMapping)
			},
			run: func(t *testing.T, path, _ string) error {
				t.Helper()
				return runCLI(t, "map", "record", strconv.Itoa(mapPR), path, "--repo", mapSlug)
			},
			name: "mapping.ndjson", typed: "claim",
		},
		"cells record": {
			home: func(t *testing.T) string {
				t.Helper()
				return briefedForCells(t).PRFile(cellsOwner, cellsRepo, cellsPR, state.FileCoverage)
			},
			run: func(t *testing.T, path, _ string) error {
				t.Helper()
				return runCLI(t, "cells", "record", strconv.Itoa(cellsPR), path, "--repo", cellsSlug)
			},
			name: "cells.ndjson", typed: "unit",
		},
	}

	for name, cmd := range commands {
		for shape, tc := range map[string]struct {
			line string
			// cause checks the decode's own error the refusal carries.
			cause func(t *testing.T, err error)
		}{
			"not JSON": {
				line: `{"id":`,
				cause: func(t *testing.T, err error) {
					t.Helper()
					var syntax *json.SyntaxError
					assert.ErrorAs(t, err, &syntax, "the line is cut off inside its object")
				},
			},
			"a field of the wrong type": {
				line: `{"` + cmd.typed + `":7}`,
				cause: func(t *testing.T, err error) {
					t.Helper()
					var mistyped *json.UnmarshalTypeError
					require.ErrorAs(t, err, &mistyped, "the field takes a string and was given a number")
					assert.Equal(t, cmd.typed, mistyped.Field)
					assert.Equal(t, "number", mistyped.Value)
				},
			},
		} {
			t.Run(name+" over a line that is "+shape, func(t *testing.T) {
				written := cmd.home(t)
				before := heldBytes(t, written)
				path := filepath.Join(t.TempDir(), cmd.name)
				require.NoError(t, os.WriteFile(path, []byte("\n"+tc.line+"\n"), 0o600))

				err := cmd.run(t, path, written)

				var malformed *state.MalformedLineError
				require.ErrorAs(t, err, &malformed)
				assert.Equal(t, path, malformed.File)
				assert.Equal(t, 2, malformed.Line, "the blank line above it is counted")
				tc.cause(t, malformed.Err)
				assert.Equal(t, ExitValidation, exitCodeFor(err))
				assert.Equal(t, malformedLineHint, hintFor(err))
				assert.Equal(t, before, heldBytes(t, written), "the refused file leaves the round as it found it")
			})
		}
	}
}

// §11.2 through `cr probe run --kind mutation`: a `--patch` that is not a
// unified diff is input data, coded 1, and the refusal names the file and,
// where one line is at fault, that line. Before this, both shapes left the
// command in a plain error and exited 2 with the usage hint.
func TestAPatchThatIsNotADiffExitsWithTheValidationCode(t *testing.T) {
	const hint = "correct the `--patch` file at the line the message names so it is a unified diff " +
		"with --- and +++ headers and @@ hunks; if it came from `git diff`, re-run it with --no-ext-diff"
	for name, tc := range map[string]struct {
		patch string
		// line is the patch line the refusal names, zero for one about
		// the patch as a whole.
		line    int
		problem string
	}{
		"prose with no hunk": {
			patch: "this is not a diff\n",
			problem: "holds no hunk: §5.3.1's mutation is a unified diff against a sandbox file; " +
				"if it came from `git diff`, re-run it with --no-ext-diff, " +
				"which is what a configured diff.external replaces",
		},
		"a hunk line with no marker": {
			patch:   "--- a/app.go\n+++ b/app.go\n@@ -1 +1 @@\nno marker\n",
			line:    4,
			problem: `"no marker" is not a hunk line`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")
			patch := writePatch(t, tc.patch)

			err := runCLI(t, "probe", "run", fixturePR, "--repo", fixtureSlug,
				"--kind", "mutation", "--patch", patch)

			var malformed *git.MalformedPatchError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, &git.MalformedPatchError{File: patch, Line: tc.line, Problem: tc.problem}, malformed)
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, hint, hintFor(err))
			assert.NoFileExists(t, log, "the refusal comes before any suite is run")
		})
	}
}
