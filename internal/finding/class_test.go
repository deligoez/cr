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

// The rejection has to be openable: §6.1.3 names the line and the field for a
// missing one, and a bad class is no harder to find than a missing one. The
// value is quoted because the faults that reach here are invisible otherwise —
// a trailing space, a capital, a newline.
func TestAnInvalidClassNamesTheLineAndTheValue(t *testing.T) {
	err := ValidateClass("review-correctness.ndjson", 7, "Missing Test")

	var invalid *InvalidClassError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, "review-correctness.ndjson", invalid.File)
	assert.Equal(t, 7, invalid.Line)
	assert.Equal(t, "Missing Test", invalid.Class)
	assert.Equal(
		t,
		`review-correctness.ndjson line 7: class "Missing Test" is not kebab-case; §6.1 fixes the form at [a-z0-9-]+`,
		err.Error(),
	)
}
