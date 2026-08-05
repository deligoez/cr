package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §1.5 makes an axis id outside the closed set abort with exit code 3, and the
// abort must survive the wrapping a command adds on the way out. Anything with
// no mapping stays a malformed invocation.
func TestInvalidAxisExitsWithTheFileCode(t *testing.T) {
	invalid := axis.Validate("roles/security.json", "axis", "security")
	assert.Equal(t, ExitFile, exitCodeFor(invalid))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("loading roles: %w", invalid)))
	assert.Equal(t, ExitUsage, exitCodeFor(errors.New("unknown flag")))
}

// §2.5 item 3 makes a malformed profile file abort with exit code 3, and the
// abort must survive the wrapping a command adds on the way out.
func TestMalformedProfileExitsWithTheFileCode(t *testing.T) {
	_, err := profile.Parse("profiles/generic.json", []byte(`{"id": "generic"}`))
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("loading profiles: %w", err)))
}
