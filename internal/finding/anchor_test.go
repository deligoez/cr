package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §9.2 writes one rule for both sizes: the content hash is the normalised hash
// per §1.4 of the lines from start_line to line inclusive, treated as one text,
// "so a single-line anchor and a multi-line anchor hash by the same rule".
//
// Asserting that the two calls return what strings.Join and text.NormalisedHash
// return would restate the implementation, so what is asserted here is what that
// sentence promises and a special-cased one-line path would quietly break:
// §1.4's normalisation reaches a one-line anchor exactly as it reaches a
// three-line one, the pre-image of several lines is those lines under one LF
// each — which is the text a one-line anchor over the same content carries — and
// the digest is taken over the whole range rather than its first line.
func TestAOneLineAndAThreeLineAnchorHashByTheSameFunction(t *testing.T) {
	one := hashOf(t, []string{"return $this->total;"})
	three := hashOf(t, []string{"if ($discount) {", "    $total -= $discount;", "}"})

	// §1.4 runs on both sizes. Trailing whitespace is what step 3 strips
	// and tabbed indentation is what step 4 collapses; a path that reached
	// the digest without normalising would show up here rather than as two
	// sites that agree round after round and never agree with each other.
	assert.Equal(t, one, hashOf(t, []string{"return $this->total;  "}))
	assert.Equal(t, three, hashOf(t, []string{"if ($discount) {", "\t$total -= $discount;", "}\t"}))

	// One text is one text however it was split on the way in, so the
	// three-line anchor hashes as the one-line anchor carrying the same LFs.
	assert.Equal(t, three, hashOf(t, []string{"if ($discount) {\n $total -= $discount;\n}"}))

	// The whole range reaches the digest. A hash of the first line alone,
	// or one blind to order, satisfies every assertion above.
	assert.NotEqual(t, three, hashOf(t, []string{"if ($discount) {"}))
	assert.NotEqual(t, three, hashOf(t, []string{"}", "    $total -= $discount;", "if ($discount) {"}))
}

// hashOf is one anchor's content hash, with §1.4 step 1's error out of the way:
// every pre-image here is valid UTF-8, so a failure is the test's own fault.
func hashOf(t *testing.T, lines []string) string {
	t.Helper()
	sum, err := AnchorContentHash(lines)
	require.NoError(t, err)
	return sum
}
