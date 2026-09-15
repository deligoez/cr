package intent

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/text"
)

// Field feedback 1.2: `jira issue view --plain` appended colour sequences to its
// footer lines and carried a U+00A0 inside a sentence. Clean takes out exactly
// those and leaves every line break, padding run and wrap where it was.
func TestCleanRemovesControlSequencesAndNoBreakSpacesOnly(t *testing.T) {
	for name, pair := range map[string]struct{ raw, cleaned string }{
		"an SGR colour sequence around a footer": {
			raw:     "\x1b[38;5;242mView this issue on Jira\x1b[0m\n",
			cleaned: "View this issue on Jira\n",
		},
		"a no-break space inside a sentence": {
			raw:     "Kampanya tutari 50 TL.",
			cleaned: "Kampanya tutari 50 TL.",
		},
		"padding and a hard wrap are kept": {
			raw:     "a line wrapped at the column   \nand its tail   \n\n",
			cleaned: "a line wrapped at the column   \nand its tail   \n\n",
		},
		"an OSC hyperlink ended by ST": {
			raw:     "see \x1b]8;;https://x.test\x1b\\the doc\x1b]8;;\x1b\\ now",
			cleaned: "see the doc now",
		},
		"an OSC title ended by BEL": {
			raw:     "\x1b]0;title\x07body",
			cleaned: "body",
		},
		"a charset designation and a lone escape": {
			raw:     "\x1b(Bone\x1b\ntwo\x1b",
			cleaned: "one\ntwo",
		},
		"an unterminated CSI never eats the line break": {
			raw:     "one\x1b[12\ntwo",
			cleaned: "one\ntwo",
		},
		"text with neither is unchanged": {
			raw:     "Retries back off.\r\n\tIndented.",
			cleaned: "Retries back off.\r\n\tIndented.",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, pair.cleaned, Clean(pair.raw))
			assert.Equal(t, pair.cleaned, Clean(pair.cleaned), "a second pass changes nothing")
		})
	}
}

// A text §1.4 refuses is handed on unchanged, so removing a sequence cannot
// join two stray bytes into a character and let it past the refusal.
func TestCleanLeavesInvalidUTF8ForNormalisationToRefuse(t *testing.T) {
	raw := "a\xc2\x1b[0m\xa0b"
	assert.Equal(t, raw, Clean(raw))
	reading := read(raw)
	assert.Equal(t, Reading{Text: raw}, reading)
	_, err := DetectDrift(slices.Values([]Claim{}), reading.Spans(nil))
	var invalid *text.InvalidUTF8Error
	require.ErrorAs(t, err, &invalid, "§1.4 still refuses the text")
}

// Both of §3.1's sources are cleaned alike: the file holding what the tracker
// command printed yields the text the command yields.
func TestBothIntentSourcesAreCleanedAlike(t *testing.T) {
	printed := "CR-1: pay now\n\x1b[2mfooter\x1b[0m\n"
	path := filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(path, []byte(printed), 0o600))
	tracker := stubTracker(t, `printf 'CR-1: pay\302\240now\n\033[2mfooter\033[0m\n'`)

	fromFile, err := Read(Source{File: path}, "CR-1")
	require.NoError(t, err)
	fromCommand, err := Read(Source{Cmd: []string{tracker, Placeholder}}, "CR-1")
	require.NoError(t, err)

	assert.Equal(t, Reading{Text: "CR-1: pay now\nfooter\n", asRead: printed}, fromFile)
	assert.Equal(t, fromFile, fromCommand)
}
