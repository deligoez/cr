package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §3.5.3 gives `threads.proximity_lines` a default of 10, and §2.7 makes the
// built-in default the lowest layer, so a run that configured nothing attaches
// threads within ten lines of a unit's hunks — and a layer above it moves the
// window.
func TestTheThreadProximityWindowDefaultsToTenAndIsSettable(t *testing.T) {
	defaults, err := Resolve(Sources{})
	require.NoError(t, err)
	assert.Equal(t, 10, defaults.Int("threads.proximity_lines"))

	moved, err := Resolve(Sources{Environ: []string{"CR_THREADS_PROXIMITY_LINES=3"}})
	require.NoError(t, err)
	assert.Equal(t, 3, moved.Int("threads.proximity_lines"))
}
