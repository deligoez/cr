package draft

import (
	"fmt"
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
	blocks, err := blocksOf(in.Body)
	if err != nil {
		return Triage{}, err
	}
	if err := refuseUnknownBlocks(queued, blocks); err != nil {
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
	moved, err := edit.anchor(in.Trees, in.Name)
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
// at all. v0.1 has no manual-comment channel because every record carries a
// role, an axis and a grade cr computed, and a hand-written block can carry
// none of them.
//
// It runs before the verbs rather than after, so a draft holding an unknown
// block is refused without any of its other blocks being acted on — a reviewer
// who mistyped an id would otherwise have the record they renamed discarded as
// deleted by the same run that told them about the typo.
func refuseUnknownBlocks(queued []*finding.Finding, blocks map[string]readBlock) error {
	known := make(map[string]bool, len(queued))
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
		Problem: "names no record this round rendered, and §7.2.3 gives v0.1 no manual-comment " +
			"channel in the draft; restore the id cr wrote, and write a comment of your own on " +
			"GitHub after posting"}
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
		blocks[current] = readBlock{marker: marker, at: i + 1}
	}
	flush()
	return blocks, nil
}
