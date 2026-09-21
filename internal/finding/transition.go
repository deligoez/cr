package finding

import (
	"fmt"
	"slices"
	"strings"
)

// Actor is the third column of §9.1's transition table: the command, in the
// mode it was invoked in, that is asking to move a record.
//
// The table is not keyed on the pair of states. `cr draft` and `cr post
// --confirm` may both discard a queued record, but only `cr post` may post
// one, and `cr post --reconcile` — which may post on adopt — may not discard.
// A decision that read the two states alone would collapse those rows into one
// permission and hand every command the union of what any command may do, so
// the actor is part of the question and not context around it.
//
// The type is shaped like State, and for the same reason: its only field is
// unexported, so an actor cannot be built outside this file. That is how the
// clause under §9.1's table — `cr merge` is not a producer — is kept true by
// construction. `cr merge` has no value here, nothing outside the package can
// mint one, and TestNoCommandOutsideTheTableIsAnActor keeps the list closed.
type Actor struct{ name string }

// The actors of §9.1's third column, in the order the table's rows name them.
//
// `cr post --confirm` and `cr post --reconcile` are two actors rather than one
// command with a flag, because the table gives them different powers; and the
// qualifiers the table writes in prose — `cr post --reconcile` **on adopt**,
// `cr brief` **on a new head** — are preconditions those commands check before
// they ask. Nothing here can check them: whether a reconcile adopted, and
// whether the head moved, are facts about the run, and §9.1 gives them no state
// of their own to be read from.
var (
	// ActorRecord is `cr record`, which creates a record in draft and
	// applies §6.4.3's and §3.5.4's suppressions to one.
	ActorRecord = Actor{"cr record"}
	// ActorDraft is `cr draft`, which queues a record and discards one
	// during §7.2's triage.
	ActorDraft = Actor{"cr draft"}
	// ActorPostConfirm is `cr post --confirm`, the run that has been
	// through §8.5's gate.
	ActorPostConfirm = Actor{"cr post --confirm"}
	// ActorPostReconcile is `cr post --reconcile`, which may post on adopt
	// and may do nothing else.
	ActorPostReconcile = Actor{"cr post --reconcile"}
	// ActorBrief is `cr brief`, which stales the open records of the round
	// it closes, per §9.3.4.
	ActorBrief = Actor{"cr brief"}
	// ActorVerify is `cr verify`, which records the agent's judgement about
	// a posted record and moves it out of `posted` when that judgement
	// settles it (§9.5.5).
	//
	// It is an actor rather than a mode of some other command because the
	// judgement is the agent's and the write is cr's: §9.5.6 forbids cr to
	// reach the judgement itself, so the command that carries one in from
	// outside needs its own row.
	ActorVerify = Actor{"cr verify"}
	// ActorWithdrawConfirm is `cr withdraw --confirm`, the run that has
	// been through §8.5's gate and retracts a posted concern (§9.6.2).
	//
	// Only the confirmed run is an actor. A withdrawal prints its reply and
	// writes nothing without `--confirm`, so an unconfirmed run moves no
	// record and has no row to ask for.
	ActorWithdrawConfirm = Actor{"cr withdraw --confirm"}
)

// actors is §9.1's third column, deduplicated, in the order its rows name them.
var actors = []Actor{
	ActorRecord,
	ActorDraft,
	ActorPostConfirm,
	ActorPostReconcile,
	ActorBrief,
	ActorVerify,
	ActorWithdrawConfirm,
}

// String returns the command line the actor goes by in §9.1's table, which is
// what a refusal names and what §9.1.1's journal will record.
func (a Actor) String() string {
	return a.name
}

// Actors returns §9.1's third column in table order. The result is a copy, so a
// caller can neither widen the set nor reorder it.
func Actors() []Actor {
	return append(make([]Actor, 0, len(actors)), actors...)
}

// From is the left column of §9.1's transition table.
//
// Five of the six rows name a state the record is already in. The first names
// none — `— (new record)` — so the column's value set is the seven states plus
// creation, and creation is not one of them.
//
// It cannot be spelled as the zero State either. state.go gives that value the
// meaning "this record has been through no §9.1 transition", which is neither
// open nor terminal, and every Finding that has not been stamped carries it. If
// the zero State also meant creation, then a stored record whose state failed
// to read would be creatable a second time, and `cr record` would quietly
// re-open a record the round had already posted. So the two are different
// types: a State becomes a From only through Existing, and Creation is a value
// no State can equal.
type From struct {
	state    State
	creation bool
}

// Creation is §9.1's `— (new record)`: the left column of the row that brings a
// record into being.
var Creation = From{creation: true}

// Existing is the From of a record that is already in state s.
//
// The zero From is deliberately not Creation: it is Existing(State{}), a record
// in no state §9.1 defines, which no row names and every actor is refused.
func Existing(s State) From {
	return From{state: s}
}

// String renders the From as a refusal reads it: the state's name when there is
// one, and what the record is instead when there is not.
func (f From) String() string {
	switch {
	case f.creation:
		return "a new record"
	case f.state.Valid():
		return f.state.String()
	default:
		return "in no §9.1 state"
	}
}

// row is one row of §9.1's transition table, in its three columns. A cell
// holding more than one value is a slice, so the table below is the spec's
// transcribed rather than expanded — a row that gains a value gains it in one
// place, where the spec wrote it.
type row struct {
	from []From
	to   []State
	by   []Actor
}

// table is §9.1's transition table, row for row, in the order the spec writes
// it. It is the whole of what cr permits: §9.1 says a transition not listed
// there MUST be rejected, so the decision is closed by default and a triple
// nobody thought about is refused rather than allowed.
//
// `cr merge` is absent, and that absence is the spec's own sentence: it runs on
// role output files before any record exists, and §6.4.3's duplicate marking is
// applied by `cr record` as it writes. §6.5.1 says the same from the other side
// — `cr merge`'s output carries no computed field except `duplicate_of` — so
// nothing it produces is in a §9.1 state at all.
var table = []row{
	{from: []From{Creation}, to: []State{StateDraft}, by: []Actor{ActorRecord}},
	{from: []From{Existing(StateDraft)}, to: []State{StateDuplicate, StateSuppressed}, by: []Actor{ActorRecord}},
	{from: []From{Existing(StateDraft)}, to: []State{StateQueued}, by: []Actor{ActorDraft}},
	{from: []From{Existing(StateQueued)}, to: []State{StateDiscarded}, by: []Actor{ActorDraft, ActorPostConfirm}},
	{from: []From{Existing(StateQueued)}, to: []State{StatePosted}, by: []Actor{ActorPostConfirm, ActorPostReconcile}},
	{from: []From{Existing(StateDraft), Existing(StateQueued)}, to: []State{StateStale}, by: []Actor{ActorBrief}},
	{from: []From{Existing(StatePosted)}, to: []State{StateAnswered, StateAddressed}, by: []Actor{ActorVerify}},
	{from: []From{Existing(StatePosted)}, to: []State{StateWithdrawn}, by: []Actor{ActorWithdrawConfirm}},
}

// SentStates returns `posted` and every state §9.1's table reaches from it, in
// table order: the records whose comment reached GitHub, whatever has happened
// to them since.
//
// It is derived from the table rather than written out, for the reason
// UnsentStates is: v0.5 gave `posted` three exits, and a reader that still
// asked for `posted` alone lost a record the moment `cr verify` settled it.
// §2.6.3.1 harvests "comments posted from recorded rounds", and an answered
// question's comment was posted all the same.
func SentStates() []State {
	sent := []State{StatePosted}
	for _, candidate := range states {
		for _, r := range table {
			if slices.Contains(r.from, Existing(StatePosted)) && slices.Contains(r.to, candidate) &&
				!slices.Contains(sent, candidate) {
				sent = append(sent, candidate)
			}
		}
	}
	return sent
}

// Sent reports whether a record in this state reached GitHub, per SentStates.
func (s State) Sent() bool {
	return slices.Contains(SentStates(), s)
}

// move is one cell of the expanded table: the exact question a command asks.
// Every field is comparable, so the set below is a map and the decision is a
// lookup rather than a walk.
type move struct {
	from  From
	to    State
	actor Actor
}

// allowed is table expanded into the triples it lists. It is built once, from
// the rows, so there is no second transcription of §9.1 to drift from the
// first.
var allowed = expand()

func expand() map[move]bool {
	set := make(map[move]bool)
	for _, r := range table {
		for _, from := range r.from {
			for _, to := range r.to {
				for _, by := range r.by {
					set[move{from: from, to: to, actor: by}] = true
				}
			}
		}
	}
	return set
}

// MayTransition answers whether by may move the record from `from` to `to`.
//
// It decides and records nothing: no state is written here, and §9.1.1's
// journal is written by the command that made the move. A caller asks before
// it writes, and a nil answer is the only thing that lets it.
//
// The record id is taken because §9.1 fixes what a refusal must say — the
// record and its current state — and neither is derivable from the states
// alone.
func MayTransition(record string, from From, to State, by Actor) error {
	if allowed[move{from: from, to: to, actor: by}] {
		return nil
	}
	return &IllegalTransitionError{Record: record, From: from, To: to, Actor: by}
}

// IllegalTransitionError reports a move §9.1's table does not list.
//
// The cli layer maps it onto exit code 4, which §9.1 fixes: the record is
// well-formed and the command line is right, and what refuses is where the
// record already stands, which is the state conflict §11.2 codes 4.
type IllegalTransitionError struct {
	// Record is the id of the record the move was asked for. §9.1 requires
	// the refusal to name it.
	Record string
	// From is where the record stands now, which §9.1 requires the refusal
	// to name as well, or Creation when there is no record yet.
	From From
	// To is the state the command asked for.
	To State
	// Actor is the command, in the mode it was invoked in, that asked.
	Actor Actor
}

func (e *IllegalTransitionError) Error() string {
	return fmt.Sprintf("%s is %s and %s may not move it to %s; %s",
		e.Record, e.From, e.Actor, e.To, movesOutOf(e.From))
}

// movesOutOf is the hint of §12.4: the next actionable step is whichever of
// §9.1's rows does apply to the state the record is actually in, so the refusal
// names them rather than leaving the caller to open the spec.
func movesOutOf(from From) string {
	listed := make([]string, 0)
	for _, r := range table {
		if !slices.Contains(r.from, from) {
			continue
		}
		for _, to := range r.to {
			for _, by := range r.by {
				listed = append(listed, fmt.Sprintf("%s by %s", to, by))
			}
		}
	}
	if len(listed) == 0 {
		return "§9.1 lists no move out of it"
	}
	return "§9.1 allows " + strings.Join(listed, ", ")
}
