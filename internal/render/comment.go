package render

import (
	"fmt"
	"strings"
)

// Reserved is the sequence §8.1.3 keeps out of every agent body: the opening
// of §7.1.1's record marker and of every cr-owned region below. A body holding
// it is refused rather than escaped, because inside a draft there is no way to
// tell text that looks like a delimiter from a delimiter.
const Reserved = "<!-- cr:"

// The named pairs §8.1.3 gives the label and provenance regions. The evidence
// pair lives in evidence.go beside the region it delimits; all three are
// gathered in ownedRegions below.
const (
	labelOpen       = Reserved + "label -->"
	labelClose      = Reserved + "/label -->"
	provenanceOpen  = Reserved + "provenance -->"
	provenanceClose = Reserved + "/provenance -->"
)

// ownedRegion is one cr-owned region of §8.1.3, known by its own named pair.
//
// A pair rather than a position: §8.1.3 has recovery "never depend on
// counting", so a region is found by its opening and closing marker wherever
// it sits, and a comment missing one region is read exactly as one holding it.
type ownedRegion struct {
	open, close string
}

// The three owned regions of §8.1.3, each by its pair.
var (
	labelRegion      = ownedRegion{open: labelOpen, close: labelClose}
	provenanceRegion = ownedRegion{open: provenanceOpen, close: provenanceClose}
	evidenceRegion   = ownedRegion{open: evidenceOpen, close: evidenceClose}
)

// ownedRegions are §8.1.3's three pairs, in the order the sequence places
// them: the §8.1.4 label, the §8.1.6 provenance block, and — beneath the agent
// body — the §8.1.7 evidence block.
var ownedRegions = []ownedRegion{labelRegion, provenanceRegion, evidenceRegion}

// wrap delimits content by the region's pair, each marker on a line of its
// own, which is the shape ProbeEvidence already gives the evidence region.
func (r ownedRegion) wrap(content string) string {
	return r.open + "\n" + strings.Trim(content, "\n") + "\n" + r.close
}

// regionSeparator is what stands between two regions of a comment: a blank
// line, so each region is a Markdown block of its own and an HTML comment
// marker never runs into the prose beside it.
const regionSeparator = "\n\n"

// Comment is one comment as §8.1.3's fixed sequence of regions.
//
// The three cr-owned fields hold a whole delimited region — the text a region
// constructor returns, markers included — or the empty string when §8.1.4,
// §8.1.6 or §8.1.7 has none to place. Body is the agent's region, which is the
// only one the agent writes and the only one §8.1.5, §7.1.5 and §7.1.6 read.
type Comment struct {
	// Label is the §8.1.4 label region, present on a question alone.
	Label string
	// Provenance is the §8.1.6 provenance region, present when the record's
	// origin is weak.
	Provenance string
	// Body is the agent region.
	Body string
	// Evidence is the §8.1.7 evidence region, present when the grade
	// asserts.
	Evidence string
}

// String is the comment as it is written into the draft and posted: the
// regions that apply, in §8.1.3's order, a blank line between each two.
//
// The order is fixed here rather than by the caller, so no caller can place the
// evidence above the body or the label beneath it.
func (c *Comment) String() string {
	regions := make([]string, 0, len(ownedRegions)+1)
	for _, region := range []string{c.Label, c.Provenance, c.Body, c.Evidence} {
		if region != "" {
			regions = append(regions, region)
		}
	}
	return strings.Join(regions, regionSeparator)
}

// AgentRegion recovers the agent's region from a comment as it was read back
// out of a draft, discarding every cr-owned region in it.
//
// This is how §8.1.3's "cr regenerates every owned region and discards edits
// inside them" is carried out: an owned region is removed whole, between its
// own opening and closing marker, whatever was typed inside it, and the caller
// regenerates it from the record. What is left is the agent's, and only that is
// held to §8.1.5 or compared against `rendered.json`.
//
// A region is found by its pair and never by counting, so a comment the agent
// rearranged still recovers. A marker without its partner is not a region, and
// it is left where it stands: the agent region then carries Reserved, and
// ValidateBody refuses it naming the record, rather than cr guessing where a
// half-deleted region was meant to end.
//
// The separator each removed region stood behind goes with it, so a comment
// that nobody edited recovers exactly the Body it was built from.
func AgentRegion(comment string) string {
	for _, region := range ownedRegions {
		comment = withoutRegion(comment, region)
	}
	return strings.Trim(comment, "\n")
}

// withoutRegion removes every complete occurrence of region from text, each
// together with the separator that follows it.
func withoutRegion(text string, region ownedRegion) string {
	for {
		start := strings.Index(text, region.open)
		if start < 0 {
			return text
		}
		length := strings.Index(text[start:], region.close)
		if length < 0 {
			return text
		}
		rest := strings.TrimPrefix(text[start+length+len(region.close):], regionSeparator)
		text = text[:start] + rest
	}
}

// BodyError refuses an agent region that §8.1 does not let reach the author.
//
// It names the record, because the fix is an edit to one block of `draft.md`
// and the record id is how the reviewer finds it. The cli layer maps it onto
// exit code 1, which §8.1.3 fixes.
type BodyError struct {
	// Record is the id of the record whose body is refused.
	Record string
	// Problem completes the sentence naming what is wrong with it.
	Problem string
}

func (e *BodyError) Error() string {
	return fmt.Sprintf("the body of record %s %s", e.Record, e.Problem)
}

// ValidateBody holds one agent region to §8.1.3: it is non-empty, and it
// carries no Reserved sequence.
//
// A body of nothing but whitespace is empty. It would post as a comment with
// nothing in it, which is what the requirement is there to stop.
//
// Reserved covers the marker comment and every owned delimiter at once. A body
// holding a record marker would split its block in two when the draft is read
// back, and one holding a region delimiter would have a slice of the agent's
// own prose discarded as cr's — so both are refused before either can happen.
func ValidateBody(record, body string) error {
	if strings.TrimSpace(body) == "" {
		return &BodyError{Record: record, Problem: "is empty, and §8.1.3 requires every body to be non-empty"}
	}
	if strings.Contains(body, Reserved) {
		return &BodyError{Record: record, Problem: fmt.Sprintf(
			"contains %q, which §8.1.3 reserves for the record marker and cr's own regions", Reserved)}
	}
	return nil
}
