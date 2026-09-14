// Package text holds the one text transformation spec/0.1.0.md §1.4 defines.
//
// §1.4 calls it "exactly this transformation and no other", and six sections
// hash or compare through it: §2.6.3 groups harvested comment bodies by
// normalised body, §3.3 hashes a claim's span and the issue text behind it,
// §3.4.6 hashes a unit's changed lines, §7.4 keys a waiver by the anchored
// lines and their context, §8.3.3 hashes the posted payload, and §9.2 hashes
// an anchor's content.
// Those callers sit in different packages and no one of them owns the
// transform, so it lives in a leaf package that imports nothing of cr's.
// Putting it in internal/unit beside the unit hash would make internal/finding
// and internal/intent import unit in order to compare two strings.
//
// Everything the transform feeds is a comparison — deduplication, waiver keys,
// anchor identity — which is why §1.4's three negatives are load-bearing rather
// than decorative. A step that quietly case-folded would make two findings
// differing only in case hash alike, and §6.4's suppression would drop a real
// one on the strength of it.
package text

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// InvalidUTF8Error reports input that §1.4 step 1 refuses to decode.
//
// It carries the offset and the byte because that is all the user can act on:
// the text came from an issue tracker, a diff, or a comment body, and the only
// repair is to hand cr the same text encoded as UTF-8. The cli layer maps it
// onto exit code 1 — the file was found and read, and what is wrong is the
// bytes inside it.
type InvalidUTF8Error struct {
	// Offset is the zero-based index of the first byte that starts no
	// UTF-8 sequence.
	Offset int
	// Byte is that byte, so the error can show what was rejected.
	Byte byte
}

func (e *InvalidUTF8Error) Error() string {
	return fmt.Sprintf(
		"byte %d is 0x%02x, which begins no UTF-8 sequence; §1.4 decodes as UTF-8 and "+
			"normalises nothing else — re-encode the source as UTF-8 and run the command again",
		e.Offset, e.Byte,
	)
}

// Normalise applies the six steps of §1.4, in the order §1.4 numbers them.
//
// It takes a string rather than a []byte even though step 1 is a decode,
// because a Go string is a byte sequence and carries invalid UTF-8 through
// unharmed: string(b) copies bytes and replaces nothing. Decoding is what
// replaces — []rune(s) and a `for range` over s both yield U+FFFD for an
// invalid byte, and a transform that decoded that way would have nothing left
// to detect. So step 1 runs first, over the bytes, before any step can lose the
// evidence; every step after it compares bytes against SPACE, TAB and LF, none
// of which a UTF-8 continuation byte can impersonate.
func Normalise(in string) (string, error) {
	// Step 1: decode as UTF-8; invalid input fails with exit code 1.
	if err := CheckUTF8(in); err != nil {
		return "", err
	}

	// Step 2: replace CRLF and lone CR with LF. CRLF goes first, so a
	// Windows line ending becomes one LF rather than two.
	unified := strings.ReplaceAll(in, "\r\n", "\n")
	unified = strings.ReplaceAll(unified, "\r", "\n")

	lines := strings.Split(unified, "\n")
	kept := make([]string, 0, len(lines))
	// pending records that a blank line was seen with a kept line already
	// behind it. It is how step 5 drops the leading and trailing runs
	// without a second pass: a blank before anything is kept never sets it,
	// and a blank at the end sets it and is never spent.
	pending := false
	for _, line := range lines {
		// Step 3: strip trailing SPACE and TAB, and nothing else.
		line = strings.TrimRight(line, " \t")
		// Step 4: collapse every run of SPACE or TAB, leading
		// indentation included, to a single SPACE.
		line = collapseRuns(line)
		if line == "" {
			// Step 5: a blank line is remembered, never written
			// straight through, so a run of them collapses to one.
			pending = len(kept) > 0
			continue
		}
		if pending {
			kept = append(kept, "")
			pending = false
		}
		kept = append(kept, line)
	}

	// Step 6: rejoin with a single LF and no trailing one, so a one-line
	// input normalises to that line with no terminator.
	return strings.Join(kept, "\n"), nil
}

// collapseRuns is step 4. It scans bytes rather than runes because U+0020 and
// U+0009 are ASCII, and no byte of a multi-byte UTF-8 sequence is ever below
// 0x80 — so a byte equal to SPACE or TAB is that character and never part of
// another one.
func collapseRuns(line string) string {
	var out strings.Builder
	inRun := false
	for i := 0; i < len(line); i++ {
		if c := line[i]; c == ' ' || c == '\t' {
			if !inRun {
				out.WriteByte(' ')
				inRun = true
			}
			continue
		}
		out.WriteByte(line[i])
		inRun = false
	}
	return out.String()
}

// CheckUTF8 is §1.4 step 1 alone: nil when in decodes as UTF-8, and the
// InvalidUTF8Error naming the first byte that does not otherwise.
//
// It is exported for the text cr stores without normalising. encoding/json
// replaces an invalid byte with U+FFFD on both decode and encode, so text that
// reaches a record or a note unchecked is stored as something nobody wrote, and
// the refusal step 1 gives normalised text is the one that text gets too.
func CheckUTF8(in string) error {
	if offset := firstInvalidUTF8(in); offset >= 0 {
		return &InvalidUTF8Error{Offset: offset, Byte: in[offset]}
	}
	return nil
}

// firstInvalidUTF8 returns the offset of the first byte that begins no UTF-8
// sequence, or -1 when the whole string decodes.
//
// The size test is what separates a broken byte from a legitimately encoded
// U+FFFD: the decoder reports the replacement character for both, but a real
// one occupies three bytes and an undecodable byte exactly one. Without it a
// text that spells U+FFFD on purpose would be rejected as malformed.
func firstInvalidUTF8(s string) int {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			return i
		}
		i += size
	}
	return -1
}
