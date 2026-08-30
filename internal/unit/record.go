package unit

import "github.com/deligoez/cr/internal/state"

// Record is one line of units.ndjson: §3.4.6's unit, and the head and round
// §2.3.3 stamps onto it.
//
// The unit is embedded whole rather than re-declared with the fields a
// particular reader wants. A second declaration of a stored record's shape is a
// second thing to keep in agreement with the writer, and it agrees silently — a
// field renamed on the way out decodes as a zero value in the second
// declaration, so a reader checking a record's `unit` against the round's ids
// would compare it against a set of empty strings and report nothing.
//
// It lives here rather than beside either of its users because both of them are
// about the same file: `cr brief` writes units.ndjson per §3.7 and every reader
// of §4.1.6's and §4.5.6's unit ids reads it back.
type Record struct {
	Unit
	state.Stamp
}
