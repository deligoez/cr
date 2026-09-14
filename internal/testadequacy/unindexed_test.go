package testadequacy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The symbol half owes an entry for files outside the head index only when a
// unit of the round is a test file, since a round with no test file has no
// reference for the missing symbols to be absent from — and never when the
// index covers every file.
func TestTheSymbolHalfOwesUnindexedFilesAnEntryOnlyWhenATestFileChanged(t *testing.T) {
	p := laravelPest(t)
	outside := []string{"src/Money.php"}

	assert.Equal(t, []Unavailable{{
		Lens: SymbolLens,
		Reason: "profile \"laravel-pest\" builds §4.3.1's symbol index over its match.globs, which cover none " +
			"of src/Money.php, and those files declare symbols at the head, so a symbol the test files reference " +
			"from them is not attached; add a glob covering them to the profile's match.globs",
		author: "lens test/symbols did not look at src/Money.php: cr's index of the repository's existing code " +
			"does not cover it, so cr did not read which code there the changed tests exercise",
	}}, Unindexed(&p, []string{"src/Money.php", "tests/Unit/MoneyTest.php"}, outside))
	assert.Equal(t, []Unavailable{}, Unindexed(&p, []string{"src/Money.php", "README.md"}, outside),
		"no unit is a test file")
	assert.Equal(t, []Unavailable{}, Unindexed(&p, []string{"tests/Unit/MoneyTest.php"}, []string{}),
		"the index covers every file")
}
