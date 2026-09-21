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
// §9 no longer ends that life at posting: v0.5 gives §9.4 through §9.6 to the
// author replying, the author pushing, and the reviewer verifying, resolving or
// withdrawing, and `answered`, `addressed` and `withdrawn` are the states those
// produce.
//
// The vocabulary is still closed, and the closing still matters. There is no
// `verified`, `resolved` or `accepted`: verifying writes `answered` or
// `addressed` depending on what settled the record, and resolving a thread is a
// write to GitHub that §9.6.1 permits only on a record already in one of those
// two — it moves nothing, so it names nothing here.
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

// The ten states of §9.1's table, in the order the table lists them.
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
	// StatePosted was sent to GitHub and its thread created.
	//
	// It was terminal through v0.4, where the loop ended at posting. §9.1
	// gives it four exits now, three of them transitions and the fourth
	// standing still: a concern nobody has settled is open, not finished.
	StatePosted = State{"posted"}
	// StateAnswered is a posted question the author's reply settled
	// (§9.5.5).
	StateAnswered = State{"answered"}
	// StateAddressed is a posted record whose concern is gone at the
	// current head (§9.5.5).
	StateAddressed = State{"addressed"}
	// StateWithdrawn is a posted record the reviewer retracted (§9.6.2).
	StateWithdrawn = State{"withdrawn"}
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
	StateAnswered,
	StateAddressed,
	StateWithdrawn,
	StateStale,
}

// terminal is §9.1.2's list, in the order that item writes it. Only this list
// is written out; the open set is derived from it below, because §9.1.2 says
// "every other state is open" and two lists maintained side by side can
// disagree — §10.2.4's completeness check reads the open set as "no record
// remains in `draft` or `queued`", so a drift between them is a wrong verdict
// about whether a round is finished.
var terminal = []State{
	StateAnswered,
	StateAddressed,
	StateWithdrawn,
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

// Valid reports whether s is one of §9.1's ten. The zero State is not: a
// record that has never been through a §9.1 transition is in no state at all,
// which is neither open nor terminal.
func (s State) Valid() bool {
	return slices.Contains(states, s)
}

// Terminal reports whether s is one of §9.1.2's seven terminal states.
//
// `posted` left this list in v0.5 and that is the release's whole shape: a
// concern the author has not answered and nobody has verified is open, and
// §9.1.3 has the statistics count it that way.
func (s State) Terminal() bool {
	return slices.Contains(terminal, s)
}

// Open reports whether a record in this state is Open in §1.1's sense: any
// record in a non-terminal state per §9.1.
func (s State) Open() bool {
	return s.Valid() && !s.Terminal()
}

// States returns §9.1's ten in table order. The result is a copy, so a caller
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
//
// Two mutants survive in the capacity hint and are left deliberately, for the
// reason reservedFields already gives: the arithmetic changes only how much is
// allocated up front, and a test written to kill it would assert an
// implementation detail.
func OpenStates() []State {
	open := make([]State, 0, len(states)-len(terminal))
	for _, candidate := range states {
		if !candidate.Terminal() {
			open = append(open, candidate)
		}
	}
	return open
}

// UnsentStates returns the open states a record passes through before it is
// posted, in §9.1 table order: §10.2.4's set, derived rather than written out.
//
// It exists because v0.5 split two sets that were the same set through v0.4.
// §10.2.4 blocks a round on a record "still in `draft` or `queued`", and until
// `posted` became open those were exactly the open states, so the check read
// OpenStates and was right by coincidence. Reading OpenStates now would block
// every round that had posted anything — which is to say every round that
// finished — because §9.1.2 keeps a posted concern open until somebody settles
// it.
//
// The derivation is kept rather than replaced by a literal pair, so a state
// added to §9.1 before posting still blocks completeness and is still named in
// the reason. What is excluded is one state and the exclusion is the point:
// posting is where a record leaves this round's work, whatever happens to it
// afterwards.
func UnsentStates() []State {
	unsent := make([]State, 0, len(states)-len(terminal))
	for _, candidate := range OpenStates() {
		if candidate != StatePosted {
			unsent = append(unsent, candidate)
		}
	}
	return unsent
}

// Unsent reports whether a record in this state is still this round's work to
// finish, in §10.2.4's sense.
func (s State) Unsent() bool {
	return slices.Contains(UnsentStates(), s)
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
		"%q is not a record state; §9.1 has exactly %s, and verifying writes answered or addressed while resolving a thread moves no record",
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
