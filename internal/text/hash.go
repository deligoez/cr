package text

import (
	"crypto/sha256"
	"encoding/hex"
)

// hashLength is §1.4's truncation: the first 16 characters of the lowercase hex
// digest, which is 64 bits of the 256 the digest carries.
//
// It is spelled once, here, because the number is the whole of what the callers
// could drift apart on. Five sections name the same value — §3.4.6's unit hash,
// §7.4.1's waiver key, §8.3.3's payload hash, §9.2's anchor content hash and the
// span and issue hashes §6.1 marks computed — and a site that truncated to 12
// would still produce a stable, self-consistent, lowercase hex string. Nothing
// downstream of it would fail; the two sites would simply never agree again.
const hashLength = 16

// NormalisedHash is the normalised hash §1.4 defines: SHA-256 over the
// normalised text, lowercase hex, truncated to the first 16 characters.
//
// It takes the raw text and runs §1.4's six steps itself, rather than taking
// text the caller already normalised. That choice is what makes "every hash
// routes through this function" true by construction instead of by convention.
// A signature taking already-normalised text can be called perfectly correctly —
// right function, right package, right spelling — on text that was never
// normalised, and the mistake is invisible: the result is still sixteen hex
// characters, still deterministic, still equal to itself round after round. It
// only shows up as two sites that never agree, which is exactly the failure the
// single entry point exists to prevent. Normalising inside means an unnormalised
// input cannot reach the digest at all.
//
// Nothing is lost on the other side. Normalise is idempotent, so a caller
// holding text it normalised for some other reason may pass it straight in and
// reach the same value.
//
// The error is step 1's, and it is returned rather than swallowed because §1.4
// makes undecodable input fail with exit code 1. A hash function that fell back
// to hashing the bytes anyway would turn a refusal into a value, and the value
// would look exactly like every other one.
func NormalisedHash(in string) (string, error) {
	normalised, err := Normalise(in)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(normalised))
	return hex.EncodeToString(sum[:])[:hashLength], nil
}
