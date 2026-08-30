package activation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/profile"
)

// keyPattern is §3.2's default. It is written out rather than resolved through
// internal/config because what this package is measured on is the axis decision,
// not the pattern; TestTheKeyPatternDefaultIsTheOneTheSpecNames in internal/intent
// is what ties this expression to the configuration table that supplies it.
const keyPattern = `[A-Z][A-Z0-9]+-[0-9]+`

// builtin parses one of the profiles §2.4.5 ships, so the cases below run
// against the files cr actually writes rather than against a literal that can
// drift from them.
func builtin(t *testing.T, id string) profile.Profile {
	t.Helper()
	p, err := profile.Parse(id+".json", []byte(profile.Builtins()[id]))
	require.NoError(t, err)
	return p
}

// parsed builds a profile from a file body, for the shapes no shipped profile
// has: an axis switched off, and an axis left out of `axes` altogether.
func parsed(t *testing.T, stem, body string) profile.Profile {
	t.Helper()
	p, err := profile.Parse(stem+".json", []byte(body))
	require.NoError(t, err)
	return p
}

// resolved is a §3.2 resolution that found a key. The issue text comes from a
// file per §3.1.4, so no tracker command is reached.
func resolved(t *testing.T) intent.Intent {
	t.Helper()
	path := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(path, []byte("Add a thing to the thing.\n"), 0o600))

	i, err := intent.Resolve(intent.KeySources{Branch: "feature/CR-1-add-a-thing"}, keyPattern, intent.Source{File: path})
	require.NoError(t, err)
	require.NotEmpty(t, i.Key.Value, "the case needs a key to have resolved")
	return i
}

// unresolved is §3.2's empty intent: no source yielded a key, so §4.5.3 applies.
func unresolved(t *testing.T) intent.Intent {
	t.Helper()
	i, err := intent.Resolve(intent.KeySources{Branch: "feature/add-a-thing"}, keyPattern, intent.Source{})
	require.NoError(t, err)
	return i
}

// off is a Disabled reduced to the two things a case asserts on: which axis went
// off and which item of §4.5 took it off. The reason is checked separately, as a
// property every entry has, so a case does not pin a sentence and stop being
// about the rule.
type off struct {
	Axis string
	Rule string
}

func switchedOff(disabled []Disabled) []off {
	out := make([]off, 0, len(disabled))
	for _, d := range disabled {
		out = append(out, off{Axis: d.Axis, Rule: d.Rule})
	}
	return out
}

// axesOf reduces the unavailable entries to the axis ids they name.
func axesOf(unavailable []intent.Unavailable) []string {
	out := make([]string, 0, len(unavailable))
	for _, u := range unavailable {
		out = append(out, u.Axis)
	}
	return out
}

// §4.5.1 makes an axis active when the resolved configuration enables it and its
// prerequisites are met, §4.5.2 takes the test axis out when the profile
// declares no `tests.cmd`, and §4.5.3 marks the intent axis unavailable when no
// issue key resolves. The three are asserted together because the failure worth
// catching is not one of them being wrong on its own — it is one of them
// swallowing another, which only shows when all four axes are read off the same
// run.
//
// Every case therefore asserts the whole partition: the exact active set, the
// exact disabled set with the item of §4.5 that each entry cites, and the exact
// unavailable set. A rule that quietly widened would take an axis out of one of
// the other two lists, and there is nowhere for it to go unseen.
func TestAxisActivationAppliesTheThreeRulesOf45(t *testing.T) {
	for _, tc := range []struct {
		name        string
		profile     profile.Profile
		intent      intent.Intent
		active      []string
		disabled    []off
		unavailable []string
	}{
		{
			// §4.5.1's positive half. Nothing is off, which is the
			// baseline the other cases are a departure from.
			name:        "every axis enabled and every prerequisite met",
			profile:     builtin(t, "laravel-pest"),
			intent:      resolved(t),
			active:      []string{axis.Intent, axis.Correctness, axis.Convention, axis.Test},
			disabled:    []off{},
			unavailable: []string{},
		},
		{
			// §4.5.1's negative half, in both spellings a profile
			// has for it: `correctness` is declared false, and
			// `convention` is left out of `axes` altogether. §2.4
			// requires the object, not an entry per axis, so an
			// absent id is a real configuration and not a malformed
			// one.
			name: "an axis the configuration does not enable",
			profile: parsed(t, "custom", `{
			  "id": "custom",
			  "match": {"files": ["go.mod"], "globs": ["**/*.go"]},
			  "axes": {"intent": true, "correctness": false, "test": true},
			  "tests": {"cmd": ["go", "test", "./..."], "globs": ["**/*_test.go"]}
			}`),
			intent:      resolved(t),
			active:      []string{axis.Intent, axis.Test},
			disabled:    []off{{Axis: axis.Correctness, Rule: RuleConfigured}, {Axis: axis.Convention, Rule: RuleConfigured}},
			unavailable: []string{},
		},
		{
			// §4.5.2. `generic` enables all four axes and ships no
			// `tests.cmd`, so the test axis goes off automatically
			// while the configuration says nothing about it — which
			// is the case the word "automatically" is for.
			name:        "a profile that declares no tests.cmd",
			profile:     builtin(t, "generic"),
			intent:      resolved(t),
			active:      []string{axis.Intent, axis.Correctness, axis.Convention},
			disabled:    []off{{Axis: axis.Test, Rule: RuleNoTestCommand}},
			unavailable: []string{},
		},
		{
			// §4.5.3, and the containment that matters with it: the
			// intent axis is out and the other three still ran, so
			// an absent tracker narrows the review by one lens
			// rather than by four.
			name:        "no issue key resolves",
			profile:     builtin(t, "laravel-pest"),
			intent:      unresolved(t),
			active:      []string{axis.Correctness, axis.Convention, axis.Test},
			disabled:    []off{},
			unavailable: []string{axis.Intent},
		},
		{
			// The overlap the ordering decides. §4.5.2 is written
			// with no condition, so a profile that also switches
			// `test` off does not get to answer for the axis in its
			// place: the entry cites the prerequisite, which is the
			// half that would still be missing after the
			// configuration was changed.
			name: "an axis both switched off and missing its prerequisite",
			profile: parsed(t, "no-runner", `{
			  "id": "no-runner",
			  "match": {"files": [], "globs": ["**/*"]},
			  "axes": {"intent": true, "correctness": true, "convention": true, "test": false}
			}`),
			intent:      resolved(t),
			active:      []string{axis.Intent, axis.Correctness, axis.Convention},
			disabled:    []off{{Axis: axis.Test, Rule: RuleNoTestCommand}},
			unavailable: []string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := Activate(&tc.profile, tc.intent)

			assert.Equal(t, tc.active, a.Active)
			assert.Equal(t, tc.disabled, switchedOff(a.Disabled))
			assert.Equal(t, tc.unavailable, axesOf(a.Unavailable))

			// §4.5.4 asks for a reason, not a flag, and §12.4 asks
			// it to name the next actionable step. A disabled axis
			// whose reason did not name the profile that switched
			// it off would leave the user hunting for which of the
			// §2.4 layers to edit.
			for _, d := range a.Disabled {
				assert.Contains(t, d.Reason, tc.profile.ID)
			}
			for _, u := range a.Unavailable {
				assert.NotEmpty(t, u.Reason)
			}
		})
	}
}
