package cli

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
