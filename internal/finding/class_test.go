package finding

import (
	"testing"

	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §6.1 fixes the defect class at [a-z0-9-]+ and rejects any other form. The
// empty string is the value that decides the rule: a test built from misspelt
// classes alone passes whether the pattern requires one character or none, and
// a class of no characters would put every record that omitted one into a
// single §6.4.1 dedup group and a single §7.4 waiver.
func TestAClassIsKebabCaseOrNothing(t *testing.T) {
	for _, class := range []string{
		"missing-test", "a", "0", "reinvented-helper-2", "-leading", "trailing-",
	} {
		assert.NoError(t, ValidateClass(state.FileFindings, 1, class), class)
	}
	for _, class := range []string{
		"", "Missing-Test", "missing_test", "missing test", "missing.test",
		"eksik-testç", "missing-test\n", "missing-test\nother",
	} {
		assert.Error(t, ValidateClass(state.FileFindings, 1, class), "%q", class)
	}
}

