package cli

import (
	"errors"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/profile"
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
	return ExitUsage
}
