package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// The detect-less rule this file plants, written the way a user writes one:
// a file under §2.2's rules directory, named by its id.
const (
	injectedRuleID    = "handle-every-error"
	injectedTitle     = "Every error is handled where it is returned."
	injectedRationale = "A dropped error turns a failure into a wrong answer nobody sees."
)

// plantRule writes one detect-less rule into the global rules directory of the
// state tree at root, and returns nothing: what the tests below read is the
// effect it does not have.
func plantRule(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "rules")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	doc, err := json.Marshal(map[string]any{
		"id":        injectedRuleID,
		"title":     injectedTitle,
		"rationale": injectedRationale,
		"class":     "unchecked-error",
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, injectedRuleID+".json"), doc, 0o600))
}

// §2.6.1.4's injection reaches the prompt through §4.6.1's fan-out, and never
// through the role's own instructions file.
//
// The eject is where the two could be confused, and it is the only place cr
// writes a role file at all: §2.5.2 hands the user an editable copy of the
// built-in, and a cr that answered §2.6.1.4 by folding the rule's text into
// that copy would look identical from the outside — the role would read the
// standard, and the prompt would carry it.
//
// It would also be wrong in a way nothing could undo. §2.5.2 forbids
// overwriting an ejected file, so the rule's text would be frozen into the
// user's role on the day they ejected: editing the rule afterwards would change
// nothing, deleting it would leave the standard enforced, and §2.6 item 2's
// layering would have no effect on a copy that is no longer a rule.
//
// So the file is asserted byte-for-byte against the built-in, with a rule
// standing in the tree that a role-file injection would have had to reach.
func TestEjectingRolesCarriesNoRuleText(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".cr")
	t.Setenv(state.HomeEnv, root)

	runInit(t)
	plantRule(t, root)
	runInit(t, "--eject-roles")

	shipped := role.Builtins()
	require.Contains(t, shipped, "convention",
		"§2.6's default axis is convention, so that is the role an injection would reach")
	for id, content := range shipped {
		onDisk, err := os.ReadFile(filepath.Join(root, "roles", id+".json"))
		require.NoError(t, err)
		assert.Equalf(t, content, string(onDisk),
			"§2.5.2: the ejected %s role is the built-in, and a rule in the tree did not change it", id)
		assert.NotContains(t, string(onDisk), injectedTitle)
		assert.NotContains(t, string(onDisk), injectedRationale)
	}
}

// The built-in roles carry no rule text either, which is the half the eject
// above cannot see.
//
// §2.6 keeps rules and roles apart — a role is a lens and supplies prose, a
// rule is a specific standard and is data — and the failure mode this closes is
// a standard written into a role's `instructions` because that was the only
// channel that existed. A role saying it is checked by §4.6.1's fan-out instead
// leaves the standard where §2.6 item 2 can override it and §2.6.3.4 can report
// it dead.
func TestNoBuiltinRoleCarriesARuleAsProse(t *testing.T) {
	shipped := role.Builtins()
	require.NotEmpty(t, shipped)

	for id, content := range shipped {
		var carried map[string]any
		require.NoError(t, json.Unmarshal([]byte(content), &carried))
		_, named := carried["rules"]
		assert.Falsef(t, named,
			"§2.5's table gives a role no rules field, and %s carries one", id)
	}
}

// Nothing in internal/role can read a rule, which is what makes the two
// assertions above hold for code nobody has written yet.
//
// The import is the whole of the mechanism. A role's instructions are built in
// internal/role and nowhere else, so a rule can only reach them through this
// package — and it cannot, because the dependency does not exist and would have
// to be added deliberately. §2.6.1.4's injection therefore has one route to a
// prompt, the fan-out of §4.6.1, and the other route is closed by construction
// rather than by nobody having taken it yet.
func TestTheRolePackageCannotReadARule(t *testing.T) {
	const rolePkg = "internal" + string(filepath.Separator) + "role" + string(filepath.Separator)

	scanned := 0
	var found []string
	crSource(t, func(rel string, imports []string) {
		if !strings.HasPrefix(rel, rolePkg) {
			return
		}
		scanned++
		for _, name := range imports {
			if strings.HasSuffix(name, "/internal/rule") {
				found = append(found, rel+" imports "+name)
			}
		}
	})

	require.NotZero(t, scanned, "no file of internal/role was read, so this guard proved nothing")
	assert.Empty(t, found,
		"§2.6.1.4: a rule reaches a prompt through §4.6.1's fan-out, never through a role")
}
