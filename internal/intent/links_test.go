package intent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Field feedback 1.1: the issue linked a Google Docs spec and nothing said cr
// had not read it. Links finds every URL, once each, in the order the text
// holds them, with the wrapper a tracker prints around a link left out.
func TestLinksListsEveryURLTheIssueTextCarries(t *testing.T) {
	issue := "Spec: https://docs.google.com/document/d/1AbC/edit#heading=h.x1.\n" +
		"See [the design|https://www.figma.com/file/Q9?node-id=1-2] and (http://wiki.test/Page).\n" +
		"Again: https://docs.google.com/document/d/1AbC/edit#heading=h.x1, and <HTTPS://Example.test/a_b>.\n" +
		"Not a link: ftp://files.test, https:// alone, or docs.google.com without a scheme."

	assert.Equal(t, []string{
		"https://docs.google.com/document/d/1AbC/edit#heading=h.x1",
		"https://www.figma.com/file/Q9?node-id=1-2",
		"http://wiki.test/Page",
		"HTTPS://Example.test/a_b",
	}, Links(issue))
}

// An issue text with no link yields an empty list, never nil, so the payload
// the list reaches says `[]`.
func TestLinksOfTextWithoutALinkIsEmpty(t *testing.T) {
	assert.Equal(t, []string{}, Links("CR-1: format money amounts.\n"))
	assert.Equal(t, []string{}, Links(""))
}

// v0.2.3 QA D-T1-1: a terminal hyperlink whose visible text carries no URL is
// still a link the issue carries. A reading's links name every OSC 8 target
// Clean removed, ended by ST or BEL, with or without parameters, in the order
// the source held them and once each beside a plain URL naming the same
// target; the text cr stores is Clean's, unchanged.
func TestReadingLinksNameTheTargetsOfTerminalHyperlinks(t *testing.T) {
	raw := "See \x1b]8;;https://docs.test/spec\x1b\\the spec\x1b]8;;\x1b\\ first.\n" +
		"Then https://plain.test/a and \x1b]8;id=7;https://bell.test/b\x07the board\x1b]8;;\x07.\n" +
		"Again \x1b]8;;https://plain.test/a\x1b\\https://plain.test/a\x1b]8;;\x1b\\, " +
		"\x1b]0;https://title.test/t\x07a title, and \x1b[1mbold\x1b[0m.\n"
	reading := read(raw)
	assert.Equal(t, "See the spec first.\nThen https://plain.test/a and the board.\n"+
		"Again https://plain.test/a, a title, and bold.\n", reading.Text)
	assert.Equal(t, []string{"https://docs.test/spec", "https://plain.test/a", "https://bell.test/b"}, reading.Links())
}

// A reading Clean changed nothing in, or one carrying no link, lists what its
// text holds: its plain URLs, or nothing.
func TestReadingLinksOfTextCleanLeftAlone(t *testing.T) {
	plain := read("Spec: https://docs.test/spec.\n")
	assert.Equal(t, Reading{Text: "Spec: https://docs.test/spec.\n"}, plain)
	assert.Equal(t, []string{"https://docs.test/spec"}, plain.Links())
	assert.Equal(t, []string{}, read("\x1b]8;;\x1b\\CR-1\x1b[0m: no link here.\n").Links())
}

// The sentence names the link and today's way to bring its text into a claim.
func TestLinkDisclosureNamesTheLinkAndTheWayPast(t *testing.T) {
	assert.Equal(t, "the issue text links https://docs.google.com/d/1, which cr did not read: "+
		"a requirement stated only there is in no claim unless its text is recorded with `cr note` "+
		"and a claim is drawn from that note, per §3.3.2", LinkDisclosure("https://docs.google.com/d/1"))
}
