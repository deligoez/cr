package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// §2.2 keeps every profile at ~/.cr/profiles/<id>.json and §2.4 makes the id
// its file stem, so the profile_id meta.json records names a file of that
// directory and nothing else.
//
// QA found meta.json's profile_id joined into the profile path unchecked: with
// `../../x` recorded and a valid profile of id `x` two directories above the
// profiles directory, `cr test` loaded that profile and ran its tests.cmd.
//
// Here the out-of-tree profile is a copy of the fixture's own, whose runner
// writes the log, so a run that reached it would leave the log behind. Both
// commands that load the round's profile to run its suite are driven, and each
// is asserted to refuse with §11.2's 3, the unusable-state hint, and a message
// naming meta.json, while the log stays absent.
func TestAStoredProfileIDThatLeavesTheProfilesDirectoryIsRefused(t *testing.T) {
	commands := []struct {
		name string
		args func(t *testing.T) []string
	}{
		{"test", func(*testing.T) []string {
			return []string{"test", fixturePR, "--repo", fixtureSlug}
		}},
		{"probe_run", func(t *testing.T) []string {
			return []string{"probe", "run", fixturePR, "--repo", fixtureSlug,
				"--kind", "mutation", "--patch", writePatch(t, fixtureDiff)}
		}},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			prepared, _, _, log := probeFixture(t, "echo 'Tests:  4 passed'\n")

			body, err := os.ReadFile(prepared.Profile("qa"))
			require.NoError(t, err)
			outside := filepath.Clean(filepath.Join(prepared.ProfilesDir(), "..", "..", "x.json"))
			require.NoError(t, os.WriteFile(outside,
				[]byte(strings.Replace(string(body), `"id":"qa"`, `"id":"x"`, 1)), 0o600))

			meta, err := prepared.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
			require.NoError(t, err)
			meta.ProfileID = "../../x"
			held, err := prepared.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
			require.NoError(t, err)
			require.NoError(t, held.WriteMeta(&meta))
			require.NoError(t, held.Unlock())

			err = runCLI(t, command.args(t)...)
			require.Error(t, err)
			metaPath := prepared.PRFile(fixtureOwner, fixtureProject, fixturePRNumber, state.FileMeta)
			var file *state.FileError
			require.True(t, errors.As(err, &file), "the refusal is a file of cr's own state: %v", err)
			assert.Equal(t, "cannot use "+metaPath+": profile_id \"../../x\" is not one profile's file stem: "+
				"§2.4 makes the id the name of a file directly under ~/.cr/profiles, "+
				"so it holds no path separator and is not . or ..", err.Error())
			assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2: cr's own state it cannot use is 3")
			assert.Equal(t, state.UnusableHint, hintFor(err))
			assert.NoFileExists(t, log, "no profile's tests.cmd ran")
		})
	}
}
