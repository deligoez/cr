package brief

import (
	"fmt"

	"github.com/deligoez/cr/internal/gh"
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
// ingested threads, in thread order and reply order.
//
// Only replies are offered. A thread the author opened is a comment on their own
// change rather than an answer to a reviewer, and §3.5.5 names replies. An
// author GitHub answered no login for — the account is gone — matches nothing,
// because an empty login would otherwise match every reply whose account is gone
// as well.
func candidateNotes(threads []gh.Thread, author, key string, pr int) []CandidateNote {
	offered := make([]CandidateNote, 0)
	if author == "" {
		return offered
	}
	if key == "" {
		key = keyPlaceholder
	}
	record := fmt.Sprintf(`cr note %s "<text>" --source thread --pr %d`, key, pr)
	for i := range threads {
		for _, reply := range threads[i].Replies {
			if reply.Author == author {
				offered = append(offered, CandidateNote{
					Thread: threads[i].ID, Reply: reply, Record: record,
				})
			}
		}
	}
	return offered
}
