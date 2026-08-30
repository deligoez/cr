package finding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// State is one row of §9.1's table: where a record stands in its life.
//
// §9's opening paragraph fixes how short that life is. A record's life in v0.1
// ends when it is posted; everything after that — the author replying, pushing,
// and the reviewer verifying, resolving, or withdrawing — is the re-review half
// of the loop, and §1.3.6 puts it in v0.2. So there is no `verified`,
// `resolved`, `accepted`, or `withdrawn` here, and the type is shaped so that
// there cannot be one by accident either.
//
// That shape is why this is a struct around an unexported name rather than the
// string type its five siblings in finding.go use. A defined string type is
// open: `var s State = "verified"` compiles anywhere in the tree, because an
// untyped constant is assignable to it, and the vocabulary would then be a
// convention rather than a fence. A struct whose only field is unexported can
// be built nowhere outside this file — a composite literal needs the field, and
// there is no conversion into it — so a state that is not one of the seven
// below is unrepresentable, and adding one is an edit to this file, where
// TestTheSevenStatesAreTheOnesTheSpecWrites is waiting.
//
// The cost is that the seven are `var` rather than `const`, since Go has no
// constant of struct type. Reassigning one is not widening the vocabulary — it
// is sabotage that breaks every test at once — and the hole it leaves is much
// smaller than the one it closes.
type State struct{ name string }

// The seven states of §9.1's table, in the order the table lists them.
var (
	// StateDraft is recorded, not yet queued for a draft.
	StateDraft = State{"draft"}
	// StateQueued is rendered into the current draft.
	StateQueued = State{"queued"}
	// StateDuplicate is suppressed as a duplicate of another record
	// (§6.4.3), which names the representative in `duplicate_of`.
	StateDuplicate = State{"duplicate"}
	// StateSuppressed is suppressed by an existing human thread (§3.5.4).
	StateSuppressed = State{"suppressed"}
	// StateDiscarded was deleted during triage: a waiver is written and
	// `disposition` is set (§7.2).
	StateDiscarded = State{"discarded"}
	// StatePosted was sent to GitHub and its thread created. It is where
	// v0.1 ends.
	StatePosted = State{"posted"}
	// StateStale was abandoned unposted when the head moved (§9.3.4).
	StateStale = State{"stale"}
)

// states is §9.1's table in its own order. It is the whole vocabulary: the
// transition table of §9.1 and the journal of §9.1.1 are built on top of this
// set and add no state to it.
var states = []State{
	StateDraft,
	StateQueued,
	StateDuplicate,
	StateSuppressed,
	StateDiscarded,
	StatePosted,
	StateStale,
}

// terminal is §9.1.2's list, in the order that item writes it. Only this list
// is written out; the open set is derived from it below, because §9.1.2 says
// "every other state is open" and two lists maintained side by side can
// disagree — §10.2.4's completeness check reads the open set as "no record
// remains in `draft` or `queued`", so a drift between them is a wrong verdict
// about whether a round is finished.
var terminal = []State{
	StatePosted,
	StateDiscarded,
	StateDuplicate,
	StateSuppressed,
	StateStale,
}

// String returns the state's name, which is what it goes by on the wire and in
// §9.1's table.
func (s State) String() string {
	return s.name
}

// Valid reports whether s is one of §9.1's seven. The zero State is not: a
// record that has never been through a §9.1 transition is in no state at all,
// which is neither open nor terminal.
func (s State) Valid() bool {
	return slices.Contains(states, s)
}

// Terminal reports whether s is one of §9.1.2's five terminal states.
func (s State) Terminal() bool {
	return slices.Contains(terminal, s)
}

// Open reports whether a record in this state is Open in §1.1's sense: any
// record in a non-terminal state per §9.1.
func (s State) Open() bool {
	return s.Valid() && !s.Terminal()
}

// States returns §9.1's seven in table order. The result is a copy, so a caller
// can neither widen the set nor reorder it.
func States() []State {
	return append(make([]State, 0, len(states)), states...)
}

// OpenStates returns the states §1.1 calls open, in §9.1 table order.
//
// It is derived rather than written out, so a caller asking which states are
// open — §10.2.4's completeness check is the first — reads the same answer
// Open gives one record, and cannot restate it as a list of its own that a
// later state would leave behind.
func OpenStates() []State {
	open := make([]State, 0, len(states)-len(terminal))
	for _, candidate := range states {
		if !candidate.Terminal() {
			open = append(open, candidate)
		}
	}
	return open
}

// UnknownStateError reports a value that names no state of §9.1.
//
// It carries the value so the user can see what was rejected. Reaching it means
// something outside cr wrote the field: §6.1.4 has cr write `state` and rejects
// a record arriving with it, so a stored line naming an eighth state is either
// hand-edited or written by a version this one does not share a vocabulary
// with.
type UnknownStateError struct {
	// Value is the offending name, exactly as it was written.
	Value string
}

func (e *UnknownStateError) Error() string {
	names := make([]string, 0, len(states))
	for _, known := range states {
		names = append(names, known.name)
	}
	return fmt.Sprintf(
		"%q is not a record state; v0.1 has exactly %s, and §1.3.6 puts verifying, resolving, and withdrawing in v0.2",
		e.Value, strings.Join(names, ", "),
	)
}

// ParseState resolves a name into the state it goes by in §9.1's table. It is
// the only way into the type from a string, so every value that exists came
// either from a constant above or through this check.
func ParseState(name string) (State, error) {
	for _, known := range states {
		if known.name == name {
			return known, nil
		}
	}
	return State{}, &UnknownStateError{Value: name}
}

// MarshalJSON writes the state's name. The field is omitted entirely when no
// state is set, through `omitzero` on §6.1's row, so nothing writes a record
// with an empty state under a key that claims to hold one.
func (s State) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.name)
}

// UnmarshalJSON reads a state back through ParseState, so a name outside §9.1
// is refused at the file rather than carried around as a value nothing can act
// on.
//
// null is left alone, as the encoding/json convention has it. A line holding
// `"state": null` supplied the key, and §6.1.4's fence — which reads the wire
// keys, not the decoded record — is what has to answer it; failing here would
// answer with the wrong error and take that item's rejection out of §6.1.4's
// hands.
func (s *State) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), nullLiteral) {
		return nil
	}
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	parsed, err := ParseState(name)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}
