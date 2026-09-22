package brief

import (
	"encoding/json"
	"time"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/migrate"
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
func invalidate(src *Sources, held *state.Lock, assembled *Brief) error {
	// §9.1.1: one journal line per record the sweep moves, at the head this
	// brief moves to. The moves are decided line by line inside the sweep,
	// and the journal is written under the same lock after every move is
	// decided and before findings.ndjson publishes them — the order `cr
	// record`, `cr draft` and `cr post` write theirs in.
	journal := finding.NewJournal(finding.ActorBrief, assembled.Head, time.Now())
	swept, err := sweepOpenRecords(held, journal, newCarrier(src, assembled))
	if err != nil {
		return err
	}
	assembled.Staled, assembled.Carried, assembled.Migrations = swept.staled, swept.carried, swept.migrations
	if err := clearMapping(held, assembled); err != nil {
		return err
	}
	return carryClaims(held, assembled)
}

// clearMapping is §9.3.4's second clause, scoped by §9.3.5 to the round being
// opened: that round starts with no mapping, and every earlier round's pairs
// stay in the file as history.
//
// The closing round's pairs are not dangling. `write` adds the opening round's
// units to units.ndjson beside the closing round's rather than replacing them,
// so every pair a round recorded still names a unit that round's lines hold,
// and a stale record's unit stays resolvable from state. No command of the
// opening round reads them: each reads its own round's lines per §9.3.5.
//
// §9.3.2 refuses `cr map record` until this brief has opened the round, so
// the clearing removes no pair any earlier round recorded.
func clearMapping(held *state.Lock, assembled *Brief) error {
	return state.ClearStamped(held, state.FileMapping, assembled.Round)
}

// swept is what sweepOpenRecords did, in findings.ndjson's order.
type swept struct {
	// staled and carried are the ids moved to `stale` and carried to
	// `draft`, which are the records the journal holds a line for.
	staled, carried []string
	// migrations are §9.4.7's lines, one per record the sweep migrated.
	migrations []migrate.Record
}

// sweepOpenRecords is §9.3.4's first clause: every record still in `draft` or
// `queued` is migrated per §9.4, and moves to `draft` in the new round when
// §9.4.5 carries it and to `stale` otherwise.
//
// A carried record's line moves rather than being copied, and that is the one
// place RewriteStamped's lines change their round and head. §6.1 keeps a record
// id for the life of the pull request, and every reader that finds a record by
// id across rounds — finding.MoveSent among them — refuses an id two lines
// carry; a copy in the new round would be exactly that. The closing round keeps
// its history of the move in transitions.ndjson and migrations.ndjson.
//
// A record already in the new round is left alone: it is one an interrupted
// run of this same increment carried, and migrating it again from the place it
// was carried to would report a move that did not happen.
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
//
// A field it cannot decode is refused as the stored file cr cannot use that it
// is: §6.1.4 has cr write `state`, so a line naming no state of §9.1, or an id
// that is not an f<n> string — null and "" included — is cr's own state edited
// by hand, which §11.2 codes 3. The fields arrive keyed as RewriteStamped
// documents, the way encoding/json binds them, so a line spelling a key
// `"State"` is swept as every read of it decodes it.
// RewriteStamped puts the file's path and the line's number in front, because
// the line is what the user has to open and only the walk knows which it was.
//
// It returns the ids it moved, in the file's order, which are the records the
// journal holds a line for.
func sweepOpenRecords(held *state.Lock, journal *finding.Journal, carrier *carrier) (swept, error) {
	out := swept{staled: make([]string, 0), carried: make([]string, 0), migrations: make([]migrate.Record, 0)}
	err := state.RewriteStamped(held, state.FileFindings,
		func(fields map[string]json.RawMessage) (bool, error) {
			var current finding.State
			if written, supplied := fields[fieldState]; supplied {
				if err := json.Unmarshal(written, &current); err != nil {
					return false, unusable(fieldState, err)
				}
			}
			// §9.3.4 names the two states it stales — `draft` and
			// `queued` — and this asks for them by name rather than
			// for whichever states are open.
			//
			// The two were the same set through v0.4 and are not
			// from v0.5: §9.1.2 made `posted` open, because a
			// concern nobody has settled is not finished. Staling
			// by openness would abandon every posted concern the
			// moment the author pushed, which is the opposite of
			// what §9.4 through §9.6 exist to do — the record has
			// to survive the push to be verified against it.
			//
			// A line carrying no state, or the JSON null §9.1's
			// decoder leaves alone, is in no §9.1 state at all and
			// matches neither name. Leaving it is the conservative
			// half: cr wrote no such record, so moving one would be
			// this version acting on a line it does not understand.
			if current != finding.StateDraft && current != finding.StateQueued {
				return false, nil
			}
			// A record named anything §6.1 does not spell f<n> is no
			// id: "" among them, and a JSON null, which decodes into a
			// string as "" with no error.
			var id string
			if err := json.Unmarshal(fields[fieldID], &id); err != nil {
				return false, unusable(fieldID, err)
			}
			if !finding.ValidID(id) {
				return false, unusable(fieldID, &storedIDError{written: fields[fieldID]})
			}
			return out.move(fields, id, current, journal, carrier)
		},
		// The journal is appended once every move is decided and before
		// findings.ndjson is published, so an append that fails leaves
		// every record where it was and the re-run moves and journals
		// them again. Publishing first would leave the records stale
		// with no line, and the re-run would find nothing left to move.
		// migrations.ndjson is written with it, for the same reason.
		func() error {
			if err := journal.Write(held); err != nil {
				return err
			}
			if len(out.migrations) == 0 {
				return nil
			}
			return state.AppendRecords(held, state.FileMigrations, out.migrations)
		})
	if err != nil {
		return swept{}, err
	}
	return out, nil
}

// move is one open record's half of the sweep: carried to `draft` in the new
// round when §9.4.5 carries it, staled otherwise, and journalled and reported
// either way.
func (out *swept) move(
	fields map[string]json.RawMessage, id string, current finding.State,
	journal *finding.Journal, carrier *carrier,
) (bool, error) {
	record, err := decodeSwept(fields)
	if err != nil {
		return false, err
	}
	if record.Round == carrier.assembled.Round {
		return false, nil
	}
	line, moved, err := carrier.carry(record)
	if err != nil {
		return false, err
	}
	out.migrations = append(out.migrations, line)
	if moved != nil {
		if err := journal.Move(id, finding.Existing(current), finding.StateDraft); err != nil {
			return false, err
		}
		out.carried = append(out.carried, id)
		return true, carried(fields, moved, carrier.assembled)
	}
	if err := journal.Move(id, finding.Existing(current), finding.StateStale); err != nil {
		return false, err
	}
	stale, err := json.Marshal(finding.StateStale)
	if err != nil {
		return false, err
	}
	fields[fieldState] = stale
	out.staled = append(out.staled, id)
	return true, nil
}

// decodeSwept reads a whole record out of the fields the sweep was handed, the
// way every other read of findings.ndjson binds them.
func decodeSwept(fields map[string]json.RawMessage) (*finding.Finding, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	var record finding.Finding
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, unusable("record", err)
	}
	return &record, nil
}

// carried writes §9.4.5's move onto the line's fields: the new round and head,
// the unit and anchor the record now sits on, its citations as §9.4.8 read
// them again, no probe, and `draft`.
func carried(fields map[string]json.RawMessage, moved *finding.Finding, assembled *Brief) error {
	set := map[string]any{
		"round": assembled.Round, "head": assembled.Head,
		"unit": moved.Unit, "anchor": moved.Anchor, fieldState: finding.StateDraft,
	}
	if moved.Citations != nil {
		set["citations"] = moved.Citations
	}
	for key, value := range set {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		fields[key] = encoded
	}
	delete(fields, "probe")
	return nil
}

// unusable is the refusal of one stored findings.ndjson field the sweep could
// not decode, carrying state.UnusableHint as §12.4's step. It names the file
// rather than its path, which RewriteStamped adds with the line.
func unusable(field string, err error) error {
	return state.FileFailure("use the "+field+" field of", state.FileFindings, state.UnusableHint, err)
}

// storedIDError is the refusal of an open record whose id §6.1 does not spell.
// It quotes the id as the line writes it, so a null reads as null and not as
// the "" it decodes to.
type storedIDError struct {
	written json.RawMessage
}

func (e *storedIDError) Error() string {
	return "reads " + string(e.written) + ", and §6.1 spells a record id f<n>, numbered from one"
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
