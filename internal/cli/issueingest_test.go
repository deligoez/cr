package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/text"
)

// ingestLink is the one link ingestIssue carries.
const ingestLink = "https://docs.google.com/document/d/1AbC/edit"

// ingestIssue is issue text as a tracker printed it: a U+00A0 inside the first
// requirement, a link to a document cr does not read, a second paragraph no
// claim below is drawn from, and a colour-coded footer.
const ingestIssue = fixtureIssue + ": orders over 50 TL ship free.\nSpec: " + ingestLink + "\n" +
	"\n" +
	"The campaign counts sales in two windows.\n" +
	"\x1b[38;5;242mView this issue on Jira\x1b[0m\n"

// ingestCleaned is ingestIssue as cr stores and prints it.
const ingestCleaned = fixtureIssue + ": orders over 50 TL ship free.\nSpec: " + ingestLink + "\n" +
	"\n" +
	"The campaign counts sales in two windows.\n" +
	"View this issue on Jira\n"

// ingestBrief is the part of `cr brief`'s document these tests read.
type ingestBrief struct {
	Issue struct {
		Text string `json:"text"`
	} `json:"issue"`
	Drift struct {
		Drifted bool `json:"drifted"`
		Claims  []struct {
			ID         string `json:"id"`
			SpanOccurs bool   `json:"span_occurs"`
		} `json:"claims"`
	} `json:"drift"`
	Paragraphs intent.Paragraphs `json:"issue_paragraphs"`
	Honesty    []string          `json:"honesty"`
}

// briefWith runs `cr brief` over an issue file holding body and decodes it.
func briefWith(t *testing.T, body string) ingestBrief {
	t.Helper()
	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", writeOutside(t, "issue.txt", body))
	require.NoError(t, err)
	var briefed ingestBrief
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	return briefed
}

// ingestStatus is the part of `cr status`'s document these tests read.
type ingestStatus struct {
	Paragraphs struct {
		Stored bool `json:"stored"`
		intent.Paragraphs
	} `json:"issue_paragraphs"`
}

// statusParagraphs runs `cr status` and decodes its paragraph report.
func statusParagraphs(t *testing.T) ingestStatus {
	t.Helper()
	printed, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var report ingestStatus
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report
}

// paragraphTwo is the one paragraph of ingestIssue no claim is drawn from.
var paragraphTwo = intent.Paragraph{
	StartLine: 4, EndLine: 5, Text: "The campaign counts sales in two windows.\nView this issue on Jira",
}

// Field feedback 1.1 and 1.2 through `cr brief`: the issue text is stored and
// printed with its terminal sequences and U+00A0 removed and its lines as they
// were, and every link it carries is disclosed under `honesty` as not read —
// added to exactly the sentences the same pull request's brief says without
// one, and to nothing else.
func TestBriefCleansTheIssueTextAndDisclosesItsLinks(t *testing.T) {
	rerecordHome(t)
	control := briefWith(t, rerecordIssue)
	assert.Equal(t, rerecordIssue, control.Issue.Text, "the control: a text with nothing to clean is unchanged")

	briefed := briefWith(t, ingestIssue)
	assert.Equal(t, ingestCleaned, briefed.Issue.Text)
	assert.Equal(t, append(append([]string{}, control.Honesty...), intent.LinkDisclosure(ingestLink)),
		briefed.Honesty)

	var linkLines []string
	for line := range strings.SplitSeq(throughATerminal(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", writeOutside(t, "issue.txt", ingestIssue), "--no-color"), "\n") {
		if strings.Contains(line, "links "+ingestLink) {
			linkLines = append(linkLines, line)
		}
	}
	assert.Equal(t, []string{"  " + intent.LinkDisclosure(ingestLink)}, linkLines,
		"the terminal says it once, under the issue text")
}

// v0.2.3 QA D-T1-1 through `cr brief`: a link carried only as a terminal
// hyperlink's target is disclosed as not read, and a plain link the text also
// wraps in one is disclosed once; the stored and printed text and the claims'
// issue_hash stay the cleaned text's. A text whose escapes carry no link adds
// no sentence.
func TestBriefDisclosesTheTargetsOfTerminalHyperlinks(t *testing.T) {
	_, _, _ = rerecordHome(t)
	control := briefWith(t, rerecordIssue)
	const hidden = "https://docs.test/spec"
	raw := fixtureIssue + ": orders over 50 TL ship free.\n" +
		"See \x1b]8;;" + hidden + "\x1b\\the spec\x1b]8;;\x1b\\ and \x1b]8;;" + ingestLink + "\x1b\\" + ingestLink + "\x1b]8;;\x1b\\.\n"
	cleaned := fixtureIssue + ": orders over 50 TL ship free.\nSee the spec and " + ingestLink + ".\n"

	briefed := briefWith(t, raw)
	assert.Equal(t, cleaned, briefed.Issue.Text)
	assert.Equal(t, append(append([]string{}, control.Honesty...),
		intent.LinkDisclosure(hidden), intent.LinkDisclosure(ingestLink)), briefed.Honesty)

	claims := writeOutside(t, "claims.ndjson",
		`{"id":"`+fixtureIssue+`#c1","text":"Free shipping over 50 TL.","source":"description","span":"orders over 50 TL ship free"}`+"\n")
	out, err := runCLIPrinting(t, "claims", "record", fixturePR, claims, "--repo", fixtureSlug,
		"--intent-file", writeOutside(t, "issue.txt", raw))
	require.NoError(t, err)
	var recorded struct {
		Recorded []intent.Claim `json:"recorded"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &recorded))
	cleanedHash, err := text.NormalisedHash(cleaned)
	require.NoError(t, err)
	require.Len(t, recorded.Recorded, 1)
	assert.Equal(t, cleanedHash, recorded.Recorded[0].IssueHash)

	unlinked := briefWith(t, "\x1b]0;"+fixtureIssue+"\x07"+rerecordIssue+"\x1b[2mView on Jira\x1b[0m\n")
	assert.Equal(t, control.Honesty, unlinked.Honesty)
}

// §3.1.7 through `cr brief`: a link whose last path segment is the issue key
// names the issue itself and is not listed as linked and not read, whatever
// its letter case or trailing slash, while a link beside it still is.
//
// Measured on tarfin-labs/backend#6328 with cr 0.13.0: jira-cli prints `View
// this issue on Jira: https://tarfin.atlassian.net/browse/WB-3295` under every
// issue, and the brief listed that URL as a document cr did not read.
func TestBriefDoesNotListTheIssuesOwnLink(t *testing.T) {
	rerecordHome(t)
	control := briefWith(t, rerecordIssue)
	own := "https://tarfin.atlassian.net/browse/" + fixtureIssue
	lowered := "https://tarfin.atlassian.net/browse/" + strings.ToLower(fixtureIssue) + "/"
	briefed := briefWith(t, rerecordIssue+"Spec: "+ingestLink+"\nView this issue on Jira: "+own+
		"\nAlso: "+lowered+"\n")
	assert.Equal(t, append(append([]string{}, control.Honesty...), intent.LinkDisclosure(ingestLink)),
		briefed.Honesty)
}

// Field feedback 1.2 through `cr claims record`: a span is checked verbatim
// against the cleaned text, so the span as `cr brief` printed it is recorded
// and one copied with the U+00A0 is refused with exit code 1; the claim's
// issue_hash is the cleaned text's.
func TestClaimsRecordChecksSpansAgainstTheCleanedText(t *testing.T) {
	_, _, _ = rerecordHome(t)
	issue := writeOutside(t, "issue.txt", ingestIssue)
	copied := writeOutside(t, "copied.ndjson",
		`{"id":"`+fixtureIssue+`#c1","text":"Free shipping over 50 TL.","source":"description","span":"orders over 50`+" "+`TL ship free"}`+"\n")
	_, err := runCLIPrinting(t, "claims", "record", fixturePR, copied, "--repo", fixtureSlug, "--intent-file", issue)
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))

	printed := writeOutside(t, "printed.ndjson",
		`{"id":"`+fixtureIssue+`#c1","text":"Free shipping over 50 TL.","source":"description","span":"orders over 50 TL ship free"}`+"\n")
	out, err := runCLIPrinting(t, "claims", "record", fixturePR, printed, "--repo", fixtureSlug, "--intent-file", issue)
	require.NoError(t, err)
	var recorded struct {
		Recorded []intent.Claim `json:"recorded"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &recorded))
	cleanedHash, err := text.NormalisedHash(ingestCleaned)
	require.NoError(t, err)
	require.Len(t, recorded.Recorded, 1)
	assert.Equal(t, cleanedHash, recorded.Recorded[0].IssueHash)
}

// Field feedback 2.9 through `cr brief` and `cr status`: both list the issue
// paragraphs no claim span overlaps, whole and unranked; before any claim every
// paragraph is listed, and recording a claim takes out exactly the paragraph it
// was drawn from.
func TestBriefAndStatusListTheParagraphsNoClaimSpanCovers(t *testing.T) {
	rerecordHome(t)
	first := intent.Paragraph{StartLine: 1, EndLine: 2, Text: fixtureIssue + ": orders over 50 TL ship free.\nSpec: " + ingestLink}
	briefed := briefWith(t, ingestIssue)
	assert.Equal(t, intent.Paragraphs{Total: 2, Uncovered: []intent.Paragraph{first, paragraphTwo}}, briefed.Paragraphs)
	unclaimed := statusParagraphs(t)
	assert.True(t, unclaimed.Paragraphs.Stored)
	assert.Equal(t, briefed.Paragraphs, unclaimed.Paragraphs.Paragraphs, "status reads the text the brief stored")

	// The claims are recorded against a text with a third paragraph, which
	// `cr status` then reports from: the text a round's claims were checked
	// against, not the one the brief before them read.
	issue := writeOutside(t, "issue.txt", ingestIssue+"\nRefunds are excluded.\n")
	claims := writeOutside(t, "claims.ndjson",
		`{"id":"`+fixtureIssue+`#c1","text":"Free shipping over 50 TL.","source":"description","span":"orders over 50 TL ship free"}`+"\n")
	_, err := runCLIPrinting(t, "claims", "record", fixturePR, claims, "--repo", fixtureSlug, "--intent-file", issue)
	require.NoError(t, err)
	assert.Equal(t, intent.Paragraphs{Total: 3, Uncovered: []intent.Paragraph{
		paragraphTwo, {StartLine: 7, EndLine: 7, Text: "Refunds are excluded."},
	}}, statusParagraphs(t).Paragraphs.Paragraphs)

	want := intent.Paragraphs{Total: 2, Uncovered: []intent.Paragraph{paragraphTwo}}
	assert.Equal(t, want, briefWith(t, ingestIssue).Paragraphs)
	assert.Equal(t, want, statusParagraphs(t).Paragraphs.Paragraphs, "and the brief after them stores its own")

	var lines []string
	collecting := false
	for line := range strings.SplitSeq(throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color"), "\n") {
		if strings.HasPrefix(line, "issue paragraphs: ") {
			collecting = true
		} else if collecting && !strings.HasPrefix(line, "  ") {
			collecting = false
		}
		if collecting {
			lines = append(lines, line)
		}
	}
	assert.Equal(t, []string{
		"issue paragraphs: 2 total, 1 overlapped by no claim span",
		"  lines 4-5",
		"    | The campaign counts sales in two windows.",
		"    | View this issue on Jira",
	}, lines)
}

// A round whose issue text no command stored says so rather than reporting
// every paragraph covered.
func TestStatusSaysWhenTheRoundStoresNoIssueText(t *testing.T) {
	layout, _, _ := rerecordHome(t)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Remove(state.FileIssueText))
	require.NoError(t, held.Unlock())

	report := statusParagraphs(t)
	assert.False(t, report.Paragraphs.Stored)
	assert.Equal(t, intent.Paragraphs{Uncovered: []intent.Paragraph{}}, report.Paragraphs.Paragraphs)
	var said []string
	for line := range strings.SplitSeq(throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color"), "\n") {
		if strings.HasPrefix(line, "issue paragraphs: ") {
			said = append(said, line)
		}
	}
	assert.Equal(t, []string{"issue paragraphs: no issue text stored for round 1; " +
		"`cr brief` and `cr claims record` store the text they read"}, said)
}

// Field feedback 1.2's verification, through `cr brief`: a round recorded
// before cleaning — its claim's issue_hash taken over the bytes as printed and
// its span holding the U+00A0 — reports no drift and a span that still occurs
// when the issue has not changed, and still reports both when it has.
func TestBriefReportsNoFalseDriftOnARoundRecordedBeforeCleaning(t *testing.T) {
	layout, _, _ := rerecordHome(t)
	briefWith(t, ingestIssue)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	asPrinted, err := text.NormalisedHash(ingestIssue)
	require.NoError(t, err)
	span := "orders over 50 TL ship free"
	spanHash, err := text.NormalisedHash(span)
	require.NoError(t, err)
	line, err := json.Marshal(map[string]any{
		"id": fixtureIssue + "#c1", "text": "Free shipping over 50 TL.", "source": "description",
		"span": span, "span_hash": spanHash, "issue_hash": asPrinted, "head": meta.Head, "round": meta.Round,
	})
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileClaims, append(line, '\n')))
	require.NoError(t, held.StampClaims(meta.Round, meta.Head))
	require.NoError(t, held.Unlock())

	unchanged := briefWith(t, ingestIssue)
	assert.False(t, unchanged.Drift.Drifted, "the issue did not change; cr began cleaning it")
	require.Len(t, unchanged.Drift.Claims, 1)
	assert.True(t, unchanged.Drift.Claims[0].SpanOccurs)
	assert.Equal(t, intent.Paragraphs{Total: 2, Uncovered: []intent.Paragraph{paragraphTwo}}, unchanged.Paragraphs,
		"the span recorded with its U+00A0 covers the paragraph it was drawn from")

	moved := briefWith(t, strings.Replace(ingestIssue, "50 TL", "75 TL", 1))
	assert.True(t, moved.Drift.Drifted, "a changed issue still drifts")
	require.Len(t, moved.Drift.Claims, 1)
	assert.False(t, moved.Drift.Claims[0].SpanOccurs)
}
