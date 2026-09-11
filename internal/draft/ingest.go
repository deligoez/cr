package draft

import (
	"fmt"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// Triage is what §7.1.6's regeneration ingests from the current draft before
// rendering it again: the blocks the reviewer deleted, and the bodies they
// changed.
type Triage struct {
	// Deleted are the queued records whose block the draft no longer
	// holds, in the order the records arrived. §7.2 makes each one a
	// discard dispositioned `not-here`, with a pull-request-scoped waiver.
	Deleted []*finding.Finding
	// Preserved are the agent regions of the blocks whose body differs from
	// rendered.json, keyed by record id. The regenerated draft carries each
	// one in place of the body cr would render.
	Preserved map[string]string
}

// readBlock is one record's block as it was read back out of a draft: where
// its marker sits, and everything beneath the marker up to the next one.
type readBlock struct {
	at   int
	text string
}

// Ingest reads the current draft's triage state for the records it was
// rendered from, per §7.1.6: first the deletions, then the bodies to preserve.
//
// queued are the records the draft was rendered for — the ones §9.1 already
// holds in `queued`. A record with no block in file was deleted. A record whose
// block is there keeps the reviewer's body when its agent region differs from
// its rendered.json entry, and the comparison is byte-exact: the entry is the
// region as reading an untouched block back recovers it, per Rendered, so any
// difference at all — a changed word, a space, a line ending — is an edit, and
// P4 leaves it to the human rather than to cr to say which edits mattered. A
// record with no entry keeps its body too: with nothing to compare against, cr
// cannot show the block is untouched, and overwriting it could destroy a
// reviewer's prose where keeping it costs nothing.
//
// Only the agent region is compared or kept. render.AgentRegion discards every
// cr-owned region of the block, whatever was typed inside it, because §8.1.3
// has cr regenerate those.
//
// Everything above the first marker is §7.1.4's header, which cr regenerates
// whole, so nothing in it is read. A block naming a record outside queued is
// never adopted: it is not rendered again, which is what keeps a deleted block
// from being resurrected by pasting it back, and §7.2.3's abort on an unknown
// id is its own obligation. A malformed marker, or a second block for one
// record, stops the ingest naming the line, since there is then no single
// answer to what the reviewer wrote.
func Ingest(queued []*finding.Finding, file string, rendered map[string]string) (Triage, error) {
	blocks, err := blocksOf(file)
	if err != nil {
		return Triage{}, err
	}
	triage := Triage{Deleted: make([]*finding.Finding, 0), Preserved: make(map[string]string)}
	for _, record := range queued {
		found, held := blocks[record.ID]
		if !held {
			triage.Deleted = append(triage.Deleted, record)
			continue
		}
		region := render.AgentRegion(found.text)
		if entry, known := rendered[record.ID]; !known || region != entry {
			triage.Preserved[record.ID] = region
		}
	}
	return triage, nil
}

// blocksOf splits a draft into its blocks by record id.
//
// A line is a marker exactly when IsMarkerLine says so, and every such line is
// held to the grammar, so a reviewer's mistyped marker is refused rather than
// read as prose and the record behind it taken for deleted.
func blocksOf(file string) (map[string]readBlock, error) {
	blocks := make(map[string]readBlock)
	current := ""
	var body []string
	flush := func() {
		if current != "" {
			found := blocks[current]
			found.text = strings.Join(body, "\n")
			blocks[current] = found
		}
	}
	for i, line := range strings.Split(file, "\n") {
		if !IsMarkerLine(line) {
			body = append(body, line)
			continue
		}
		marker, err := ParseMarker(i+1, line)
		if err != nil {
			return nil, err
		}
		if earlier, twice := blocks[marker.ID]; twice {
			return nil, &MalformedMarkerError{At: i + 1, Line: line, Problem: fmt.Sprintf(
				"record %s already opens the block at line %d, and one record is one block", marker.ID, earlier.at)}
		}
		flush()
		current, body = marker.ID, nil
		blocks[current] = readBlock{at: i + 1}
	}
	flush()
	return blocks, nil
}
