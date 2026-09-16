package render

import (
	"fmt"
	"slices"
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

// checked wraps content whose values cr did not write — a probe's input and
// output, a citation's path — and refuses it, naming the record, when it
// carries Reserved.
//
// An owned region is generated and never read back: AgentRegion discards
// whatever stands between its pair, and ends the region at the first closing
// marker it meets. A value carrying the sequence would end the region early
// and hand the rest to the agent's region as if the agent had written it, so
// the record is refused under §8.1.3's rejection of a body holding it — as
// ProvenanceRegion refuses a rationale that would.
func (r ownedRegion) checked(record, content string) (string, error) {
	if strings.Contains(content, Reserved) {
		return "", &BodyError{Record: record, Problem: fmt.Sprintf(
			"would carry %q in its %s region, which §8.1.3 reserves for cr's own delimiters",
			Reserved, r.name())}
	}
	return r.wrap(content), nil
}

// name is the region's own word in §8.1.3's pair — "label", "provenance",
// "evidence" — read off the opening marker, so a message naming a region and
// the delimiter it names cannot drift apart.
func (r ownedRegion) name() string {
	return strings.TrimSuffix(strings.TrimPrefix(r.open, Reserved), " -->")
}

// count is how many complete occurrences of the region text holds, found
// exactly as withoutRegion removes them and OwnedSpans locates them: each
// opening marker with the first closing marker after it.
func (r ownedRegion) count(text string) int {
	n := 0
	for from := 0; ; n++ {
		start := strings.Index(text[from:], r.open)
		if start < 0 {
			return n
		}
		start += from
		length := strings.Index(text[start:], r.close)
		if length < 0 {
			return n
		}
		from = start + length + len(r.close)
	}
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

// OwnedSpan is where one complete cr-owned region of §8.1.3 sits in a comment
// read back out of a draft, as byte offsets: Start at its opening marker, End
// just past its closing one.
type OwnedSpan struct {
	// Start is the offset of the region's opening marker.
	Start int
	// End is the offset just past the region's closing marker.
	End int
	// BeneathBody reports the §8.1.7 evidence region, the one §8.1.3's
	// sequence places beneath the agent body rather than above it.
	BeneathBody bool
}

// OwnedSpans are the complete cr-owned regions of a comment, in the order the
// comment holds them, found exactly as AgentRegion finds the ones it discards:
// by each region's own pair, the opening marker and the first closing marker
// after it. A marker without its partner is not a region and has no span.
//
// It is the reading `cr triage` edits by. §7.2.4 has `--body-file` replace a
// block's agent region the way the reviewer would, which leaves every owned
// region where it stands, so what is replaced is the text between them.
func OwnedSpans(comment string) []OwnedSpan {
	spans := make([]OwnedSpan, 0, len(ownedRegions))
	for _, region := range ownedRegions {
		for from := 0; ; {
			start := strings.Index(comment[from:], region.open)
			if start < 0 {
				break
			}
			start += from
			length := strings.Index(comment[start:], region.close)
			if length < 0 {
				break
			}
			end := start + length + len(region.close)
			spans = append(spans, OwnedSpan{Start: start, End: end, BeneathBody: region == evidenceRegion})
			from = end
		}
	}
	slices.SortFunc(spans, func(a, b OwnedSpan) int { return a.Start - b.Start })
	return spans
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

// ValidateRegions holds one block read back out of a draft to §8.1.3's
// sequence of regions, before AgentRegion recovers the agent's body from it.
//
// §8.1.3 makes a comment a fixed sequence — the label, the provenance block,
// the agent body, the evidence block — so a comment carries at most one region
// of each name, and cr generates every one of them. A second region of a name
// is therefore not a region cr rendered but a pair the body brought, and it
// has to be refused here: AgentRegion finds a region by its pair wherever it
// sits, so it would take the forged one out as cr's own, discarding the text
// the reviewer wrote inside it and handing ValidateBody a body the `<!-- cr:`
// sequence has already left. The refusal is §8.1.3's own, at the exit code
// §8.1.3 fixes.
//
// A pair the body brought where the block holds no other region of that name
// is not separable from one cr rendered under a state that has since changed —
// a question hardened per §6.3.3 leaves a label region on a record cr renders
// none for now — so this reads what the block says about itself and never what
// the record would generate today. The channel where that case is separable is
// RefuseReserved's.
func ValidateRegions(record, comment string) error {
	for _, region := range ownedRegions {
		if region.count(comment) > 1 {
			return &BodyError{Record: record, Problem: fmt.Sprintf(
				"carries a second %s region, and §8.1.3's sequence places one %s region, "+
					"which cr generates itself", region.open, region.name())}
		}
	}
	return nil
}
