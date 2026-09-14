package brief

import (
	"fmt"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/text"
)

// CandidateNote is §3.5.5's offer: one reply the pull request's author wrote
// inside an ingested thread, put in front of the agent as a possible context
// note per §3.6.
//
// It is offered and never written. A reply is the author's word in a thread, and
// whether it states a fact about the issue that the tracker lacks is a judgement
// §3.6 leaves to a human, so cr names the reply and the command that would
// record it and stops there: the context store changes only when someone runs
// `cr note` with `--source thread`.
type CandidateNote struct {
	// Thread is the id of the ingested thread the reply sits in.
	Thread string `json:"thread"`
	// Reply is the author's comment, as §3.5.1 ingested it.
	Reply gh.Comment `json:"reply"`
	// Record is the command that would record it, with the note text left
	// for whoever decides the reply is worth keeping.
	Record string `json:"record"`
}

// keyPlaceholder stands in for the issue key in Record when §3.2 resolved none:
// §3.6.1 files a note under a key, so the command cannot be run until one is
// given, and saying so is truer than printing a command that would fail.
const keyPlaceholder = "<ISSUE-KEY>"

// candidateNotes is every reply the pull request's author wrote inside the
// ingested threads, in thread order and reply order, that the issue key's
// context store does not already hold.
//
// Only replies are offered. A thread the author opened is a comment on their own
// change rather than an answer to a reviewer, and §3.5.5 names replies. An
// author GitHub answered no login for — the account is gone — matches nothing,
// because an empty login would otherwise match every reply whose account is gone
// as well.
//
// A reply whose body is, under §1.4's normalisation, the text of a note stored
// for the key — by `cr answer` or `cr note`, and whether or not §3.6.6 has
// since retracted it — is not offered: the offer is an invitation to store the
// reply, and one already stored would be stored twice, or a withdrawn fact
// stored again. stored is the key's whole store.
func candidateNotes(threads []gh.Thread, author, key string, pr int, stored []note.Note) []CandidateNote {
	offered := make([]CandidateNote, 0)
	if author == "" {
		return offered
	}
	if key == "" {
		key = keyPlaceholder
	}
	recorded := make(map[string]bool, len(stored))
	for i := range stored {
		recorded[normalised(stored[i].Text)] = true
	}
	record := fmt.Sprintf(`cr note %s "<text>" --source thread --pr %d`, key, pr)
	for i := range threads {
		for _, reply := range threads[i].Replies {
			if reply.Author == author && !recorded[normalised(reply.Body)] {
				offered = append(offered, CandidateNote{
					Thread: threads[i].ID, Reply: reply, Record: record,
				})
			}
		}
	}
	return offered
}

// normalised is §1.4's normalisation of a reply body or a note text, and the
// text as given when it is not valid UTF-8: such a text equals no normalised
// one, so the reply stays offered.
func normalised(in string) string {
	out, err := text.Normalise(in)
	if err != nil {
		return in
	}
	return out
}
