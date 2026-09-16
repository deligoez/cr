package intent

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anIntentFile writes body to name under a fresh directory and returns its
// path, which is also the path §3.1.5's separator line names.
func anIntentFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// §3.1.5's issue text: the first part, then each extra intent file under a
// separator line naming the path as given, in the order the paths were given.
//
// Neither the first part nor the first extra ends in a newline, so this also
// fixes what joining does about that: a separator is a line, so it starts on
// one, and the newline that puts it there belongs to the joined text and to no
// part — which is why every part is asserted whole beside the text.
func TestTheIssueTextIsTheFirstPartThenEachExtraFileUnderItsSeparator(t *testing.T) {
	design := anIntentFile(t, "design.md", "The queue drains oldest first.")
	api := anIntentFile(t, "api.md", "POST /jobs returns 202.\n")
	issue := anIntentFile(t, "issue.txt", "CR-1: drain the queue.")

	reading, err := Read(Source{File: issue, Extra: []string{design, api}}, "CR-1")
	require.NoError(t, err)

	assert.Equal(t, "CR-1: drain the queue.\n"+
		"--- cr intent file: "+design+" ---\n"+
		"The queue drains oldest first.\n"+
		"--- cr intent file: "+api+" ---\n"+
		"POST /jobs returns 202.\n", reading.Text)
	assert.Equal(t, []Part{
		{Text: "CR-1: drain the queue.\n"},
		{File: design, Text: "The queue drains oldest first.\n"},
		{File: api, Text: "POST /jobs returns 202.\n"},
	}, Parts(reading.Text))
	assert.Empty(t, reading.asRead, "there was nothing to clean, so there is no earlier form")
}

// §3.1.6 over §3.1.5: every source's text is cleaned — the first part's and
// every extra file's — and cr's own separator lines carry the path exactly as
// it was given. The bytes before cleaning are kept joined the same way, so
// §3.3.3 can still recognise a round recorded before cr cleaned anything.
func TestEveryPartOfTheIssueTextIsCleanedAndTheSeparatorsAreNot(t *testing.T) {
	design := anIntentFile(t, "design.md", "\x1b[2mDrain 50 TL orders first.\x1b[0m\n")
	issue := anIntentFile(t, "issue.txt", "CR-1: pay 50 TL.\n")

	reading, err := Read(Source{File: issue, Extra: []string{design}}, "CR-1")
	require.NoError(t, err)

	separator := "--- cr intent file: " + design + " ---\n"
	assert.Equal(t, "CR-1: pay 50 TL.\n"+separator+"Drain 50 TL orders first.\n", reading.Text)
	assert.Equal(t, "CR-1: pay 50 TL.\n"+separator+
		"\x1b[2mDrain 50 TL orders first.\x1b[0m\n", reading.asRead)
}

// §3.1.7 over §3.1.5: a link an extra intent file carries is one the issue text
// carries, terminal hyperlinks included — cr read the text it was given and not
// what any part of it links to.
func TestTheLinksOfTheIssueTextIncludeTheExtraFilesOwn(t *testing.T) {
	design := anIntentFile(t, "design.md",
		"See \x1b]8;;https://docs.test/queue\x1b\\the design\x1b]8;;\x1b\\ and https://docs.test/api.\n")
	issue := anIntentFile(t, "issue.txt", "CR-1: drain the queue.\n")

	reading, err := Read(Source{File: issue, Extra: []string{design}}, "CR-1")
	require.NoError(t, err)

	assert.Equal(t, []string{"https://docs.test/queue", "https://docs.test/api"}, reading.Links())
}

// An extra intent file cr cannot read fails the run, naming the flag that gave
// the path rather than `--intent-file`, and yields no issue text at all.
func TestAnUnreadableExtraIntentFileIsRefusedNamingItsOwnFlag(t *testing.T) {
	issue := anIntentFile(t, "issue.txt", "CR-1: drain the queue.\n")
	missing := filepath.Join(t.TempDir(), "absent.md")

	reading, err := Read(Source{File: issue, Extra: []string{missing}}, "CR-1")

	var refused *FileError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, missing, refused.Path)
	assert.Equal(t, ExtraFlag, refused.Flag)
	assert.True(t, os.IsNotExist(refused.Err))
	assert.Equal(t, Reading{}, reading, "a run that could not read a part reads no issue text")
}

// §3.1.5's order over the two sources, and its deduplication: the
// `--intent-extra` paths in the order given, then the `intent.extra_files`
// paths, each path once — within one source as well as across the two, since a
// path named twice would write its separator twice and leave §3.3.1 two parts
// to resolve one `file` against.
func TestExtraFilesOrdersTheFlagsFirstAndCountsEachPathOnce(t *testing.T) {
	assert.Equal(t, []string{"design.md", "api.md", "shared.md"},
		ExtraFiles([]string{"design.md", "api.md", "design.md"},
			[]string{"shared.md", "design.md", "shared.md"}))
	assert.Equal(t, []string{}, ExtraFiles(nil, nil))
}

// §3.3.1 through DecodeClaims: a span must lie wholly inside one part.
//
// The four claims are the partition's corners — a claim of the first part, a
// claim of an extra file, a claim whose span crosses a separator line, and a
// claim of the first part whose span is text only an extra file holds — so a
// check that searched the joined text instead of one part would let two of them
// through.
func TestASpanMustLieWhollyInsideOnePartOfTheIssueText(t *testing.T) {
	design := anIntentFile(t, "design.md", "The queue drains oldest first.\n")
	issue := anIntentFile(t, "issue.txt", "CR-1: drain the queue.\n")
	reading, err := Read(Source{File: issue, Extra: []string{design}}, "CR-1")
	require.NoError(t, err)

	line := func(source, span, file string) []byte {
		claim := `{"id":"CR-1#c1","text":"Drain.","source":"` + source + `","span":"` + span + `"`
		if file != "" {
			claim += `,"file":"` + file + `"`
		}
		return []byte(claim + "}\n")
	}
	held := []struct {
		name         string
		line         []byte
		field, holds string
	}{
		{name: "the first part", line: line("description", "drain the queue", "")},
		{name: "an extra file", line: line("file", "drains oldest first", design)},
		{
			name: "across the separator", field: "span",
			line:  line("description", "the queue.\\n--- cr intent file: ", ""),
			holds: "does not occur in the issue text before the first §3.1.5 separator line",
		},
		{
			name: "an extra file's text under another source", field: "span",
			line:  line("description", "drains oldest first", ""),
			holds: "does not occur in the issue text before the first §3.1.5 separator line",
		},
		{
			name: "the first part's text under a file source", field: "span",
			line:  line("file", "drain the queue", design),
			holds: "does not occur in the extra intent file " + design,
		},
		{
			name: "a file this round did not read", field: "file",
			line:  line("file", "drains oldest first", "other.md"),
			holds: `names "other.md", which is not an extra intent file of this round`,
		},
	}
	for _, claim := range held {
		t.Run(claim.name, func(t *testing.T) {
			decoded, err := DecodeClaims("claims.ndjson", claim.line, "CR-1", reading.Spans(nil))
			if claim.field == "" {
				require.NoError(t, err)
				require.Len(t, decoded, 1)
				return
			}
			var refused *RejectedClaimError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, claim.field, refused.Field)
			assert.Contains(t, refused.Problem, claim.holds)
		})
	}
}

// A round that read no extra intent file checks a span against the whole issue
// text and says so in as many words, because there is no separator line to send
// a reader looking for.
func TestARoundWithNoExtraIntentFileStillNamesTheIssueText(t *testing.T) {
	reading := read("CR-1: drain the queue.\n")

	_, err := DecodeClaims("claims.ndjson",
		[]byte(`{"id":"CR-1#c1","text":"Drain.","source":"description","span":"flush"}`+"\n"),
		"CR-1", reading.Spans(nil))

	var refused *RejectedClaimError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t,
		"does not occur in the issue text; §3.3 draws a claim from a verbatim substring of its source",
		refused.Problem)
}

// §3.3.4 over §3.1.5: a separator line is no paragraph and belongs to none, so
// it breaks the run of lines around it exactly as a blank line does.
func TestASeparatorLineIsNoParagraphAndBreaksTheRunAroundIt(t *testing.T) {
	issue := "Drain the queue.\n" +
		"--- cr intent file: design.md ---\n" +
		"The queue drains oldest first.\n"

	report := UncoveredParagraphs(issue, slices.Values([]Claim{}))

	assert.Equal(t, Paragraphs{Total: 2, Uncovered: []Paragraph{
		{StartLine: 1, EndLine: 1, Text: "Drain the queue."},
		{StartLine: 3, EndLine: 3, Text: "The queue drains oldest first."},
	}}, report)
}

// A line that only looks like a separator is text like any other: the shape
// §3.1.5 fixes is the whole line, prefix and suffix both.
func TestALineThatIsNotASeparatorIsPartOfItsParagraph(t *testing.T) {
	assert.True(t, IsSeparator("--- cr intent file: design.md ---"))
	assert.True(t, IsSeparator("--- cr intent file: design.md ---\r"))
	for _, line := range []string{
		"--- cr intent file: design.md", "cr intent file: design.md ---",
		"--- cr intent file:design.md ---", "  --- cr intent file: design.md ---",
		"--- cr intent file: ---",
	} {
		assert.False(t, IsSeparator(line), "%q is not §3.1.5's separator line", line)
	}
}
