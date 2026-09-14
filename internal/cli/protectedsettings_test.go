package cli

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/state"
)

// protectedFiles are §2.7's file layers and the profile, each carrying one name
// that addresses the argued forcing of §6.3: a config key in either config
// layer, and a profile field in a profile file a command may resolve.
var protectedFiles = []struct {
	layer string
	path  func(state.Layout) string
	body  string
	name  string
}{
	{
		layer: "global config",
		path:  state.Layout.Config,
		body:  `{"grading":{"argued":"finding"}}`,
		name:  "grading.argued",
	},
	{
		layer: "repository config",
		path:  func(l state.Layout) string { return l.RepoConfig(fixtureOwner, fixtureProject) },
		body:  `{"grading":{"argued":"finding"}}`,
		name:  "grading.argued",
	},
	{
		layer: "profile",
		path:  func(l state.Layout) string { return l.Profile("crqa") },
		body: `{"id":"crqa","match":{"files":["go.mod"],"globs":["**/*"]},` +
			`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},"forcing":false}`,
		name: "forcing",
	},
}

// namedByQA are the commands QA found accepting a protected config key or
// profile field in silence while `cr brief`, `cr draft` and `cr post` refused
// it (D-V1a-6). The walk below covers the whole tree; this list only proves the
// walk reached each of them.
var namedByQA = []string{
	"record", "merge", "cells record", "claims record", "map record",
	"waivers list", "context", "answer", "claims set-aside",
	"brief", "draft", "post",
}

// exemptFromProtectedSettings are the invocations that resolve neither the
// configuration nor a profile, with the reason. cobra answers both flags before
// any hook runs, so no command's work, and no layer, is reached.
var exemptFromProtectedSettings = map[string]string{
	"--version": "cobra prints the version before any hook and reads no layer",
	"--help":    "cobra prints the usage before any hook and reads no layer",
}

// §2.7 through every command: a config key in either config layer, and a
// profile field, whose name addresses a protected decision is refused with exit
// code 3 naming the file and the key, before the command does any work.
//
// QA found the refusal per command (D-V1a-6): with `grading.argued` in a config
// layer or `forcing` in the selected profile, `cr record`, `cr merge`,
// `cr cells record`, `cr claims record`, `cr map record`, `cr waivers list`,
// `cr context`, `cr answer` and `cr claims set-aside` exited 0 while
// `cr brief`, `cr draft` and `cr post` exited 3. The walk is over the tree, so
// a command added later is held to the same answer.
func TestEveryCommandRefusesAProtectedConfigKeyOrProfileField(t *testing.T) {
	root := newRootCmd()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	commands := runnableCommands(root)
	require.Greater(t, len(commands), 20, "a walk that found few commands proves little")

	walked := make([]string, 0, len(commands))
	for _, command := range commands {
		walked = append(walked, strings.TrimPrefix(command.CommandPath(), "cr "))
	}
	for _, named := range namedByQA {
		require.Contains(t, walked, named, "the walk reaches every command QA named")
	}

	for _, file := range protectedFiles {
		for _, command := range commands {
			args := append(argsFromUse(command), "--repo", fixtureSlug)
			t.Run(file.layer+"/"+strings.Join(args, " "), func(t *testing.T) {
				layout := state.New(crHome(t))
				path := file.path(layout)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				require.NoError(t, os.WriteFile(path, []byte(file.body), 0o600))
				before := stateFiles(t, layout)

				err := runTree(t, args...)

				require.Error(t, err, "§2.7: cr %s accepted %s in the %s", strings.Join(args, " "), file.name, file.layer)
				var protected *config.ProtectedError
				require.ErrorAs(t, err, &protected)
				assert.Equal(t, file.name, protected.Name)
				assert.Equal(t, ExitFile, exitCodeFor(err))
				assert.Equal(t, path+": "+protected.Error(), err.Error(), "the refusal names the file and the key")
				assert.Equal(t, before, stateFiles(t, layout), "the refusal comes before the command's work")
			})
		}
	}

	for _, flag := range slices.Sorted(maps.Keys(exemptFromProtectedSettings)) {
		t.Run("exempt/"+flag, func(t *testing.T) {
			layout := state.New(crHome(t))
			for _, file := range protectedFiles {
				path := file.path(layout)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				require.NoError(t, os.WriteFile(path, []byte(file.body), 0o600))
			}

			assert.NoError(t, runTree(t, flag, "--repo", fixtureSlug), exemptFromProtectedSettings[flag])
		})
	}
}
