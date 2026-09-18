package draft

import (
	"encoding/json"
	"fmt"

	"github.com/deligoez/cr/internal/state"
)

// DecodeRendered reads a round's rendered.json back into the entries Rendered
// produced, keyed by record id.
//
// A document that is not an object of strings is refused rather than read as
// empty. An empty map would make every block look unrendered, and Ingest keeps
// the body of a block it has no entry for, so nothing would be lost — but it
// would be a silent answer about a file cr wrote itself and can no longer read,
// and the reviewer is owed the error instead.
//
// The refusal is the one decodeMeta gives for a meta.json that does not decode:
// a file of §2.2's tree cr found, read whole and cannot use, which §11.2 codes
// 3 with state.UnusableHint. A bare error here took exitCodeFor's usage floor
// instead — measured on main, `cr draft` exited 2 over a corrupt rendered.json,
// telling the reviewer to check a command line that was right about a file only
// a repair can fix.
func DecodeRendered(name string, body []byte) (map[string]string, error) {
	entries := make(map[string]string)
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, state.FileFailure("read", name, state.UnusableHint,
			fmt.Errorf("not §7.1.5's record-to-region entries: %w", err))
	}
	return entries, nil
}
