package profile

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// argvProfile is a resolved profile carrying just the two §2.4 fields a test
// run reads.
func argvProfile(cmd []string, filterFlag string) Profile {
	return Profile{Tests: Tests{Cmd: cmd, FilterFlag: filterFlag}}
}

// §5.2.1's `--filter` narrows the run through `tests.filter_flag`, and a
// profile that sets no such flag refuses instead of widening the question.
//
// The expression is a separate argv element rather than text appended to the
// flag, because there is no shell anywhere in cr's execution path: §3.1.1
// settled that for the tracker command and §5.1.3 inherits it, so an expression
// holding a space or a quote reaches the runner as the one argument it is.
//
// The refusal is the half worth having. Running the whole suite when a subset
// was asked for answers a different question than the one put, and §5.3.6 has
// cr carry a filter into the evidence region precisely because a filtered run
// proves the gap only for the tests it selected — a claim that is false if the
// filter was quietly dropped.
func TestTheFilterIsPassedAsTheProfilesFilterFlag(t *testing.T) {
	file := filepath.Join("home", ".cr", "profiles", "laravel-pest.json")
	runner := []string{"./vendor/bin/pest"}

	t.Run("no filter runs the whole suite", func(t *testing.T) {
		p := argvProfile(runner, "--filter")

		argv, err := p.TestArgv(file, "", nil)

		require.NoError(t, err)
		assert.Equal(t, runner, argv)
	})

	t.Run("a filter is appended as the flag and the expression", func(t *testing.T) {
		p := argvProfile(runner, "--filter")

		argv, err := p.TestArgv(file, "retries the request twice", nil)

		require.NoError(t, err)
		assert.Equal(t,
			[]string{"./vendor/bin/pest", "--filter", "retries the request twice"}, argv)
	})

	t.Run("the profile's own argv is left alone", func(t *testing.T) {
		p := argvProfile(runner, "--filter")

		_, err := p.TestArgv(file, "one", nil)
		require.NoError(t, err)

		assert.Equal(t, []string{"./vendor/bin/pest"}, p.Tests.Cmd,
			"a second run would inherit the first run's filter")
	})

	t.Run("a filter with no flag to carry it is refused", func(t *testing.T) {
		p := argvProfile(runner, "")

		argv, err := p.TestArgv(file, "retries the request twice", nil)

		assert.Nil(t, argv)
		var unavailable *UnavailableError
		require.ErrorAs(t, err, &unavailable)
		assert.Equal(t, testFilterFlagField, unavailable.Field)
		assert.Equal(t, file, unavailable.File, "§2.5 item 3: the refusal names the file to open")
	})

	t.Run("no test command at all is refused", func(t *testing.T) {
		p := argvProfile(nil, "--filter")

		argv, err := p.TestArgv(file, "", nil)

		assert.Nil(t, argv)
		var unavailable *UnavailableError
		require.ErrorAs(t, err, &unavailable)
		assert.Equal(t, testCmdField, unavailable.Field)
		assert.Contains(t, err.Error(), file)
	})

	t.Run("no profile at all names no file", func(t *testing.T) {
		p := argvProfile(nil, "")

		_, err := p.TestArgv("", "", nil)

		var unavailable *UnavailableError
		require.ErrorAs(t, err, &unavailable)
		assert.Contains(t, err.Error(), "no profile was resolved",
			"§2.4.4 has cr report the situation rather than name a file that does not exist")
	})
}

// §2.4's `tests.paths_arg`: "Argv appended to `tests.cmd` once per `--path`,
// with every `{path}` replaced by that path; when absent, `--path` MUST abort
// with exit code 3 naming the profile."
//
// The refusal is the half worth having, for the reason the filter's is. A run
// over the whole suite when one directory was asked for answers a different
// question, and §5.2.2 keys a baseline by the paths given — so a dropped path
// would have a probe graded against a population it never ran.
func TestEveryPathIsPassedThroughTheProfilesPathsArg(t *testing.T) {
	file := filepath.Join("home", ".cr", "profiles", "laravel-pest.json")
	runner := []string{"./vendor/bin/pest"}

	t.Run("each path repeats the argv with the placeholder replaced", func(t *testing.T) {
		p := argvProfile(runner, "--filter")
		p.Tests.PathsArg = []string{"--test-directory={path}"}

		argv, err := p.TestArgv(file, "", []string{"tests/Feature", "tests/Unit"})

		require.NoError(t, err)
		assert.Equal(t, []string{
			"./vendor/bin/pest", "--test-directory=tests/Feature", "--test-directory=tests/Unit",
		}, argv)
	})

	t.Run("a multi-element argv is repeated whole, and every placeholder filled", func(t *testing.T) {
		p := argvProfile(runner, "--filter")
		p.Tests.PathsArg = []string{"--path", "{path}", "--also", "{path}"}

		argv, err := p.TestArgv(file, "", []string{"tests/Unit"})

		require.NoError(t, err)
		assert.Equal(t, []string{
			"./vendor/bin/pest", "--path", "tests/Unit", "--also", "tests/Unit",
		}, argv)
	})

	t.Run("the filter comes first and the paths follow it", func(t *testing.T) {
		p := argvProfile(runner, "--filter")
		p.Tests.PathsArg = []string{"{path}"}

		argv, err := p.TestArgv(file, "an empty cart", []string{"tests/Unit"})

		require.NoError(t, err)
		assert.Equal(t,
			[]string{"./vendor/bin/pest", "--filter", "an empty cart", "tests/Unit"}, argv)
	})

	t.Run("no path leaves the argv as the filter left it", func(t *testing.T) {
		p := argvProfile(runner, "--filter")
		p.Tests.PathsArg = []string{"{path}"}

		argv, err := p.TestArgv(file, "", nil)

		require.NoError(t, err)
		assert.Equal(t, runner, argv)
	})

	t.Run("a path with no argv to carry it is refused", func(t *testing.T) {
		p := argvProfile(runner, "--filter")

		argv, err := p.TestArgv(file, "", []string{"tests/Unit"})

		assert.Nil(t, argv)
		var unavailable *UnavailableError
		require.ErrorAs(t, err, &unavailable)
		assert.Equal(t, testPathsArgField, unavailable.Field)
		assert.Equal(t, file, unavailable.File, "§2.4: the refusal names the profile")
	})
}
