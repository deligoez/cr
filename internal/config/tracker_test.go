package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/intent"
)

// §3.1.8's tracker defaults to the configured command, so every repository
// that configured nothing reads its issue exactly as before.
func TestTheTrackerDefaultsToTheCommand(t *testing.T) {
	resolved, err := Resolve(Sources{})
	require.NoError(t, err)
	assert.Equal(t, intent.TrackerCommand, resolved.String(intent.TrackerSetting))
}

// The domain is closed, and refused where it is resolved. Every caller
// compares against `github`, so a value naming neither would be read as the
// command tracker everywhere and run a jira nobody configured.
func TestAnUnknownTrackerIsRefusedNamingTheLayer(t *testing.T) {
	_, err := Resolve(Sources{Flags: map[string]any{intent.TrackerSetting: "linear"}})

	var refused *LayerError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, intent.TrackerSetting, refused.Key)
	assert.Contains(t, err.Error(), "linear")
}

// The two spellings of the domain are one: this package cannot import the
// constants it validates, so the test is what keeps them from drifting.
func TestTheTrackerDomainIsIntents(t *testing.T) {
	assert.Equal(t, intent.TrackerSetting, trackerSetting)
	assert.Equal(t, []string{intent.TrackerCommand, intent.TrackerGitHub}, trackers)
}
