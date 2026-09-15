package review

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/role"
)

// §4.6.1 and §2.5: a role declaring `classes` has them printed in its prompts,
// one per line under their own section, and a role declaring none has no such
// section.
func TestAPromptPrintsTheRolesClassesOnlyWhenItDeclaresThem(t *testing.T) {
	r := handRound()
	r.Roles[1] = role.Role{
		ID: "correctness", Title: "Correctness", Axis: axis.Correctness, Instructions: "Check it.",
		Classes: []string{"unchecked-error", "off-by-one"},
	}
	for _, prompt := range Emit(r) {
		_, section, found := strings.Cut(prompt.Text, "\n## Classes (§2.5)\n\n")
		assert.Equal(t, prompt.Role == "correctness", found, "%s on %s", prompt.Role, prompt.Unit)
		if !found {
			continue
		}
		body, _, _ := strings.Cut(section, "\n## ")
		assert.Equal(t, "The role's class vocabulary. `cr record` reports a record of this role whose class is not "+
			"one of these, and does not reject it (§2.5.6):\n- unchecked-error\n- off-by-one\n", body)
	}
}
