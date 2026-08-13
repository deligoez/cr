package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
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

// §3.1.3 codes a non-zero exit from an external command 3 and surfaces its
// stderr. git is one of the three external tools cr drives, so a git that
// refuses exits the same way a refusing tracker command does, and the code must
// survive the wrapping a command adds on the way out.
func TestAFailedGitCommandExitsWithTheFileCode(t *testing.T) {
	_, err := git.MergeBase(filepath.Join(t.TempDir(), "no-such-checkout"), "main", "feature")
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("ingesting the diff: %w", err)))
}

// gh is the third of those tools, and §3.5's ingestion is the first thing that
// drives it. A GraphQL error arrives as a refusal like any other — gh exits
// non-zero with the message on stderr — so it maps onto the same code, and the
// code must survive the wrapping a command adds on the way out.
func TestAFailedGhCommandExitsWithTheFileCode(t *testing.T) {
	err := error(&gh.CommandError{
		Args:   []string{"api", "graphql"},
		Stderr: "Could not resolve to a Repository with the name 'acme/web'.",
		Err:    errors.New("exit status 1"),
	})
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("ingesting threads: %w", err)))
}

// §6.1 rejects a class that is not kebab-case. The record's file was read and
// parsed, so nothing about it failed as a file; what is wrong is the agent's
// data inside it, which §11.2 codes 1. The code must survive the wrapping a
// command adds on the way out.
func TestAnInvalidClassExitsWithTheValidationCode(t *testing.T) {
	err := finding.ValidateClass("review-correctness.ndjson", 7, "Missing Test")
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("merging findings: %w", err)))
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
		state.FileClaims, []byte(`{"id":"c1","round":2}`), nil,
	)
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("recording claims: %w", err)))
}
