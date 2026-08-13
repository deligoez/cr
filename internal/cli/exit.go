package cli

import (
	"errors"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
)

// Exit codes, fixed by spec/0.1.0.md §11.2. Never renumber these.
const (
	// ExitOK signals success.
	ExitOK = 0
	// ExitValidation signals invalid input data.
	ExitValidation = 1
	// ExitUsage signals a malformed invocation.
	ExitUsage = 2
	// ExitFile signals a file, configuration, or external command failure.
	ExitFile = 3
	// ExitState signals a state conflict, including lock timeout and partial post.
	ExitState = 4
)

// exitCodeFor maps an error onto the code §11.2 gives its cause. The mapping
// lives here rather than in the package that raises the error, so a validator
// below cli never has to name an exit code itself. An unmapped cause is a
// malformed invocation.
func exitCodeFor(err error) int {
	var invalidAxis *axis.InvalidError
	if errors.As(err, &invalidAxis) {
		// §1.5: an axis field outside the closed set is a bad
		// configuration file, which §11.2 codes as ExitFile.
		return ExitFile
	}
	var malformedProfile *profile.MalformedError
	if errors.As(err, &malformedProfile) {
		// §2.5 item 3: a malformed profile file aborts with exit
		// code 3, naming the file and the offending field.
		return ExitFile
	}
	var protectedName *config.ProtectedError
	if errors.As(err, &protectedName) {
		// §2.7: a CR_ variable or config key addressing a protected
		// decision is a configuration failure, which §11.2 codes as
		// ExitFile. It is not a validation failure: the run never
		// reached input data, and it is not a usage error, because
		// nothing about the invocation can be corrected.
		return ExitFile
	}
	var gitCommand *git.CommandError
	if errors.As(err, &gitCommand) {
		// §3.1.3 codes a non-zero exit from an external command 3 and
		// surfaces its stderr. It fixes that for the tracker command,
		// and git is one of the same three external tools, so it fails
		// through the same mapping rather than a second one.
		return ExitFile
	}
	var reservedField *state.ReservedFieldError
	if errors.As(err, &reservedField) {
		// §6.1.4: a record supplying a field cr writes itself is
		// rejected with exit code 1. The file was found, read, and
		// parsed, so nothing about it failed as a file; what is wrong
		// is the input data inside it, which §11.2 codes 1 rather than
		// the 3 a malformed profile gets.
		return ExitValidation
	}
	return ExitUsage
}
