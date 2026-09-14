package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// utf8Hint is the step exit.go's row gives text §1.4 step 1 refuses.
const utf8Hint = "§1.4 requires UTF-8; re-encode the text the message names"

// holdRecords writes lines as one pull request's findings.ndjson, the records
// `cr answer` resolves the id it is given against.
func holdRecords(t *testing.T, l state.Layout, owner, repo string, pr int, lines ...string) {
	t.Helper()
	path := l.PRFile(owner, repo, pr, state.FileFindings)
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
}

// answerable is a posted question with the id f3, the record every answer the
// tests here give is filed against.
const answerable = `{"id":"f3","kind":"question","summary":"why is the retry unbounded?",` +
	`"state":"posted","head":"0f1e2d3","round":1}`

// §1.4 step 1 through `cr note` and `cr answer`: a text that is not UTF-8 is
// refused with exit 1 rather than stored with U+FFFD in place of the byte, and
// the store is not created.
func TestNoteAndAnswerRefuseTextThatIsNotUTF8(t *testing.T) {
	const refused = "the note text: byte 4 is 0xff, which begins no UTF-8 sequence; §1.4 decodes as UTF-8 " +
		"and normalises nothing else — re-encode the source as UTF-8 and run the command again"
	for name, args := range map[string][]string{
		"cr note":   {"note", "CR-7", "bad \xff byte", "--source", "chat", "--pr", "9"},
		"cr answer": {"answer", answeredPR, "f3", "bad \xff byte", "--source", "chat", "--repo", answeredSlug},
	} {
		t.Run(name, func(t *testing.T) {
			layout := briefedHome(t, "CR-7")
			holdRecords(t, layout, answeredOwner, answeredRepo, answeredPRNum, answerable)

			out, err := runIn(t, args...)

			require.Error(t, err)
			assert.Equal(t, refused, err.Error())
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, utf8Hint, hintFor(err))
			assert.Empty(t, out)
			assert.NoFileExists(t, layout.ContextFile("CR-7"), "nothing was appended")
		})
	}
}

// §3.2 through `cr note`: a key the effective intent.key_pattern does not
// match whole is refused with exit 1 naming the pattern, before any store is
// opened — so `cr-1` cannot share `CR-1`'s file on a case-insensitive disk, and
// `CR-1#n1` cannot open a store of its own.
func TestNoteRefusesAKeyThePatternDoesNotMatchWhole(t *testing.T) {
	const hint = "name the issue key the way `intent.key_pattern` matches it, e.g. CR-1; " +
		"`cr config --resolved` shows the pattern in force"
	for _, key := range []string{"cr-1", "CR-1#n1", "xCR-1", "CR-1 "} {
		t.Run(key, func(t *testing.T) {
			checkoutWithRemotes(t)
			layout := briefedHome(t, "CR-7")

			out, err := runIn(t, "note", key, "hearsay", "--source", "chat", "--pr", "9")

			require.Error(t, err)
			assert.Equal(t, `issue key "`+key+`" is not one intent.key_pattern "[A-Z][A-Z0-9]+-[0-9]+" matches whole; `+
				"§3.2 resolves every issue key through that pattern", err.Error())
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, hint, hintFor(err))
			assert.Empty(t, out)
			assert.NoFileExists(t, layout.ContextFile(key))
			assert.NoFileExists(t, layout.ContextFile("CR-1"))
		})
	}

	// The pattern is the effective one, the per-repository layer included:
	// narrowed to lower case there, the lower-case key is the key.
	t.Run("the pattern the repository configures", func(t *testing.T) {
		layout := briefedHome(t, "CR-7")
		perRepo := layout.RepoConfig(answeredOwner, answeredRepo)
		require.NoError(t, os.MkdirAll(filepath.Dir(perRepo), 0o700))
		require.NoError(t, os.WriteFile(perRepo, []byte(`{"intent": {"key_pattern": "[a-z]+-[0-9]+"}}`), 0o600))

		_, err := runIn(t, "note", "cr-1", "hearsay", "--source", "chat", "--pr", "9", "--repo", answeredSlug)
		require.NoError(t, err)
		stored, err := note.Load(layout, "cr-1")
		require.NoError(t, err)
		require.Len(t, stored, 1)
		assert.Equal(t, "cr-1#n1", stored[0].ID)

		_, err = runIn(t, "note", "CR-2", "hearsay", "--source", "chat", "--pr", "9", "--repo", answeredSlug)
		require.Error(t, err)
		assert.Equal(t, `issue key "CR-2" is not one intent.key_pattern "[a-z]+-[0-9]+" matches whole; `+
			"§3.2 resolves every issue key through that pattern", err.Error())
		assert.Equal(t, ExitValidation, exitCodeFor(err))
	})
}

// §3.6.2 through `cr answer`: an id spelled the way §6.1 spells one that no
// record of the pull request holds is refused with exit 1 naming it, and no
// note is appended. The records of every round count, so the refusal is not
// about the round.
func TestAnswerRefusesARecordIDThePullRequestDoesNotHold(t *testing.T) {
	layout := briefedHome(t, "CR-7")
	holdRecords(t, layout, answeredOwner, answeredRepo, answeredPRNum, answerable)

	out, err := runIn(t, "answer", answeredPR, "f9", "answered", "--source", "chat", "--repo", answeredSlug)

	require.Error(t, err)
	assert.Equal(t, "no record f9 is stored for acme/web#42, in any round: "+
		"§3.6.2 files an answer against a record the pull request holds", err.Error())
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, "answer a record the pull request holds, by the id `cr record` reported when it stored the record",
		hintFor(err))
	assert.Empty(t, out)
	assert.NoFileExists(t, layout.ContextFile("CR-7"), "nothing was appended")
}
