package finding

import (
	"encoding/json"
	"fmt"

	"github.com/deligoez/cr/internal/state"
)

// The folded spellings state.RewriteStamped hands a stored line's keys under,
// which for these two lowercase names are the names themselves.
const (
	movedFieldID    = "id"
	movedFieldState = "state"
)

// MoveSent changes one record's state in the line that holds it, whichever
// round that is, and writes journal's lines before the file is published.
//
// It exists for the records §9.5 and §9.6 act on. A posted record stays in the
// round that posted it: §9.3.4's sweep leaves it alone and nothing copies it
// into the round a moved head opens. So the line keeps its `round` and `head`.
// state.ReplaceStamped would restamp every record of the current round, and a
// record found in an earlier one would be written a second time into this
// round — beside unit ids this round has reused for other code, and counted by
// this round's posting as a comment it sent. §2.3.3's pair says which run wrote
// the record; a verdict or a withdrawal changes where it stands, which is
// §9.3.4's reason for RewriteStamped too.
//
// from is the state the caller's lock-free read saw. It is checked again under
// the lock, so a second run that settled the record in between is refused as
// the state conflict it is rather than overwritten.
func MoveSent(k *state.Lock, id string, from, to State, journal *Journal) error {
	moved := 0
	return state.RewriteStamped(k, state.FileFindings,
		func(fields map[string]json.RawMessage) (bool, error) {
			var named string
			if err := json.Unmarshal(fields[movedFieldID], &named); err != nil || named != id {
				return false, nil
			}
			var current State
			if err := json.Unmarshal(fields[movedFieldState], &current); err != nil {
				return false, err
			}
			if current != from {
				return false, &IllegalTransitionError{Record: id, From: Existing(current), To: to, Actor: journal.actor}
			}
			encoded, err := json.Marshal(to)
			if err != nil {
				return false, err
			}
			fields[movedFieldState] = encoded
			moved++
			return true, nil
		},
		func() error {
			if moved != 1 {
				return fmt.Errorf("findings.ndjson holds %d lines for record %s, and §6.1 gives an id one", moved, id)
			}
			return journal.Write(k)
		})
}
