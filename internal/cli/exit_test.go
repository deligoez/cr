package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
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

// §2.7 lets no layer supply the confirmation gate, the argued forcing, the
// question label, or the provenance and evidence regions, and a name that would
// address one is a bad configuration rather than a bad invocation, so §11.2
// codes it 3. The code must survive the wrapping a command adds on the way out.
func TestProtectedConfigNameExitsWithTheFileCode(t *testing.T) {
	_, err := config.Resolve(config.Sources{Environ: []string{"CR_POST_AUTO_CONFIRM=1"}})
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("resolving configuration: %w", err)))
}

// agentRecord stands in for a record type of one of the eight §2.3.3 files. It
// carries head and round the only way any type can: by embedding state.Stamp.
type agentRecord struct {
	state.Stamp
	ID string `json:"id"`
}

// §2.3.3 has cr stamp head and round on write, so a record supplying either is
// bad input data rather than an unusable file, and §11.2 codes that 1 and not
// the 3 a malformed profile gets. The code must survive the wrapping a command
// adds on the way out.
func TestASuppliedStampFieldExitsWithTheValidationCode(t *testing.T) {
	_, err := state.DecodeStamped[agentRecord](
		state.FileClaims, []byte(`{"id":"c1","round":2}`),
	)
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("recording claims: %w", err)))
}
