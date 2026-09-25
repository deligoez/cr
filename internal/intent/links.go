package intent

import (
	"net/url"
	"regexp"
	"strings"
)

// linkPattern is what Links calls a URL: an http or https scheme and the run of
// characters after it up to whitespace, a quote, an angle bracket, a pipe, or
// a bracket of any kind. The stops are the characters trackers wrap a link in
// when they print it — Jira's `[title|url]`, Markdown's `[title](url)`, an
// HTML attribute's quotes — so the link ends where its wrapper begins.
var linkPattern = regexp.MustCompile("(?i)https?://[^\\s<>\"'`|\\[\\](){}]+")

// linkTrailer is the punctuation a sentence puts after a link, which Links
// trims from the end of each match.
const linkTrailer = ".,;:!?"

// Links returns every URL the issue text carries, in the order each first
// appears, each once.
//
// It is a scan and nothing more: no link is fetched, followed, or judged, and
// none is ranked above another. §3.1 has cr read the issue text through one
// command or one file, so a requirement stated only in a linked document is in
// no text a claim can be drawn from, and the one thing cr can establish
// mechanically is that the link is there.
func Links(issue string) []string {
	found := make([]string, 0)
	seen := make(map[string]bool)
	for _, match := range linkPattern.FindAllString(issue, -1) {
		link := strings.TrimRight(match, linkTrailer)
		if strings.HasSuffix(link, "://") || seen[link] {
			continue
		}
		seen[link] = true
		found = append(found, link)
	}
	return found
}

// LinksBesideIssue is links without the ones that name the issue itself: a
// link whose last path segment, a trailing slash trimmed, is the issue key in
// any letter case (§3.1.7). An empty key keeps every link.
//
// Jira's command-line client prints `View this issue on Jira: <url>` under
// every issue, so without this every Jira brief listed the issue's own page as
// a document cr did not read.
func LinksBesideIssue(links []string, key string) []string {
	kept := make([]string, 0, len(links))
	for _, link := range links {
		if key != "" && strings.EqualFold(lastSegment(link), key) {
			continue
		}
		kept = append(kept, link)
	}
	return kept
}

// lastSegment is the last segment of a link's path, a trailing slash trimmed,
// and empty when the link does not parse or has no path.
func lastSegment(link string) string {
	parsed, err := url.Parse(link)
	if err != nil {
		return ""
	}
	trimmed := strings.TrimRight(parsed.Path, "/")
	return trimmed[strings.LastIndex(trimmed, "/")+1:]
}

// LinkDisclosure is the honesty sentence for one link the issue text carries.
func LinkDisclosure(link string) string {
	return "the issue text links " + link + ", which cr did not read: a requirement stated only there " +
		"is in no claim unless its text is recorded with `cr note` and a claim is drawn from that note, per §3.3.2"
}
