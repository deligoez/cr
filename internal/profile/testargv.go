package profile

import (
	"fmt"
	"slices"
)

// The dotted §2.4 fields a test run needs, spelled once so a refusal names the
// key the user has to add rather than a paraphrase of it.
const (
	testCmdField        = "tests.cmd"
	testFilterFlagField = "tests.filter_flag"
)

// UnavailableError reports a §2.4 field a command needs and the resolved
// profile does not set.
//
// It is not a MalformedError. The profile parsed, every field it does carry is
// valid, and nothing about it is wrong — it simply does not configure the thing
// that was asked for, which is a different sentence to tell the user and a
// different edit to make. §11.2 codes it 3 all the same: the command line is
// right and what refuses is the configuration.
//
// §2.4.4's "no profile matched" arrives here too, because it is the same
// situation with no file to name: the section has cr report the situation
// rather than guess, and a run with no profile has no test command by exactly
// the reason a profile without `tests.cmd` has none.
type UnavailableError struct {
	// File is the profile file to open, and is empty when no profile was
	// resolved at all.
	File string
	// Field is the dotted §2.4 field that is not set.
	Field string
	// Needs is what setting it would have made possible.
	Needs string
}

func (e *UnavailableError) Error() string {
	if e.File == "" {
		return fmt.Sprintf(
			"no profile was resolved for this pull request, so %s is unset: %s", e.Field, e.Needs)
	}
	return fmt.Sprintf("%s: %s is not set: %s", e.File, e.Field, e.Needs)
}

// TestArgv returns the argv §5.2.1 runs inside the sandbox, narrowed to filter
// when one was given.
//
// The filter is appended as `tests.filter_flag` and the expression after it,
// two argv elements rather than one string. There is no shell anywhere in cr's
// execution path — §3.1.1 settled that for the tracker command and §5.1.3
// inherits it — so an expression holding a space, a quote, or a `$` reaches the
// runner as the one argument it is, and there is nothing to quote for.
//
// A `--filter` against a profile that sets no flag is refused rather than
// dropped. Running the whole suite when a subset was asked for would answer a
// different question than the one put, and §5.3.6 rests on knowing exactly
// which tests a probe selected.
//
// file names the profile the fields came from, so a refusal says what to open.
func (p *Profile) TestArgv(file, filter string) ([]string, error) {
	if len(p.Tests.Cmd) == 0 {
		return nil, &UnavailableError{
			File:  file,
			Field: testCmdField,
			Needs: "§2.4 makes an absent test command a disabled test axis, and §5.2.1 has cr run it inside the sandbox",
		}
	}
	argv := slices.Clone(p.Tests.Cmd)
	if filter == "" {
		return argv, nil
	}
	if p.Tests.FilterFlag == "" {
		return nil, &UnavailableError{
			File:  file,
			Field: testFilterFlagField,
			Needs: "§5.2.1 passes --filter as that flag, so this profile has no way to narrow a run to a subset",
		}
	}
	return append(argv, p.Tests.FilterFlag, filter), nil
}
