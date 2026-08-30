package unit

import (
	"slices"
	"testing"

	"github.com/deligoez/cr/internal/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shipped parses one of the profiles cr ships, so detectability is asked of
// the files a user really gets rather than of a profile the test invented.
func shipped(t *testing.T, id string) profile.Profile {
	t.Helper()
	p, err := profile.Parse(id+".json", []byte(profile.Builtins()[id]))
	require.NoError(t, err)
	return p
}

// indexOf is a SymbolIndex over a fixed set of paths, standing in for §4.3.1's
// head index until the task that builds it lands. It is the shape that index
// has to satisfy, which is the point of writing the interface this narrow.
type indexOf []string

func (i indexOf) Indexed(path string) bool { return slices.Contains(i, path) }

// §3.4.3 makes an enclosing symbol detectable only when the profile declares
// symbols.lang and cr can build a symbol index for the file. Each half is
// asserted missing on its own as well as present together, because either
// alone is a symbol cr cannot actually name: a declared language with no index
// leaves nothing to look a line up in, and an index with no declared language
// is one cr had no basis to build and no reason to trust for this repository.
//
// The negative answers are three and not one — no language, no index at all,
// and a file this index does not cover — because §3.4.3 is per file. A profile
// declaring php does not make a symbol detectable in a file the index skipped.
func TestASymbolIsDetectableOnlyWithBothALanguageAndAnIndex(t *testing.T) {
	php := shipped(t, "laravel-pest")
	generic := shipped(t, "generic")
	require.Equal(t, "php", php.Symbols.Lang)
	require.Empty(t, generic.Symbols.Lang)

	index := indexOf{"app/Money.php"}

	assert.True(t, Detectable(&php, index, "app/Money.php"))
	assert.False(t, Detectable(&php, index, "app/Other.php"), "a file the index does not cover")
	assert.False(t, Detectable(&php, nil, "app/Money.php"), "no index at all")
	assert.False(t, Detectable(&generic, index, "app/Money.php"), "no symbols.lang")
}
