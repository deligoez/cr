package profile

import (
	"fmt"
	"slices"
	"strings"
)

// The dotted §2.4 fields a test run needs, spelled once so a refusal names the
// key the user has to add rather than a paraphrase of it.
const (
	testCmdField        = "tests.cmd"
	testFilterFlagField = "tests.filter_flag"
	testPathsArgField   = "tests.paths_arg"
)

// pathPlaceholder is the position §2.4 substitutes each `--path` into, in every
// element of `tests.paths_arg`. A `tests.paths_arg` that holds none is a runner
// whose path argument is positional, and the path is then not passed at all —
// which is why nothing here requires the placeholder to be present: §2.4 says
// what is replaced, and a profile that replaces nothing is the author's
// statement about their runner rather than a fault cr can diagnose.
const pathPlaceholder = "{path}"

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

// Hint is §12.4's next actionable step: the profile file that has to declare
// the field, by its path.
//
// The step names the file rather than a command that prints configuration,
// because none prints a profile's fields — `cr config --resolved` shows the
// configuration layers of §2.7, and a profile is a separate file cr reads
// whole. The round's profile is read again on every run, so an edit to that
// file is all the next run needs. With no profile resolved there is no file
// to name, and the step is to give the round one.
func (e *UnavailableError) Hint() string {
	if e.File == "" {
		return fmt.Sprintf(
			"no profile file was resolved for this round: add a profile declaring %s under the "+
				"profiles directory of cr's state root, select it with match.files or the `profile` "+
				"configuration key, and run `cr brief <pr>` so the round records it", e.Field)
	}
	if e.Field == testPathsArgField {
		// A runner can select tests by name and not by file, as cargo
		// does, and a shipped profile leaves the field unset on purpose
		// then: declaring it would not make --path mean anything.
		return fmt.Sprintf(
			"run without --path, or narrow the run with --filter; declare %s in the profile file %s "+
				"only if its test runner can select tests by path", e.Field, e.File)
	}
	return fmt.Sprintf(
		"declare %s in the profile file %s, which is the profile this round resolved; "+
			"`cr config --resolved` does not show profile fields", e.Field, e.File)
}

// CountsOccurrences reports whether §5.2.1 counts the profile's patterns'
// matches rather than summing their capture group.
func (p *Profile) CountsOccurrences() bool {
	return p.Tests.CountMode == CountModeOccurrences
}

// TestArgv returns the argv §5.2.1 runs inside the sandbox, narrowed to filter
// when one was given and to paths when any were.
//
// The filter is appended as `tests.filter_flag` and the expression after it,
// two argv elements rather than one string. There is no shell anywhere in cr's
// execution path — §3.1.1 settled that for the tracker command and §5.1.3
// inherits it — so an expression holding a space, a quote, or a `$` reaches the
// runner as the one argument it is, and there is nothing to quote for. A path
// reaches it the same way, through `tests.paths_arg` once per path, in the
// order the paths were given.
//
// A `--filter` against a profile that sets no flag is refused rather than
// dropped, and so is a `--path` against one that sets no `tests.paths_arg`.
// Running the whole suite when a subset was asked for would answer a
// different question than the one put, and §5.3.6 rests on knowing exactly
// which tests a probe selected — which is the same reason §5.2.2 keys a
// baseline by the filter and the paths together.
//
// file names the profile the fields came from, so a refusal says what to open.
func (p *Profile) TestArgv(file, filter string, paths []string) ([]string, error) {
	if len(p.Tests.Cmd) == 0 {
		return nil, &UnavailableError{
			File:  file,
			Field: testCmdField,
			Needs: "§2.4 makes an absent test command a disabled test axis, and §5.2.1 has cr run it inside the sandbox",
		}
	}
	argv := slices.Clone(p.Tests.Cmd)
	if filter != "" {
		if p.Tests.FilterFlag == "" {
			return nil, &UnavailableError{
				File:  file,
				Field: testFilterFlagField,
				Needs: "§5.2.1 passes --filter as that flag, so this profile has no way to narrow a run to a subset",
			}
		}
		argv = append(argv, p.Tests.FilterFlag, filter)
	}
	if len(paths) == 0 {
		// §2.4's `tests.paths_default`: what a runner needs to be told to
		// run everything, for one whose bare invocation does not.
		return append(argv, p.Tests.PathsDefault...), nil
	}
	if len(p.Tests.PathsArg) == 0 {
		return nil, &UnavailableError{
			File:  file,
			Field: testPathsArgField,
			Needs: "§2.4 has every --path passed through that argv, so this profile has no way to narrow a run to a path",
		}
	}
	for _, path := range paths {
		for _, arg := range p.Tests.PathsArg {
			argv = append(argv, strings.ReplaceAll(arg, pathPlaceholder, path))
		}
	}
	return argv, nil
}
