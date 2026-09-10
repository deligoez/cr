package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// §8.1.5: a `kind=question` body with no `?` is refused with exit code 1,
// naming the record id.
//
// It is exercised on the body rather than through a command, for the reason
// §6.3.3's rejection is: the command that carries it is `cr post`, and v0.1 has
// not built it yet. What this pins is the code the refusal maps to, which is
// §11.2's and lives here.
func TestADeclarativeQuestionIsCodedOneNamingTheRecord(t *testing.T) {
	err := render.ValidatePostBody("f9", finding.KindQuestion, "The error Decode returns is dropped.")
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§8.1.5 refuses with exit code 1")
	assert.Contains(t, err.Error(), "f9", "§8.1.5: the refusal names the record id")

	require.NoError(t,
		render.ValidatePostBody("f9", finding.KindQuestion, "Is the error Decode returns dropped?"))
}
