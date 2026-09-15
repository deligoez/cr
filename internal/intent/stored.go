package intent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"

	"github.com/deligoez/cr/internal/state"
)

// storedIssue is state.FileIssueText's document: the issue text one round last
// read, cleaned, with the round and head it was read at.
type storedIssue struct {
	state.Stamp
	Text string `json:"text"`
}

// StoreIssueText records the issue text a command read for a round, under the
// lock that command holds for its other writes to the round.
//
// `cr status` reads it back rather than running the tracker command, which it
// has no `--intent-file` to bypass: a report of which paragraphs no claim span
// covers is a report about the text the round's claims were checked against,
// and the text as a later tracker run returns it may be another.
func StoreIssueText(held *state.Lock, at state.Stamp, issue string) error {
	body, err := json.MarshalIndent(storedIssue{Stamp: at, Text: issue}, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode %s: %w", state.FileIssueText, err)
	}
	return held.Write(state.FileIssueText, append(body, '\n'))
}

// StoredIssueText returns the issue text last stored for round, and false when
// none is: no file, or one written for another round. A round whose text was
// stored by an earlier round only is a round nobody has read the issue for.
func StoredIssueText(l state.Layout, owner, repo string, pr, round int) (issue string, stored bool, err error) {
	body, err := l.ReadPR(owner, repo, pr, state.FileIssueText)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var document storedIssue
	if err := json.Unmarshal(body, &document); err != nil {
		return "", false, state.FileFailure("read",
			l.PRFile(owner, repo, pr, state.FileIssueText), state.UnusableHint, err)
	}
	if document.Round != round {
		return "", false, nil
	}
	return document.Text, true, nil
}
