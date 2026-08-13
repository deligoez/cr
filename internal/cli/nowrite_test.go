package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
)

// Invariant 3 and §2.1.2, asserted from outside internal/gh, which is the only
// place the assertion means anything: a boundary tested only by the package
// that declares it proves that the package agrees with itself.
//
// This is the shape a caller that has not been through §8.5's gate is in. It
// can reach the read door, which refuses a write. It can name the type, and
// the only value of it available here is the zero value, because granted is
// unexported and Confirm is the sole constructor — so the write door refuses
// it too. There is no third door: gh.Run and Confirmation.Write are the whole
// exported surface that starts the binary.
func TestAWriteFromOutsideTheGateIsRefused(t *testing.T) {
	review := []string{"api", "repos/cli/cli/pulls/11451/reviews", "--method", "POST", "--input", "-"}
	mutation := []string{"api", "graphql", "-f", "query=mutation{addComment(input:{body:\"x\"}){clientMutationId}}"}

	for name, attempt := range map[string]func() (string, error){
		"a POST through the read door":     func() (string, error) { return gh.Run(review...) },
		"a mutation through the read door": func() (string, error) { return gh.Run(mutation...) },
		"a POST with a fabricated token": func() (string, error) {
			return gh.Confirmation{}.Write(review...)
		},
		"a mutation with a fabricated token": func() (string, error) {
			return gh.Confirmation{}.Write(mutation...)
		},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := attempt()

			var refused *gh.WriteRefusedError
			require.ErrorAs(t, err, &refused)
			assert.Empty(t, out)
			assert.Contains(t, refused.Error(), "§2.1.2")
		})
	}
}
