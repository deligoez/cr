package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/role"
)

// protectedVariables name one CR_ variable per decision §2.7 keeps out of
// every layer: the argued forcing of §6.3, the confirmation gate of §8.5 and
// the question label of §8.1.4. None of them addresses a setting cr knows,
// which is the case the refusal exists for.
var protectedVariables = []string{"CR_ARGUED_FORCING", "CR_POST_CONFIRM", "CR_QUESTION_LABEL"}

// runnableCommands collects every command of the tree a caller can run, at any
// depth, including the help and completion commands cobra adds at Execute.
func runnableCommands(cmd *cobra.Command) []*cobra.Command {
	found := make([]*cobra.Command, 0)
	if cmd.Runnable() {
		found = append(found, cmd)
	}
	for _, child := range cmd.Commands() {
		found = append(found, runnableCommands(child)...)
	}
	return found
}

// argsFromUse gives a command the positional arguments its Use line names, so
// its Args check passes and the run reaches the hooks: cobra validates the
// arguments before PersistentPreRunE, and a usage refusal would answer 2 for a
// reason that has nothing to do with the variable.
func argsFromUse(cmd *cobra.Command) []string {
	args := strings.Split(cmd.CommandPath(), " ")[1:]
	for _, word := range strings.Fields(cmd.Use)[1:] {
		switch {
		case word == prPlaceholder:
			args = append(args, "7")
		case strings.HasPrefix(word, "<"):
			args = append(args, "x")
		}
	}
	return args
}

// runTree runs the whole command tree with args and returns what it refused.
func runTree(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	return cmd.Execute()
}

// §2.7 through every command: a CR_ variable addressing a protected decision is
// refused with exit code 3 before the command does any work.
//
// QA found the refusal per command (D-S05-5): the commands resolving the
// configuration refused, and `cr record`, `cr merge`, `cr cells record`,
// `cr claims record`, `cr map record`, `cr rules check`, `cr waivers list` and
// `cr context` ran on and said nothing. The walk is over the tree rather than a
// list, so a command added later is held to the same answer without anyone
// remembering to name it. "Before any work" is read off the state root: it is
// created empty, and a command refused before its work — `cr init` included —
// leaves it empty.
func TestEveryCommandRefusesAProtectedVariable(t *testing.T) {
	root := newRootCmd()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	commands := runnableCommands(root)
	require.Greater(t, len(commands), 20, "a walk that found few commands proves little")

	for _, name := range protectedVariables {
		for _, command := range commands {
			args := argsFromUse(command)
			t.Run(name+"/"+strings.Join(args, " "), func(t *testing.T) {
				home := crHome(t)
				t.Setenv(name, "0")

				err := runTree(t, args...)

				require.Error(t, err, "§2.7: cr %s accepted %s", strings.Join(args, " "), name)
				var protected *config.ProtectedError
				require.ErrorAs(t, err, &protected)
				assert.Equal(t, name, protected.Name)
				assert.Equal(t, ExitFile, exitCodeFor(err))
				entries, readErr := os.ReadDir(home)
				require.NoError(t, readErr)
				assert.Empty(t, entries, "the refusal comes before the command's work")
			})
		}
	}
}

// §6.3.3 and §2.7 through a command: a profile field whose name addresses a
// protected decision is refused with exit code 3, naming the file and the
// field, instead of being decoded into nothing (QA D-S05-5).
func TestAProfileFieldAddressingAProtectedDecisionIsRefused(t *testing.T) {
	for field, body := range map[string]string{
		"forcing":              `"forcing":false`,
		"post.confirm":         `"post":{"confirm":true}`,
		"tests.question_label": `"tests":{"question_label":"Note"}`,
	} {
		t.Run(field, func(t *testing.T) {
			layout := gradedHome(t)
			require.NoError(t, layout.EnsureProfile("generic",
				`{"id":"generic","match":{"files":[],"globs":["**/*"]},`+
					`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},`+body+`}`))
			require.NoError(t, os.WriteFile(
				layout.RepoConfig(fixtureOwner, fixtureProject), []byte(`{"profile":"generic"}`), 0o600))

			err := runTree(t, "rules", "list", "--repo", fixtureSlug)

			require.Error(t, err)
			var protected *config.ProtectedError
			require.ErrorAs(t, err, &protected)
			assert.Equal(t, field, protected.Name)
			assert.Equal(t, ExitFile, exitCodeFor(err))
			assert.Equal(t, layout.Profile("generic")+": "+protected.Error(), err.Error(),
				"the refusal names the profile file and the field")
		})
	}
}

// The role file's half, through a command: a key addressing a protected
// decision is refused with exit code 3, naming the file and the key. It is
// refused as a key outside §2.5's table, which the role parser already fences
// by name, so a role cannot carry one into nothing the way a profile could.
func TestARoleFieldAddressingAProtectedDecisionIsRefused(t *testing.T) {
	layout := gradedHome(t)
	path := layout.RepoRole(fixtureOwner, fixtureProject, "correctness")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(
		`{"id":"correctness","title":"Correctness","axis":"correctness",`+
			`"instructions":"Look for defects.","focus":[],"profiles":[],"force_kind":"finding"}`), 0o600))

	err := runTree(t, "status", "7", "--repo", fixtureSlug)

	require.Error(t, err)
	var malformed *role.MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, path, malformed.File)
	assert.Equal(t, "force_kind", malformed.Field)
	assert.Equal(t, ExitFile, exitCodeFor(err))
}
