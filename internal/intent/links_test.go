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

// The sentence names the link and today's way to bring its text into a claim.
func TestLinkDisclosureNamesTheLinkAndTheWayPast(t *testing.T) {
	assert.Equal(t, "the issue text links https://docs.google.com/d/1, which cr did not read: "+
		"a requirement stated only there is in no claim unless its text is recorded with `cr note` "+
		"and a claim is drawn from that note, per §3.3.2", LinkDisclosure("https://docs.google.com/d/1"))
}
