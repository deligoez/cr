// Package note holds the out-of-band context store of spec/0.1.0.md §3.6.
//
// A note is a fact that is true about the issue but absent from the tracker.
// §3.6.6 calls it unverified hearsay, and everything downstream is shaped by
// that: §3.3.2 admits a note as a claim only under `source: note` with the
// note's id, and §8.1.6 makes cr disclose that provenance in the posted body
// rather than only in the draft the author never sees. So a note is stored
// with where it came from attached, and nothing here can produce one without.
package note

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Source names where a note came from.
type Source string

// The sources of §3.6.3, which closes the set.
const (
	// SourceChat is a conversation in a chat client.
	SourceChat Source = "chat"
	// SourceJira is the tracker itself, outside the issue text cr reads.
	SourceJira Source = "jira"
	// SourceThread is a review thread on a pull request.
	SourceThread Source = "thread"
	// SourceMeeting is something said in a meeting.
	SourceMeeting Source = "meeting"
	// SourceOther is everything else, named so a fact is never dropped for
	// want of a label.
	SourceOther Source = "other"
)

// sources is §3.6.3's set in the order it names them.
var sources = []Source{SourceChat, SourceJira, SourceThread, SourceMeeting, SourceOther}

// Sources returns §3.6.3's set in the order it names them. The result is a
// copy, so a caller can neither widen the set nor reorder it.
func Sources() []Source {
	return append(make([]Source, 0, len(sources)), sources...)
}

// InvalidSourceError reports a `--source` §3.6.3 does not admit, an absent one
// included.
//
// §3.6.1 shows the flag in its synopsis and never says it is required, and cr
// requires it: a note whose provenance is unknown cannot carry the disclosure
// §8.1.6 obliges it to reach the reader with, so there is no note to record.
// Both faults are the invocation rather than the data in a file, which is why
// they share one error and why internal/cli maps neither — §11.2's code 2 is
// what an unmapped error already means there.
type InvalidSourceError struct {
	// Value is what the flag carried, empty when it carried nothing.
	Value string
}

func (e *InvalidSourceError) Error() string {
	admitted := make([]string, 0, len(sources))
	for _, source := range sources {
		admitted = append(admitted, string(source))
	}
	if e.Value == "" {
		return "--source is required: §3.6.3 admits " + strings.Join(admitted, ", ")
	}
	return fmt.Sprintf("--source %q: §3.6.3 admits %s", e.Value, strings.Join(admitted, ", "))
}

// ParseSource returns the §3.6.3 source value names.
func ParseSource(value string) (Source, error) {
	if !slices.Contains(sources, Source(value)) {
		return "", &InvalidSourceError{Value: value}
	}
	return Source(value), nil
}

// Note is one out-of-band fact recorded against an issue key (§3.6.1). One
// line of ~/.cr/context/<ISSUE-KEY>.ndjson holds one of these.
type Note struct {
	// ID is the `<ISSUE-KEY>#n<n>` of §3.6.1, allocated by NextID.
	ID string `json:"id"`
	// Text is the fact, as the human wrote it. §3.3.2 makes it the span of
	// any claim drawn from this note, so it is stored verbatim.
	Text string `json:"text"`
	// Source is where the fact came from (§3.6.3). §8.1.6 names it in the
	// posted body of every record resting on this note.
	Source Source `json:"source"`
	// PR is the pull request the note came from. It is provenance and not a
	// scope: §3.6.4 has the note load for every later pull request that
	// resolves to the same issue key.
	PR int `json:"pr"`
	// Record is the record this note answers (§3.6.2), spelled as §6.1
	// spells one, and empty on a note that answers none. It is read
	// together with PR above rather than alone: §11 scopes a record id to
	// its pull request, so the pair is the reference and the id by itself
	// is not.
	Record string `json:"record,omitempty"`
	// RecordedAt is §3.6.1's timestamp, in UTC.
	RecordedAt time.Time `json:"recorded_at"`
	// RetractedAt is when §3.6.6 retracted the note, in UTC, and nil while
	// the note still stands. Retracting marks the note rather than erasing
	// it: §3.3.2 lets a claim cite a note by id and §8.1.6 discloses that
	// provenance in a posted body, so a deleted line would leave those
	// citations pointing at nothing and would free the id for the next
	// note to take.
	RetractedAt *time.Time `json:"retracted_at,omitempty"`
}

// Retracted reports whether §3.6.6 has retracted the note.
func (n *Note) Retracted() bool { return n.RetractedAt != nil }

// Standing reports what a citation of this note may still do.
//
// A note can only ever report the two standings it can be in itself. An id the
// store holds no note for is StandingDangling, and only StandingOf can see
// that, because only it is given the store.
func (n *Note) Standing() Standing {
	if n.Retracted() {
		return StandingRetracted
	}
	return StandingStands
}

// Standing is what a record or coverage cell citing a note may still do
// (§3.6.6).
//
// It is derived from the store every time it is asked for and is never copied
// onto the citing record. That is the whole of the round-9 finding
// retracted-provenance-still-posts: a retraction has to bite inside the round
// it happens in, and §9.3.5 exempts this store from round scoping outright, so
// there is no snapshot for a retraction to arrive too late for. `cr draft` and
// `cr post` read the standing as it is at the moment they run. Deferring the
// consequence to the next round would not be a delay but a loss — v0.1 ends at
// posting per §9, so for this pull request the next round may never arrive.
//
// Every consumer asks this rather than deriving it again. §6.3's register and
// §8.1.6's provenance region turn on one fact, and two derivations of it are
// two chances to disagree about whether an assertion may go out.
type Standing string

const (
	// StandingStands is a note the store holds and nobody has retracted.
	// It is the only standing that carries provenance, which makes it the
	// only one §8.1.6 has a region to emit for and the only one a record
	// may take the assertion register from.
	StandingStands Standing = "stands"
	// StandingRetracted is a note §3.6.6 has retracted. The citing record
	// is retained rather than deleted, so the decision stays auditable,
	// and is reported as needing re-evaluation rather than silently kept.
	StandingRetracted Standing = "retracted"
	// StandingDangling is an id no note in the store bears. It is not a
	// standing citation for want of a retraction: a record citing it has
	// exactly as little provenance to disclose as one citing a retracted
	// note, so §8.1.6 emits no region for it either.
	StandingDangling Standing = "dangling"
)

// Stands reports whether a citation of this standing may still be built on.
//
// One predicate and not two, because §3.6.6's two consequences have one cause:
// a record whose note no longer stands has no provenance left to disclose, so
// it neither qualifies for the assertion register nor gives §8.1.6 a region to
// emit.
func (s Standing) Stands() bool { return s == StandingStands }

// StandingOf reports the standing of the note noteID names.
//
// notes MUST be every note the store holds, for the reason NextID's argument
// must be: an id absent from the slice is reported as dangling, so a slice
// narrowed by round or by pull request would retract notes nobody retracted.
func StandingOf(notes []Note, noteID string) Standing {
	at := indexOf(notes, noteID)
	if at < 0 {
		return StandingDangling
	}
	return notes[at].Standing()
}

// indexOf is the position of the note bearing id, or -1 when the store holds
// none.
func indexOf(notes []Note, id string) int {
	return slices.IndexFunc(notes, func(n Note) bool { return n.ID == id })
}

// Find returns the note bearing id, and whether the store holds one.
//
// It is StandingOf's other half: a citation of a note needs the standing to
// decide what may be asserted on it and the note itself to read what it says,
// and §3.3.2's claim needs the second — the span of a note-sourced claim is
// that note's body, so validating one means reading it.
//
// notes MUST be every note the store holds, for the reason StandingOf's must:
// a slice narrowed by round or by pull request would report a note somebody
// recorded as one the store does not hold. The returned pointer is into notes,
// so a caller reads the store's own record rather than a copy that could drift
// from it.
func Find(notes []Note, id string) (*Note, bool) {
	at := indexOf(notes, id)
	if at < 0 {
		return nil, false
	}
	return &notes[at], true
}

// UnknownNoteError reports a `--remove` naming a note the store does not hold.
//
// The id is spelled the way §3.6.1 spells one and the store read and parsed
// without trouble, so neither the invocation nor the file is what is wrong.
// What fails is the retraction itself: §3.6.6 has no note to retract. §11.2
// codes that 1, alongside NoIssueKeyError, rather than the 2 a mistyped command
// line gets.
type UnknownNoteError struct {
	// ID is the note id that named nothing.
	ID string
	// IssueKey is the store that was searched, which is the store the id
	// itself named per §3.6.1.
	IssueKey string
}

func (e *UnknownNoteError) Error() string {
	return fmt.Sprintf(
		"no note %s is recorded against %s: run `cr context %s` for the ids the store holds",
		e.ID, e.IssueKey, e.IssueKey,
	)
}

// idInfix separates an id's issue key from its number, per §3.6.1's
// `<ISSUE-KEY>#n<n>`.
const idInfix = "#n"

// NextID allocates the id for a new note against issueKey.
//
// existing MUST be every note the store holds. §3.3.2 lets a claim cite a note
// by id and §3.6.6 has a retraction reach every record citing one, so an id is
// spent for the life of the issue: reusing one would point a claim at a fact
// nobody recorded under it. A retracted note therefore keeps its number rather
// than freeing it, which is why nothing here filters the input.
func NextID(issueKey string, existing []Note) string {
	highest := 0
	// Indexed rather than ranged by value: only the id is read here.
	for i := range existing {
		// `>=` would behave identically — it would assign the value
		// already held — so no test can tell the two apart.
		if n, ok := parseID(issueKey, existing[i].ID); ok && n > highest {
			highest = n
		}
	}
	return issueKey + idInfix + strconv.Itoa(highest+1)
}

// SplitID reads the issue key out of a note id.
//
// §3.6.1 forms every id as `<ISSUE-KEY>#n<n>`, so an id already names the store
// it belongs to. That is why §11's row is `cr note --remove <note-id>` and
// takes no issue key: there is no second place for the two to disagree, and no
// way to retract a note out of a store it was never in.
//
// The split is at the *last* infix, because §2.2 only forbids an issue key to
// hold a path separator — one may contain `#n` — while the number after the
// last one is fixed. What comes out is then checked by parseID, so only the
// canonical spelling splits at all.
func SplitID(id string) (issueKey string, ok bool) {
	// Both faults at once: -1 is an id with no infix, and 0 is one whose
	// key is empty, which §2.2 has no file for.
	at := strings.LastIndex(id, idInfix)
	if at < 1 {
		return "", false
	}
	issueKey = id[:at]
	if _, canonical := parseID(issueKey, id); !canonical {
		return "", false
	}
	return issueKey, true
}

// parseID reads the n of a `<issueKey>#n<n>` id.
//
// It accepts only the canonical spelling: CR-1#n7 is an id, CR-1#n+7, CR-1#n07
// and CR-1#n-7 are not, and neither is CR-1#n0, because notes are numbered from
// one. An id under another issue key is another store's, and an id cr did not
// write is no evidence about what is taken, so neither contributes to the next
// allocation rather than being read as some number near it.
func parseID(issueKey, id string) (int, bool) {
	rest, found := strings.CutPrefix(id, issueKey+idInfix)
	if !found {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || rest != strconv.Itoa(n) || n < 1 {
		return 0, false
	}
	return n, true
}
