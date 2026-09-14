package draft

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// Draft is the file §7.2's verbs are read out of, together with everything
// reading it needs from outside: what cr last rendered into it, and the trees
// §6.1.2 re-validates a moved anchor against.
//
// The four travel as one value because they are one reading. A body compared
// against the wrong rendering, or an anchor re-validated against the wrong
// head, is a wrong answer that looks like a right one, and a call site that
// passes them separately is a call site that can pair them wrongly.
type Draft struct {
	// Name is the draft's path, as the user is told it.
	Name string
	// Body is the file as the reviewer left it.
	Body string
	// Rendered is §7.1.5's rendered.json: each record's agent region as cr
	// generated it, keyed by record id.
	Rendered map[string]string
	// Trees are §6.1.2's two revisions, read only when a marker moves an
	// anchor. Nothing here opens either otherwise.
	Trees finding.Trees
	// Unchanged is §9.2.1's refusal `cr record` makes of an anchor on lines
	// the round's diff did not change, asked of a copy of the record
	// carrying the anchor a marker moved it to once that anchor resolves.
	// A *finding.RejectedRecordError it returns is the location row's
	// abort, naming the record. Nil asks nothing.
	Unchanged func(moved *finding.Finding) error
	// Posted are the ids of the round's records §9.1 already holds in
	// `posted`. Their blocks are records this round rendered, so a draft
	// still holding one is not refused as naming an unknown id, and nothing
	// is read off it: §9.1 lists no move out of `posted`.
	Posted map[string]bool
}

// Retriage is one admitted marker edit that changes what findings.ndjson holds:
// §7.2's freely editable `severity`, and the re-validated location of its
// `path`, `start_line` and `line` row.
//
// The two travel together because they are the same kind of thing and neither
// is a §7.3.1 outcome. A softening is a verb the draft keeps saying, so it is
// re-read from the file on every run; these two are values cr has accepted, and
// a value that did not reach the record would be silently reverted by the next
// rendering.
type Retriage struct {
	// Record is the queued record the edit was read against.
	Record *finding.Finding
	// Severity is what §7.2's severity row was edited to, and empty when
	// the marker left it where cr wrote it.
	Severity finding.Severity
	// Anchor is the re-validated anchor of §7.2's location row, carrying
	// §9.2's content hash recomputed over the lines it now names, and nil
	// when the marker left the location alone.
	Anchor *finding.Anchor
}

// Triage is what §7.2's verbs make of the current draft, read before it is
// rendered again per §7.1.6: the blocks the reviewer deleted, the ones they
// marked wrong, the ones they retyped, the fields they edited, and the bodies
// they changed.
//
// A record in none of the lists is kept: its block is there, its marker asks
// for nothing, and it is posted as rendered or, when its body is in Preserved,
// as edited.
type Triage struct {
	// Deleted are the queued records whose block the draft no longer
	// holds, in the order the records arrived. §7.2 makes each one a
	// discard dispositioned `not-here`, with a pull-request-scoped waiver.
	Deleted []*finding.Finding
	// Wrong are the queued records whose marker carries
	// `disposition=wrong`, in the same order. §7.2 makes each one a discard
	// as a false positive, with a repository-wide waiver, whatever its body
	// holds.
	Wrong []*finding.Finding
	// Softened are the queued findings whose marker the reviewer changed to
	// `kind=question`, in the same order.
	Softened []*finding.Finding
	// Hardened are the queued questions whose marker the reviewer changed
	// to `kind=finding`, which §7.2 admits only on a recomputed grade of
	// `probed` or `cited`.
	Hardened []*finding.Finding
	// Retriaged are the admitted edits to the two rows §7.2 lets the
	// reviewer move on the stored record, in the same order.
	Retriaged []Retriage
	// Preserved are the agent regions of the blocks whose body differs from
	// rendered.json, keyed by record id. The regenerated draft carries each
	// one in place of the body cr would render. A discarded record has
	// none, since it is not rendered again.
	Preserved map[string]string
}

// Outcome is §7.3.1's outcome action for one record the triage read, and so
// its contribution to §7.3's statistics.
//
// Hardening is deliberately absent. §7.3.1 closes the vocabulary at five action
// names, and a question the reviewer let assert is still a record they kept.
func (t *Triage) Outcome(record *finding.Finding) finding.Outcome {
	switch {
	case slices.Contains(t.Deleted, record):
		return finding.OutcomeDiscardedNotHere
	case slices.Contains(t.Wrong, record):
		return finding.OutcomeDiscardedWrong
	case slices.Contains(t.Softened, record):
		return finding.OutcomeSoftened
	}
	return finding.OutcomeKept
}

// readBlock is one record's block as it was read back out of a draft: its
// marker, where the marker sits, and everything beneath it up to the next one.
type readBlock struct {
	marker Marker
	at     int
	line   string
	text   string
}

// Ingest reads the current draft's triage state for the records it was
// rendered from: §7.2's five verbs, deletions first, then the marker edit
// semantics of its field table, then the bodies to preserve per §7.1.6.
//
// queued are the records the draft was rendered for — the ones §9.1 already
// holds in `queued`. The verbs are read in the order §7.2 lets them override
// one another. A record with no block in the file was deleted. A block whose
// marker carries `disposition=wrong` is discarded as a false positive whatever
// its body holds, so the reviewer never has to delete text to say it, and
// nothing further is read off a block that discards: the record leaves the
// round, and holding its remaining fields to the table would refuse a discard
// over a field no comment will ever carry.
//
// A kept or softened block keeps the reviewer's body when its agent region
// differs from its rendered.json entry, and the comparison is byte-exact: the
// entry is the region as reading an untouched block back recovers it, per
// Rendered, so any difference at all — a changed word, a space, a line ending —
// is an edit, and P4 leaves it to the human rather than to cr to say which
// edits mattered. A record with no entry keeps its body too: with nothing to
// compare against, cr cannot show the block is untouched, and overwriting it
// could destroy a reviewer's prose where keeping it costs nothing.
//
// Only the agent region is compared or kept. render.AgentRegion discards every
// cr-owned region of the block, whatever was typed inside it, because §8.1.3
// has cr regenerate those.
//
// Everything above the first marker is §7.1.4's header, which cr regenerates
// whole, so nothing in it is read. A block naming a record outside queued is
// never adopted, which is §7.2.3's own refusal below. A malformed marker, or a
// second block for one record, stops the ingest naming the line, since there is
// then no single answer to what the reviewer wrote.
func Ingest(queued []*finding.Finding, in *Draft) (Triage, error) {
	read, err := readBlocks(in.Body)
	if err != nil {
		return Triage{}, err
	}
	blocks, err := indexBlocks(queued, read)
	if err != nil {
		return Triage{}, err
	}
	if err := refuseUnknownBlocks(queued, in.Posted, blocks); err != nil {
		return Triage{}, err
	}
	if err := refuseMovedIDs(queued, blocks, in.Rendered); err != nil {
		return Triage{}, err
	}
	triage := Triage{
		Deleted: make([]*finding.Finding, 0), Wrong: make([]*finding.Finding, 0),
		Softened: make([]*finding.Finding, 0), Hardened: make([]*finding.Finding, 0),
		Retriaged: make([]Retriage, 0), Preserved: make(map[string]string),
	}
	for _, record := range queued {
		found, held := blocks[record.ID]
		if !held {
			triage.Deleted = append(triage.Deleted, record)
			continue
		}
		discarded, err := triage.read(record, &found, in)
		if err != nil {
			return Triage{}, err
		}
		if discarded {
			continue
		}
		region := render.AgentRegion(found.text)
		if entry, known := in.Rendered[record.ID]; !known || region != entry {
			triage.Preserved[record.ID] = region
		}
	}
	return triage, nil
}

// read holds one block's marker to §7.2's edit-semantics table and records what
// the admitted edits ask for, reporting whether the record was discarded.
//
// Every row is read before anything is recorded, so a marker asking for one
// thing §7.2 admits and one it does not is refused whole rather than half
// applied.
func (t *Triage) read(record *finding.Finding, block *readBlock, in *Draft) (discarded bool, err error) {
	edit := &markerEdit{record: record, was: markerOf(record), now: block.marker, at: block.at}
	wrong, err := edit.disposition()
	if err != nil {
		return false, err
	}
	if wrong {
		t.Wrong = append(t.Wrong, record)
		return true, nil
	}
	asked, retyped, err := edit.kind()
	if err != nil {
		return false, err
	}
	severity, err := edit.severity()
	if err != nil {
		return false, err
	}
	moved, err := edit.anchor(in)
	if err != nil {
		return false, err
	}
	if retyped {
		t.retype(record, asked)
	}
	if severity != "" || moved != nil {
		t.Retriaged = append(t.Retriaged,
			Retriage{Record: record, Severity: severity, Anchor: moved})
	}
	return false, nil
}

// retype files a record whose marker changed register under the verb §7.2 names
// for that direction.
func (t *Triage) retype(record *finding.Finding, asked finding.Kind) {
	if asked == finding.KindQuestion {
		t.Softened = append(t.Softened, record)
		return
	}
	t.Hardened = append(t.Hardened, record)
}

// refuseUnknownBlocks is §7.2's `id` row and §7.2.3 in one refusal: a block
// naming a record this draft was not rendered for aborts with exit code 1
// naming the id, rather than being adopted as a new record.
//
// The two readings reach the same block. A changed id leaves the record it was
// taken from with no block and puts a block in the file under a name the round
// does not hold, so §7.2's immutable `id` is enforced by refusing the second
// half; and a block written by hand is that same block with no record behind it
// at all. v0.2 has no manual-comment channel because every record carries a
// role, an axis and a grade cr computed, and a hand-written block can carry
// none of them.
//
// A block naming a record the round has posted is neither reading: its id is
// the one cr wrote, for a record this round rendered. A draft holds such blocks
// after `cr post --reconcile` adopts a review whose send also carried a discard,
// which stays queued for the next `cr draft` to store, so refusing them would
// leave that discard unstorable.
//
// It runs before the verbs rather than after, so a draft holding an unknown
// block is refused without any of its other blocks being acted on — a reviewer
// who mistyped an id would otherwise have the record they renamed discarded as
// deleted by the same run that told them about the typo.
func refuseUnknownBlocks(
	queued []*finding.Finding, posted map[string]bool, blocks map[string]readBlock,
) error {
	known := make(map[string]bool, len(queued)+len(posted))
	maps.Copy(known, posted)
	for _, record := range queued {
		known[record.ID] = true
	}
	// The earliest such block is reported rather than whichever one the
	// iteration reaches first: §2.1.1 has the same inputs give the same
	// result, and Go's map order is deliberately not the same twice.
	firstAt, firstID := 0, ""
	for id := range blocks {
		if at := blocks[id].at; !known[id] && (firstAt == 0 || at < firstAt) {
			firstAt, firstID = at, id
		}
	}
	if firstAt == 0 {
		return nil
	}
	return &MarkerEditError{ID: firstID, At: firstAt, Field: "id",
		Problem: "names no record this round rendered, and §7.2.3 gives v0.2 no manual-comment " +
			"channel in the draft; restore the id cr wrote, and write a comment of your own on " +
			"GitHub after posting"}
}

// Bodies is the agent region of every block a rendered draft holds, keyed by
// record id.
//
// It is what a reader after the fact needs and Ingest cannot give it: Ingest
// answers §7.2's question — what did the reviewer change — against the records
// the draft was rendered for and the rendered.json it was compared with, and a
// round that has been posted and left behind has neither in hand. §2.6.3.1's
// harvest asks the other question: what did the author actually receive. The
// draft is where that is written, because §8.1.2 makes editing it the one input
// path for reader-facing prose.
//
// Only the agent region is returned, for the reason Ingest keeps only that one:
// §8.1.3 has cr regenerate every owned region from the record, so a label or a
// provenance block is cr's wording and not a thing anybody said twice. Grouping
// on it would make two comments look alike because cr rendered the same header
// over both.
func Bodies(file string) (map[string]string, error) {
	blocks, err := blocksOf(file)
	if err != nil {
		return nil, err
	}
	bodies := make(map[string]string, len(blocks))
	for id := range blocks {
		bodies[id] = render.AgentRegion(blocks[id].text)
	}
	return bodies, nil
}

// blocksOf splits a draft into its blocks by record id, refusing a second
// block for one record as a malformed marker.
//
// It is the reading with no records in hand, which is Bodies'. Ingest has the
// records the draft was rendered for, and indexBlocks uses them to name the
// marker that was edited rather than whichever of the two came second.
func blocksOf(file string) (map[string]readBlock, error) {
	read, err := readBlocks(file)
	if err != nil {
		return nil, err
	}
	return indexBlocks(nil, read)
}

// readBlocks is every block of a draft, in the order the file holds them.
//
// A line is a marker exactly when IsMarkerLine says so, and every such line is
// held to the grammar, so a reviewer's mistyped marker is refused rather than
// read as prose and the record behind it taken for deleted.
func readBlocks(file string) ([]readBlock, error) {
	read := make([]readBlock, 0)
	var body []string
	flush := func() {
		if len(read) > 0 {
			read[len(read)-1].text = strings.Join(body, "\n")
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
		flush()
		body = nil
		read = append(read, readBlock{marker: marker, at: i + 1, line: line})
	}
	flush()
	return read, nil
}

// indexBlocks keys the draft's blocks by record id, refusing a second block for
// one record.
//
// When one of the two markers reads that record's location, severity and grade
// more closely than the other, the other is a marker whose id was changed to
// one the round already renders, and the refusal is §7.2's immutable `id`
// naming that edited marker and, when cr can tell, the id it was rendered
// under. Naming cr's untouched marker instead sent the reviewer to the wrong
// line, and deleting the block it named handed the edited one to the other
// record. Two markers reading the record alike — a pasted copy — are the
// malformed second block, named at the later line.
func indexBlocks(queued []*finding.Finding, read []readBlock) (map[string]readBlock, error) {
	blocks := make(map[string]readBlock, len(read))
	for i := range read {
		block := &read[i]
		earlier, twice := blocks[block.marker.ID]
		if !twice {
			blocks[block.marker.ID] = *block
			continue
		}
		record := recordNamed(queued, block.marker.ID)
		if record == nil || drift(&earlier.marker, record) == drift(&block.marker, record) {
			return nil, &MalformedMarkerError{At: block.at, Line: block.line, Problem: fmt.Sprintf(
				"record %s already opens the block at line %d, and one record is one block",
				block.marker.ID, earlier.at)}
		}
		edited, kept := block, &earlier
		if drift(&earlier.marker, record) > drift(&block.marker, record) {
			edited, kept = &earlier, block
		}
		return nil, &MarkerIDEditError{
			At: edited.at, ID: edited.marker.ID, Kept: kept.at,
			Restore: renderedFor(queued, read, &edited.marker),
		}
	}
	return blocks, nil
}

// refuseMovedIDs is §7.2's immutable `id` over a draft where a block is missing:
// a block still holding what cr rendered for the missing record — its location,
// severity and grade, or its body — under another record's id is that record's
// block with its id changed, not a deletion beside an edit.
//
// Read as §7.2's verbs it would be both: the missing record discarded as
// not-here with a waiver, and the other one softened, re-anchored and
// re-severitied onto the first one's line with the first one's prose — a body
// silently moved to another record's id, which is exactly what an immutable id
// exists to prevent.
func refuseMovedIDs(queued []*finding.Finding, blocks map[string]readBlock, rendered map[string]string) error {
	for _, missing := range queued {
		if _, held := blocks[missing.ID]; held {
			continue
		}
		for _, record := range queued {
			block, held := blocks[record.ID]
			if held && carries(&block, record, missing, rendered) {
				return &MarkerIDEditError{At: block.at, ID: record.ID, Restore: missing.ID}
			}
		}
	}
	return nil
}

// carries reports whether a block of own holds what cr rendered for other
// rather than for own: other's marker fields where own's are not, or other's
// rendered body where own's is not.
func carries(block *readBlock, own, other *finding.Finding, rendered map[string]string) bool {
	if drift(&block.marker, other) == 0 && drift(&block.marker, own) > 0 {
		return true
	}
	region := render.AgentRegion(block.text)
	entry, known := rendered[other.ID]
	mine, mineKnown := rendered[own.ID]
	return known && region == entry && (!mineKnown || region != mine)
}

// drift is how many of a marker's location, severity and grade fields read
// otherwise than record's own marker does, per markerOf. The kind is left
// out: a softening changes it on a block that is still its own record's.
func drift(marker *Marker, record *finding.Finding) int {
	n := 0
	for _, differs := range []bool{
		marker.Path != record.Anchor.Path,
		marker.Side != string(record.Anchor.Side),
		marker.StartLine != record.Anchor.StartLine,
		marker.Line != record.Anchor.Line,
		marker.Severity != string(record.Severity),
		marker.Grade != string(record.Grade),
	} {
		if differs {
			n++
		}
	}
	return n
}

// recordNamed is the queued record with id, and nil when none has it.
func recordNamed(queued []*finding.Finding, id string) *finding.Finding {
	for _, record := range queued {
		if record.ID == id {
			return record
		}
	}
	return nil
}

// renderedFor is the id of the queued record with no block in the draft whose
// own marker fields marker reads, which is the id an edited marker was rendered
// under, and empty when no such record is found.
func renderedFor(queued []*finding.Finding, read []readBlock, marker *Marker) string {
	for _, record := range queued {
		if record.ID == marker.ID || drift(marker, record) > 0 {
			continue
		}
		if !slices.ContainsFunc(read, func(block readBlock) bool { return block.marker.ID == record.ID }) {
			return record.ID
		}
	}
	return ""
}
