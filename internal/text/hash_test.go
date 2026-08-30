package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pinnedFragment is the input the fixed value below is taken over, and it is
// the realistic fragment from the six-step table rather than a word: every one
// of §1.4's steps moves something in it, so the normal form
// (" func f() {\n\n return 1\n}") shares no byte sequence of any length with
// what goes in. A hash that skipped Normalise, or ran four steps of the six,
// therefore cannot land on the same value by accident.
const pinnedFragment = "\r\n\tfunc  f() {\t\r\n\r\n\r\n\t\treturn  1\t\r\n}\r\n\r\n"

// pinnedHash is SHA-256 over pinnedFragment's normal form, lowercase hex, first
// sixteen characters. It is written out as a literal on purpose: an expected
// value computed the way the code computes it agrees with any implementation,
// including a wrong one, and this one has to disagree with all but the right
// one. §3.4.6, §7.4.1, §8.3.3, §9.2 and §6.1 all name this value, so it is a
// contract with five sections and not an artefact of the current code — if a
// change moves it, the change is wrong or every stored hash in ~/.cr is.
const pinnedHash = "46d80e6defec6c66"

// The value is asserted, and so is the fact that it is the *normalised* text's.
// Hashing pinnedFragment as it stands is what a caller that forgot step zero
// would produce, and it is a perfectly well-formed sixteen-character lowercase
// hex string — which is why the shape assertions alone would pass it.
func TestNormalisedHashIsTheValueSection14Pins(t *testing.T) {
	got, err := NormalisedHash(pinnedFragment)
	require.NoError(t, err)
	assert.Equal(t, pinnedHash, got, "§1.4's value for this fragment, fixed across every section that names it")

	assert.Len(t, got, 16, "§1.4 truncates to the first 16 characters")
	assert.Regexp(t, `^[0-9a-f]{16}$`, got, "§1.4 says lowercase hex")

	normalised, err := Normalise(pinnedFragment)
	require.NoError(t, err)
	require.NotEqual(t, pinnedFragment, normalised, "the fragment is only worth pinning because the steps move it")
	viaNormalForm, err := NormalisedHash(normalised)
	require.NoError(t, err)
	assert.Equal(t, pinnedHash, viaNormalForm,
		"the digest is taken over the normal form, so handing it the normal form changes nothing")

	assert.NotEqual(t, "0e8cce8b6619337f", got,
		"and not over the raw bytes, whose own §1.4-shaped hash is this and is wrong")
}
