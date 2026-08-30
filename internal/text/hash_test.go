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

// The hash is only useful because of what it lets a caller conclude from an
// equality, and every section that names it concludes something: §6.4 suppresses
// a duplicate finding, §7.4.1 matches a waiver against unchanged code, §3.3
// decides an issue has drifted, §9.2 identifies an anchor's content. All four
// read equal hashes as "the same text" and unequal ones as "different text", so
// the property worth asserting is the biconditional and not either half.
//
// It runs over the corpus the six-step tests share, which is built of the
// shapes the steps disagree about, so the pairs it forms are exactly the ones
// where an implementation that hashed too early or too late would part company
// with Normalise.
func TestNormalisedHashAgreesExactlyWithNormalisation(t *testing.T) {
	forms := make([]string, len(normalisationCorpus))
	hashes := make([]string, len(normalisationCorpus))
	for i, in := range normalisationCorpus {
		form, err := Normalise(in)
		require.NoError(t, err)
		hash, err := NormalisedHash(in)
		require.NoError(t, err)
		forms[i], hashes[i] = form, hash
	}

	sawEqual, sawDiffer := false, false
	for i := range normalisationCorpus {
		for j := i + 1; j < len(normalisationCorpus); j++ {
			if forms[i] == forms[j] {
				sawEqual = true
				assert.Equal(t, hashes[i], hashes[j],
					"%q and %q normalise alike, so nothing downstream may tell them apart",
					normalisationCorpus[i], normalisationCorpus[j])
				continue
			}
			sawDiffer = true
			assert.NotEqual(t, hashes[i], hashes[j],
				"%q and %q normalise differently, so nothing downstream may treat them as one",
				normalisationCorpus[i], normalisationCorpus[j])
		}
	}
	require.True(t, sawEqual, "the corpus held no pair that normalises alike, so half the property went untested")
	require.True(t, sawDiffer, "the corpus held no pair that normalises differently, so the other half did")
}
