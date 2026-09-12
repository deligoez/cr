package cli

import (
	"os"

	"github.com/deligoez/cr/internal/state"
)

// readInput reads a file the caller named on the command line.
//
// Every §11 command that takes a `<file>` positional or a file flag comes
// through here, so §11.2's answer to an input cr cannot read is one answer for
// the surface rather than one per command. os.ReadFile returns *fs.PathError,
// which exitCodeFor has no mapping for and which therefore took the fallback of
// §11.2's code 2 — measured 2026-09-13 on the built binary,
// `cr record 1 missing.ndjson --repo o/r` printed
// `open missing.ndjson: no such file or directory` and exited 2, telling the
// user to retype a command line that was right.
//
// The hint is a parameter rather than one sentence for the whole surface,
// because the next actionable step differs: `cr record`'s input is written by
// `cr merge`, and `cr probe run`'s is written by the agent.
func readInput(path, hint string) ([]byte, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, state.FileFailure("read", path, hint, err)
	}
	return body, nil
}
