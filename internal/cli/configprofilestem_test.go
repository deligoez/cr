package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
)

// A configured `profile` that is not one profile file stem is refused with
// exit 3 naming the layer and the key, before any profile is loaded.
//
// meta.json's profile_id is refused where it is read (0e5eaa6), and QA found
// the configured value still joined into a path unchecked: `profile: ../x`
// loaded ~/.cr/x.json, from outside the profiles directory, and ran its
// tests.cmd. The file the value names exists here and does not parse, so a
// run that loaded it would fail as a malformed profile rather than as the
// configuration refusal asserted.
func TestAConfiguredProfileThatIsNotAFileStemIsRefused(t *testing.T) {
	const reason = "is not one profile's file stem: §2.4 makes a profile's id the name of a file directly " +
		"under ~/.cr/profiles, so it is not absolute, holds no path separator, and is neither . nor .. alone"
	for _, fixture := range []struct {
		name, value, layer string
		// env sets the value through CR_PROFILE rather than a file.
		env bool
	}{
		{name: "a parent directory in the per-repository config", value: "../x", layer: config.LayerRepoConfig},
		{name: "an absolute path in the per-repository config", value: "/tmp/x", layer: config.LayerRepoConfig},
		{name: "a nested path in the environment", value: "nested/x", layer: config.LayerEnv, env: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := crHome(t)
			require.NoError(t, os.WriteFile(filepath.Join(root, "x.json"), []byte("{not a profile"), 0o600))
			source := filepath.Join(root, "repos", "acme", "web", "config.json")
			if fixture.env {
				t.Setenv("CR_PROFILE", fixture.value)
				source = "CR_PROFILE"
			} else {
				require.NoError(t, os.MkdirAll(filepath.Dir(source), 0o700))
				require.NoError(t, os.WriteFile(source,
					[]byte(fmt.Sprintf(`{"profile": %q}`, fixture.value)), 0o600))
			}
			issue := filepath.Join(t.TempDir(), "issue.txt")
			require.NoError(t, os.WriteFile(issue, []byte("CR-1: retry the upload\n"), 0o600))

			printed, err := runIn(t, "brief", "1", "--repo", "acme/web", "--issue", "CR-1", "--intent-file", issue)
			require.Error(t, err)
			assert.Empty(t, printed)
			assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2 codes a configuration failure 3")

			var layer *config.LayerError
			require.ErrorAs(t, err, &layer)
			assert.Equal(t, config.Origin{From: fixture.layer, Source: source}, layer.Origin)
			assert.Equal(t, "profile", layer.Key)
			assert.Equal(t, fmt.Sprintf("%q %s", fixture.value, reason), layer.Err.Error())
		})
	}
}
