package mapping

import "fmt"

// NoGapError reports a set-aside naming a claim the round raises no §4.1.3
// entry for.
//
// §4.1.8 stamps `set_aside_note` on *that gap entry*, so an entry is what the
// command needs and there is none: the claim is mapped to a unit, or it is not
// one of the round's claims at all. Neither is a broken file or a mistyped
// command line — the state directory read and parsed without trouble and the
// command line is the shape §11 gives it — so §11.2 codes it 1, alongside the
// unknown claim id §4.1.6 refuses a mapping pair for.
//
// The round is carried because §9.3.5 makes the answer round-scoped: the same
// claim can raise an entry in one round and none in the next, and a refusal
// that named only the claim would read as a claim cr has never heard of.
type NoGapError struct {
	// Claim is the claim id the set-aside named.
	Claim string
	// Round is the round whose entries were searched.
	Round int
}

func (e *NoGapError) Error() string {
	return fmt.Sprintf(
		"round %d raises no unimplemented-claim entry for %s: "+
			"§4.1.8 stamps an entry §4.1.3 raised, and a claim the round maps to a unit raises none; "+
			"run `cr status` for the claims this round has left unmapped",
		e.Round, e.Claim,
	)
}

// SetAside is §4.1.8's stamp: the round's gap entries, with noteID written onto
// the one standing against claim.
//
// It returns the whole round rather than the one entry it changed, because that
// is what the write takes. state.ReplaceStamped replaces the current round's
// records with what it is handed and carries every earlier round through
// untouched, so an entry left out of this result would be an entry deleted —
// §10.2.3 would stop blocking on an unimplemented claim nobody set aside.
//
// Judging the claim out of scope is not decided here and is not decided
// anywhere in cr. §4.1.8 reserves it for the agent, and what this does is
// record the decision and the note id it rests on: the note itself is checked
// by the caller against the store §3.6 keeps for the pull request's issue key,
// which is the one validation §4.1.8 asks for.
//
// A second set-aside of the same claim overwrites the note rather than refusing.
// The stamp is one field holding the note the decision rests on, and a reviewer
// who recorded a better note is correcting the reference rather than making a
// new decision; §4.1.8 gives the field no history and cr has no second slot to
// keep one in.
func SetAside(recorded []Gap, claim, noteID string, round int) ([]*Gap, error) {
	current := make([]*Gap, 0, len(recorded))
	stamped := false
	for i := range recorded {
		if recorded[i].Round != round {
			continue
		}
		entry := recorded[i]
		if entry.Claim == claim {
			entry.SetAsideNote = noteID
			stamped = true
		}
		current = append(current, &entry)
	}
	if !stamped {
		return nil, &NoGapError{Claim: claim, Round: round}
	}
	return current, nil
}
