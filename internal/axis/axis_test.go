package axis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spec/0.1.0.md §1.5 names the four axes and no others, so the set itself is
// asserted here rather than only the acceptance of each member.
func TestValidateAcceptsExactlyTheFourAxisIDs(t *testing.T) {
	assert.Equal(t, []string{"intent", "correctness", "convention", "test"}, IDs())
	for _, id := range IDs() {
		assert.True(t, Valid(id), "%s must be a valid axis id", id)
		assert.NoError(t, Validate("roles/some-role.json", "axis", id))
	}
}

// §1.5 aborts on any other value, and the abort is only actionable if it says
// which file carried it and what it said, so both are asserted on the error and
// in the message the user reads.
func TestValidateNamesTheOffendingFileAndValue(t *testing.T) {
	err := Validate("roles/security.json", "axis", "security")
	require.Error(t, err)

	var invalid *InvalidError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "roles/security.json", invalid.File)
	assert.Equal(t, "axis", invalid.Field)
	assert.Equal(t, "security", invalid.Value)

	assert.Contains(t, err.Error(), "roles/security.json")
	assert.Contains(t, err.Error(), `"security"`)
	assert.Contains(t, err.Error(), "intent, correctness, convention, test")
}

// The set is closed in v0.1, so neither route into it may work: a config file
// and a CR_ variable naming a fifth axis must reach no setting, and the ids the
// validator consults must not be writable through what IDs returns.
func TestNoConfigurationLayerCanExtendTheAxisSet(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(global, []byte(`{"axes": {"security": true}}`), 0o600))

	resolved, err := config.Resolve(config.Sources{
		Environ:      []string{"CR_AXES_SECURITY=true"},
		GlobalConfig: global,
	})
	require.NoError(t, err)
	for key := range resolved.Map() {
		assert.NotContains(t, key, "axis", "no setting may address an axis id")
		assert.NotContains(t, key, "axes", "no setting may address an axis id")
	}

	tampered := IDs()
	tampered[0] = "security"
	assert.Equal(t, "intent", IDs()[0], "IDs must hand out a copy")
	assert.False(t, Valid("security"))
	require.Error(t, Validate(global, "axes.security", "security"))
}
