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
	// RecordedAt is §3.6.1's timestamp, in UTC.
	RecordedAt time.Time `json:"recorded_at"`
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
