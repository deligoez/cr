package finding

import "github.com/deligoez/cr/internal/git"

// DuplicateLink is one way two merged records can point at the same code
// without sharing §6.4.1's key.
//
// There are exactly two, and they are the two field-feedback 2.4 measured on
// all four real cross-unit duplicate pairs of tarfin-labs/backend#6233. A link
// is a location both records name, never a comparison of what they say: which
// of two records about one line is the better comment is a judgement, and cr
// forms none.
type DuplicateLink string

const (
	// LinkSharedCitation is two records citing the same path and line.
	LinkSharedCitation DuplicateLink = "shared-citation"
	// LinkCitesAnchor is one record citing a line inside the other record's
	// `anchor.start_line`..`anchor.line` range, on the same path. A citation
	// numbers a line of the head (§6.2.3), so only a RIGHT anchor can hold
	// one; a LEFT anchor numbers the merge base, and the same number there
	// is a different line.
	LinkCitesAnchor DuplicateLink = "cites-anchor"
)

// PossibleDuplicate is one pair of merged records that a link joins, listed for
// the human to compare.
//
// It is a report and nothing more. Neither record is dropped, marked or
// altered: §6.4.1 is the one dedup rule, and a pair that escapes it is left for
// the reader of the draft to settle, because two records sharing a location
// can still be about two different defects.
type PossibleDuplicate struct {
	// Records are the two record ids, in the order they arrived in the
	// merge, which is the order the merged file holds them in.
	Records []string `json:"records"`
	// Links are the kinds that join them, shared-citation before
	// cites-anchor, each named once.
	Links []DuplicateLink `json:"links"`
}

// PossibleDuplicates lists every pair of records a link joins, over the records
// one merge writes.
//
// A record §6.4.3 already suppressed — one carrying `duplicate_of` — takes part
// in no pair: §6.4.1 has already deduplicated it, it never reaches the draft,
// and a pair naming it would point the human at a comment nobody will read.
//
// The pairs come back in arrival order, first by the earlier record and then
// by the later one, so one input lists its pairs one way on every run. A merge
// with no pair gets an empty list, never nil.
func PossibleDuplicates(records []*Finding) []PossibleDuplicate {
	listed := make([]*Finding, 0, len(records))
	for _, record := range records {
		if record.DuplicateOf == "" {
			listed = append(listed, record)
		}
	}
	pairs := make([]PossibleDuplicate, 0)
	for at, first := range listed {
		for _, second := range listed[at+1:] {
			links := linksBetween(first, second)
			if len(links) == 0 {
				continue
			}
			pairs = append(pairs, PossibleDuplicate{
				Records: []string{first.ID, second.ID},
				Links:   links,
			})
		}
	}
	return pairs
}

// linksBetween is every link that joins two records, in DuplicateLink's order.
func linksBetween(first, second *Finding) []DuplicateLink {
	links := make([]DuplicateLink, 0, 2)
	if shareACitation(first, second) {
		links = append(links, LinkSharedCitation)
	}
	if citesInside(first, second) || citesInside(second, first) {
		links = append(links, LinkCitesAnchor)
	}
	return links
}

// shareACitation reports whether the two records cite one path and line.
func shareACitation(first, second *Finding) bool {
	for _, cited := range first.Citations {
		for _, alsoCited := range second.Citations {
			if cited.Path == alsoCited.Path && cited.Line == alsoCited.Line {
				return true
			}
		}
	}
	return false
}

// citesInside reports whether record cites a line inside other's RIGHT anchor
// range on other's path.
func citesInside(record, other *Finding) bool {
	if other.Anchor.Side != git.Right {
		return false
	}
	for _, cited := range record.Citations {
		if cited.Path == other.Anchor.Path &&
			other.Anchor.StartLine <= cited.Line && cited.Line <= other.Anchor.Line {
			return true
		}
	}
	return false
}
