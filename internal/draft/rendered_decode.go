package draft

import (
	"encoding/json"
	"fmt"
)

// DecodeRendered reads a round's rendered.json back into the entries Rendered
// produced, keyed by record id.
//
// A document that is not an object of strings is refused rather than read as
// empty. An empty map would make every block look unrendered, and Ingest keeps
// the body of a block it has no entry for, so nothing would be lost — but it
// would be a silent answer about a file cr wrote itself and can no longer read,
// and the reviewer is owed the error instead.
func DecodeRendered(name string, body []byte) (map[string]string, error) {
	entries := make(map[string]string)
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("cannot read %s as §7.1.5's record-to-region entries: %w", name, err)
	}
	return entries, nil
}
