package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §4.3.2's `reinvention.min_similarity` is the table's one fractional setting,
// with a default of 0.6, and every layer that can supply it has to arrive as
// the same type. A file supplies a JSON number, the environment a string, and a
// flag whatever its parser produced; a whole number is an endpoint of the range
// written without a decimal point, not a type error.
func TestTheFractionalSettingIsReadFromEveryLayer(t *testing.T) {
	defaults, err := Resolve(Sources{})
	require.NoError(t, err)
	assert.InDelta(t, 0.6, defaults.Float("reinvention.min_similarity"), 0)
	assert.Equal(t, 5, defaults.Int("reinvention.max_candidates"))

	file := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"reinvention": {"min_similarity": 0.75}}`), 0o600))

	for name, tc := range map[string]struct {
		src  Sources
		want float64
	}{
		"a JSON number from a file":           {Sources{GlobalConfig: file}, 0.75},
		"a string from the environment":       {Sources{Environ: []string{"CR_REINVENTION_MIN_SIMILARITY= 0.7 "}}, 0.7},
		"a float from a flag":                 {Sources{Flags: map[string]any{"reinvention.min_similarity": 0.9}}, 0.9},
		"a whole number from a flag is exact": {Sources{Flags: map[string]any{"reinvention.min_similarity": 1}}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := Resolve(tc.src)
			require.NoError(t, err)
			assert.InDelta(t, tc.want, cfg.Float("reinvention.min_similarity"), 0)
		})
	}
}

// A value that is not a number is refused naming the setting, rather than
// read as zero — a zero threshold qualifies every symbol in the repository with
// a matching parameter count.
func TestAFractionalSettingThatIsNotANumberIsRefused(t *testing.T) {
	for name, src := range map[string]Sources{
		"a word from the environment": {Environ: []string{"CR_REINVENTION_MIN_SIMILARITY=high"}},
		"a boolean from a flag":       {Flags: map[string]any{"reinvention.min_similarity": true}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Resolve(src)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "reinvention.min_similarity")
			assert.Contains(t, err.Error(), "expected a number")
		})
	}
}

// Float answers zero for a key that is not a fractional setting, the way Int and
// String answer their zero values, so a misspelt key reads as absent rather
// than panicking a command.
func TestFloatOfAnotherTypeIsZero(t *testing.T) {
	cfg, err := Resolve(Sources{})
	require.NoError(t, err)
	assert.Zero(t, cfg.Float("reinvention.max_candidates"))
	assert.Zero(t, cfg.Float("no.such.key"))
}
