package finding

import (
	"time"

	"github.com/deligoez/cr/internal/state"
)

// Transition is one line of transitions.ndjson: §9.1.1's timestamp, head SHA
// and actor, beside the record and the two states the move joined.
//
// From is null for §9.1's first row, whose left column is a new record rather
// than a state, for the reason From keeps creation apart from the zero State.
type Transition struct {
	Record string    `json:"record"`
	From   *State    `json:"from"`
	To     State     `json:"to"`
	Actor  string    `json:"actor"`
	Head   string    `json:"head"`
	At     time.Time `json:"at"`
}

// Journal is one command run's share of §9.1.1: every move it was allowed,
// kept for the write that publishes the records the moves changed.
//
// A run has one actor, so the actor is the journal's and not each move's: the
// same `cr post --confirm` that posts a record is the one that discards another
// the reviewer deleted, and a journal cannot record the second under a
// different command's name.
type Journal struct {
	actor   Actor
	head    string
	at      time.Time
	entries []Transition
}

// NewJournal is the journal of one run of by, at head, stamped at.
func NewJournal(by Actor, head string, at time.Time) *Journal {
	return &Journal{actor: by, head: head, at: at.UTC(), entries: make([]Transition, 0)}
}

// Move asks §9.1's table whether the journal's actor may move record from
// `from` to `to`, and keeps the transition when it may. The move and its line
// are one call, so a command cannot make a move the journal does not hear
// about.
func (j *Journal) Move(record string, from From, to State) error {
	if err := MayTransition(record, from, to, j.actor); err != nil {
		return err
	}
	entry := Transition{Record: record, To: to, Actor: j.actor.String(), Head: j.head, At: j.at}
	if !from.creation {
		left := from.state
		entry.From = &left
	}
	j.entries = append(j.entries, entry)
	return nil
}

// Write appends the kept moves to the locked pull request's
// transitions.ndjson, one line each, and writes nothing when there are none.
//
// It is called under the lock the records are published under, so no other
// run's lines can land between a move and the records it changed.
func (j *Journal) Write(k *state.Lock) error {
	if len(j.entries) == 0 {
		return nil
	}
	return state.AppendRecords(k, state.FileTransitions, j.entries)
}
