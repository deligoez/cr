package intent

import (
	"strings"

	"github.com/deligoez/cr/internal/text"
)

// Clean is what cr does to issue text between reading it and storing it:
// every terminal control sequence is removed, and every U+00A0 NO-BREAK SPACE
// becomes a U+0020 SPACE. Nothing else changes. Line breaks, trailing padding
// and the places a tracker wrapped its lines stay exactly as the source
// produced them, because §3.3 draws a claim from a verbatim span and a span
// that could run across a joined wrap could straddle two items of the issue.
//
// Both were measured on a field trial: `jira issue view --plain` appended its
// footer lines with ANSI colour sequences, and a span copied from the printed
// text failed on a U+00A0 the terminal showed as a space. Neither is text a
// reader of the issue sees, so a span copied from `cr brief`'s printed issue
// text — which is this function's output — is a span that occurs.
//
// A text that is not valid UTF-8 is returned unchanged. §1.4 refuses it with
// exit code 1 wherever it is hashed, and removing an escape sequence could join
// two stray bytes into a valid character and let a text through that §1.4
// would have refused.
func Clean(raw string) string {
	return clean(raw, false)
}

// clean is Clean, and with spellTargets set it also writes each OSC 8
// hyperlink's target, between two spaces, where the sequence stood. That form
// is never stored or printed: it is the text Reading.Links scans, because a
// terminal hyperlink's visible text need not carry its URL, and a link Clean
// removed with its escape is still a link the issue carries.
func clean(raw string, spellTargets bool) string {
	if text.CheckUTF8(raw) != nil {
		return raw
	}
	return strings.ReplaceAll(stripControlSequences(raw, spellTargets), " ", " ")
}

// The bytes the control sequences stripControlSequences removes are built of.
const (
	escape         = 0x1b
	bell           = 0x07
	csiIntroducer  = '['
	oscIntroducer  = ']'
	stringTerminal = '\\'
)

// stripControlSequences removes every ECMA-48 escape sequence from s, and every
// ESC byte that begins none.
//
// A CSI sequence (ESC [, parameter and intermediate bytes, one final byte in
// 0x40–0x7E) is the colour and cursor control a terminal-oriented tool writes;
// an OSC sequence (ESC ], ended by BEL or ESC \) is how a hyperlink or a window
// title is written; any other escape is ESC, its intermediate bytes, and one
// final byte. A sequence cut short ends where a byte that cannot continue it
// appears, and a line feed never belongs to one, so no removal joins two
// lines. Every ESC is removed, which is what makes Clean idempotent: its output
// holds no byte a second pass could act on.
func stripControlSequences(s string, spellTargets bool) string {
	if strings.IndexByte(s, escape) < 0 {
		return s
	}
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != escape {
			out.WriteByte(s[i])
			i++
			continue
		}
		end := sequenceEnd(s, i)
		if target, ok := hyperlinkTarget(s[i:end]); spellTargets && ok {
			out.WriteString(" " + target + " ")
		}
		i = end
	}
	return out.String()
}

// hyperlinkTarget returns the URI of an OSC 8 hyperlink sequence — ESC ] 8 ;
// params ; URI, then its terminator — and false for every other sequence. The
// sequence that closes a hyperlink carries an empty URI.
func hyperlinkTarget(sequence string) (string, bool) {
	body, isHyperlink := strings.CutPrefix(sequence, string([]byte{escape, oscIntroducer})+"8;")
	_, target, _ := strings.Cut(body, ";")
	target = strings.TrimSuffix(strings.TrimSuffix(target, string(rune(bell))), string([]byte{escape, stringTerminal}))
	return target, isHyperlink
}

// sequenceEnd returns the index just past the escape sequence starting at the
// ESC at s[start].
func sequenceEnd(s string, start int) int {
	i := start + 1
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case csiIntroducer:
		i++
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3f {
			i++
		}
		if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7e {
			i++
		}
		return i
	case oscIntroducer:
		for i++; i < len(s); i++ {
			switch {
			case s[i] == bell:
				return i + 1
			case s[i] == escape && i+1 < len(s) && s[i+1] == stringTerminal:
				return i + 2
			case s[i] == '\n' || s[i] == escape:
				return i
			}
		}
		return i
	}
	for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
		i++
	}
	if i < len(s) && s[i] >= 0x30 && s[i] <= 0x7e {
		i++
	}
	return i
}
