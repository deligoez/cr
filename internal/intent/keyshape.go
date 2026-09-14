package intent

import (
	"fmt"
	"regexp"
)

// KeyShapeError reports an issue key typed at the command line that
// `intent.key_pattern` does not match whole.
//
// §3.2 resolves every key a round carries through the pattern, so a key it
// would never produce names a store no round loads. It is also the only thing
// keeping one issue in one store: §2.2 keeps the store at
// context/<ISSUE-KEY>.ndjson, and on a case-insensitive disk `cr-1` opens the
// file `CR-1` does while counting its ids apart. §11.2 codes the refusal 1.
type KeyShapeError struct {
	// Key is the key as it was typed.
	Key string
	// Pattern is the `intent.key_pattern` in force.
	Pattern string
}

func (e *KeyShapeError) Error() string {
	return fmt.Sprintf(
		"issue key %q is not one %s %q matches whole; §3.2 resolves every issue key through that pattern",
		e.Key, keyPatternField, e.Pattern,
	)
}

// CheckKey refuses a key pattern does not match from its first byte to its
// last, and a pattern that does not compile with KeyPatternError.
//
// The match is anchored here although ResolveKey's is not: ResolveKey finds a
// key inside a branch name or a sentence, while this is handed the key alone,
// so `CR-1#n1` holds a match and is still not a key.
func CheckKey(key, pattern string) error {
	if _, err := regexp.Compile(pattern); err != nil {
		return &KeyPatternError{Pattern: pattern, Err: err}
	}
	if !regexp.MustCompile(`^(?:` + pattern + `)$`).MatchString(key) {
		return &KeyShapeError{Key: key, Pattern: pattern}
	}
	return nil
}
