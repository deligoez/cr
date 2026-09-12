package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/suggestion"
)

// §8.2.4: a suggestion failing §8.2's validation is refused with exit code 1,
// naming the record id.
//
// It is exercised on the validator rather than through a command, for the
// reason §8.1.5's refusal is: the command that carries it is `cr post`, and
// v0.1 has not built it yet. What this pins is the code the refusal maps to,
// which is §11.2's and lives here — so the mapping is in place before the
// command that needs it, and a post wired to a different code fails here.
func TestAnUnplaceableSuggestionIsCodedOneNamingTheRecord(t *testing.T) {
	const diff = `--- a/app/Models/Order.php
+++ b/app/Models/Order.php
@@ -10,2 +10,2 @@
-$removed = 2;
+$added = 2;
 $tail = 3;
`
	hunks, err := git.ParseHunks(diff)
	require.NoError(t, err)

	record := &finding.Finding{
		ID: "f4", Kind: finding.KindFinding, Role: "correctness", Class: "dropped-error",
		Severity: finding.SeverityMedium, Unit: "u1",
		Anchor: finding.Anchor{
			Path: "app/Models/Order.php", Side: git.Right, StartLine: 400, Line: 400,
		},
		Summary: "The added line ignores the error.", Evidence: "The second result is dropped.",
		Suggestion: "$added = 3;",
	}

	refused := suggestion.Validate(record, hunks)

	require.Error(t, refused)
	assert.Equal(t, ExitValidation, exitCodeFor(refused), "§8.2.4 refuses with exit code 1")
	assert.Contains(t, refused.Error(), "f4", "§8.2.4: the refusal names the record id")

	record.Anchor.StartLine, record.Anchor.Line = 10, 11
	assert.NoError(t, suggestion.Validate(record, hunks),
		"the same record placed inside the hunk is admitted")
}
