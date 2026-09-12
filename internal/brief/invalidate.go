package brief

import (
	"encoding/json"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/state"
)

// The two fields of §6.1's table this file reads off a stored finding, by the
// JSON keys that table gives them. They are spelled here rather than taken off
// a struct because the sweep below reads the fields a line supplied and never
// decodes it: a record a later version wrote is rewritten in one field and
// carried through in every other.
const (
	fieldID    = "id"
	fieldState = "state"
)

// invalidate carries out §9.3.4, in the order that sentence writes it: every
// record still in `draft` or `queued` moves to `stale`, `mapping.ndjson` is
// cleared, and the claims are carried forward unchanged.
//
// It runs only on §9.3.3's increment. A same-head brief must be idempotent, and
// none of the three below is: a sweep would find nothing to move, but the
// clearing would take out a mapping `cr map record` had just written for the
// open round, and the carry-forward would rewrite the claims of a round nobody
// closed.
//
// The fourth clause of §9.3.4 — recompute units and threads for the new head —
// needs nothing here. `assemble` computes both from the head it fetched on
// every run, and `write` publishes them, so a moved head recomputes them by
// doing what an unmoved one does.
//
// It is called under the §2.3.1 lock `persist` holds, and before `write`
// publishes meta.json. The order is what makes a failure recoverable: meta.json
// still records the closing round, so a re-run makes the same comparison, finds
// the same increment, and does the whole of §9.3.4 again. Writing meta.json
// first would leave a round whose head says the increment happened and whose
// records were never swept, and the re-run that would have fixed it compares
// equal and does nothing.
func invalidate(held *state.Lock, assembled *Brief) error {
	if err := staleOpenRecords(held); err != nil {
		return err
	}
	if err := clearMapping(held, assembled); err != nil {
		return err
	}
	return carryClaims(held, assembled)
}

// clearMapping is §9.3.4's second clause, and the round it clears is the one
// being closed rather than the one being opened.
//
// §9.3.5 scopes a clearing to the current round, and at the moment this runs
// the current round is still the recorded one: `meta.json` has not been written
// yet, and every reader arriving before it sees the round this brief is closing.
// So the two sentences agree, and they agree on the only reading that does
// anything — clearing the round being opened would clear a round that has
// never held a record, and §9.3.4 would be a sentence with no effect.
//
// Deleting it is not deleting history, which is what §9.3.5 protects. A mapping
// is §4.1.6's join between a claim and a unit, and the unit half does not
// survive: units.ndjson is republished whole on every brief, so the ids the
// closing round's pairs name are gone the moment the units are recomputed.
// What would be kept is a set of dangling references, and §4.1.6 rejects a pair
// naming an unknown unit id — so a round that kept them would be holding rows
// it would itself refuse.
//
// The claims the pairs name do survive, because carryClaims records them into
// the round being opened. That asymmetry is the whole content of §9.3.4's
// second and third clauses read together: the claims are still the claims, the
// units are not still the units, and the mapping between them has to be made
// again.
func clearMapping(held *state.Lock, assembled *Brief) error {
	return state.ClearStamped(held, state.FileMapping, assembled.round.carries)
}

// staleOpenRecords is §9.3.4's first clause: every record still in `draft` or
// `queued` moves to `stale`.
//
// No round scopes it, because §9.3.4 does not scope it: the records it moves
// are the ones the closing round produced, so a sweep reading only the round
// being opened would reach none of them. That is round 12's
// closed-exemption-list-blocks-required-behaviour, and round-scoping's
// acceptance carries the resolution — §9.3.4's transition is exempt from
// §9.3.5's scoping, or `stale` is a state no record can reach. Applying it to
// every open record rather than to one round's is also what makes it
// self-healing: a round left un-swept by an interrupted brief is swept by the
// next one.
//
// The open set is asked of §9.1 rather than written out here. State.Open is
// derived from §9.1.2's terminal list, so `draft` and `queued` are named in one
// place, and a state added to §9.1 cannot be left out of this sweep by anyone
// forgetting to widen a literal.
//
// Every move is put to §9.1's table before it is made, exactly as `cr draft`
// puts its own. Nothing here can be talked past that table: a state §9.1 calls
// open and gives `cr brief` no row out of would refuse the brief rather than be
// swept silently — which is the honest failure, because §9.3.2 makes this
// command the only way forward and a round it left half-open would go on
// looking like a round in progress.
func staleOpenRecords(held *state.Lock) error {
	return state.RewriteStamped(held, state.FileFindings,
		func(fields map[string]json.RawMessage) (bool, error) {
			var current finding.State
			if written, supplied := fields[fieldState]; supplied {
				if err := json.Unmarshal(written, &current); err != nil {
					return false, err
				}
			}
			// A line carrying no state, or the JSON null §9.1's
			// decoder leaves alone, is in no §9.1 state at all and
			// is neither open nor terminal. Leaving it is the
			// conservative half: cr wrote no such record, so moving
			// one would be this version acting on a line it does
			// not understand.
			if !current.Open() {
				return false, nil
			}
			var id string
			if err := json.Unmarshal(fields[fieldID], &id); err != nil {
				return false, err
			}
			if err := finding.MayTransition(
				id, finding.Existing(current), finding.StateStale, finding.ActorBrief,
			); err != nil {
				return false, err
			}
			stale, err := json.Marshal(finding.StateStale)
			if err != nil {
				return false, err
			}
			fields[fieldState] = stale
			return true, nil
		})
}

// carryClaims is §9.3.4's "claims are carried forward unchanged": the claims
// the closing round recorded become the opening round's, with nothing about
// them re-read and nothing re-extracted.
//
// Unchanged is about the claim and not about §2.3.3's stamp. A round reads only
// its own records per §9.3.5, so a claim left stamped with the round that
// closed would be invisible to every command of the round that opened — and
// §4.1.6 checks a mapping's `claim` against exactly that set, so the round
// would start with claims it could map nothing to. Carrying forward is
// therefore re-recording the same claims under the new pair, which is what
// `cr map record` already documents as the reading of this sentence.
//
// The stamp lands on the payload too, because the records are the payload's own
// and ReplaceStamped writes through them. §3.7.3 has `cr brief` report the
// claims recorded for this round, and a report whose stamp disagreed with the
// file it was just written to would be a second answer to a question the run
// answered once.
//
// Nothing is re-extracted here, and nothing could be: the claims come from
// claims.ndjson, and the only thing this run read of the issue is the text
// §3.3.3's drift comparison was run against. §3.3.1 is the one door extraction
// comes through and it is `cr claims record`'s.
func carryClaims(held *state.Lock, assembled *Brief) error {
	carried := make([]*intent.Claim, 0, len(assembled.Claims))
	for i := range assembled.Claims {
		carried = append(carried, &assembled.Claims[i])
	}
	at := state.Stamp{Head: assembled.Head, Round: assembled.Round}
	return state.ReplaceStamped(held, state.FileClaims, at, carried)
}
