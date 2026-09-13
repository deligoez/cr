package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// refuseUnknownThreads holds every record carrying `suppressed_by` to §6.1's
// row for the field: a thread id when suppressed per §3.5.4, and §3.5.4's
// thread is an ingested one.
//
// The check is membership and nothing more. Whether the thread covers the
// finding is the agent's judgement, per §3.5.3 and §3.5.4, and cr neither
// forms nor second-guesses it; what cr can establish is that the id names a
// thread it ingested. Without that, a mistyped id moves the record to
// `suppressed` just as a real one does, and a finding leaves the draft on the
// strength of a thread that does not exist.
//
// The set is threads.ndjson as it stands, the threads ingested for the pull
// request, and not a round's subset of them. §2.3.3 stamps no round on that
// file, and `cr brief` rewrites it whole from §3.5.1's ingestion of every
// thread on the pull request, resolved ones included, on each run — so the
// file already is the ingestion the current round was briefed with, and a
// thread opened in an earlier round is in it for as long as it is on the pull
// request. The file is read only when some record names a thread, lock-free
// per §2.3.2.
//
// It runs inside decodeInput, straight after the decode and before §6.4.4's
// drops shorten the slice the line numbers index.
func refuseUnknownThreads(
	l state.Layout, owner, repo string, pr int, file string, body []byte, records []*finding.Finding,
) error {
	named := false
	for _, record := range records {
		named = named || record.SuppressedBy != ""
	}
	if !named {
		return nil
	}
	threads, err := gh.ReadThreads(l, owner, repo, pr)
	if err != nil {
		return err
	}
	ingested := make(map[string]bool, len(threads))
	for i := range threads {
		ingested[threads[i].ID] = true
	}
	at := state.RecordLines(body)
	for i, record := range records {
		if record.SuppressedBy == "" || ingested[record.SuppressedBy] {
			continue
		}
		return &finding.RejectedRecordError{
			File: file, Line: at[i], Field: "suppressed_by",
			Problem: fmt.Sprintf(
				"reads %q, and no thread ingested for the pull request carries that id; §3.5.4 suppresses a "+
					"record only by an ingested thread, so name one from threads.ndjson, run cr brief if the "+
					"thread is new, or drop the field",
				record.SuppressedBy),
		}
	}
	return nil
}
