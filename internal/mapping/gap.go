package mapping

import "github.com/deligoez/cr/internal/state"

// Gap is one line of intent-gaps.ndjson: §4.1.3's unimplemented-claim entry,
// raised for a claim the round's mapping maps to zero units.
//
// It is not a record and it is not a coverage cell. §4.5.5 requires every cell
// to name a unit, and a claim nothing implements has no code location by
// construction — there is no unit to name and no anchor to carry — so §4.1.3
// gives it a file of its own rather than a place in findings.ndjson or
// coverage.ndjson. §1.6.1 is the same boundary read from the posting side: v0.1
// has no unanchored comment channel, so this entry reaches the reviewer through
// `cr status` per §10.1.2 and is never drafted or posted.
//
// The four fields are §4.1.3's. `claim` names what is unimplemented; `head` and
// `round` are §2.3.3's stamp, written by state.ReplaceStamped and never by the
// agent, which is what lets §9.3.5 scope a reader to the current round's
// entries; `set_aside_note` is §4.1.8's, the note id `cr claims set-aside`
// stamps once the agent has judged the claim out of scope, and §10.2.3 stops
// blocking completeness on a claim that carries one.
//
// `set_aside_note` is written even when it is empty, rather than omitted. An
// entry is the whole of what §10.1.2 reports and what §10.2.3 reads, and the
// difference between a claim nobody has judged and a claim somebody set aside
// is the field itself: a reader that had to tell them apart by the key being
// absent would be reading the encoder's settings rather than the decision.
type Gap struct {
	// Claim is the claim id of §3.3, formed as <ISSUE-KEY>#c<n>.
	Claim string `json:"claim"`
	state.Stamp
	// SetAsideNote is the note id of §4.1.8, empty until a set-aside.
	SetAsideNote string `json:"set_aside_note"`
}

// Gaps is §4.1.3's derivation over one round: an entry for each of the given
// claims that the round's mapping maps to no unit, in the order the claims are
// given.
//
// It is the mirror of Unmapped and reads the same file for the same reason.
// §4.1.6's last sentence gives every rule of §4.1 through §4.4 that speaks of
// the claims a unit is mapped to one source, and a derivation that looked at
// what the claim says or what the diff does would be cr forming the judgement
// §4.1.6 reserves for the agent.
//
// recorded is intent-gaps.ndjson as it stands, and it is taken so that the
// carry-forward is part of the derivation rather than a step a caller could
// skip. §4.1.6 permits the mapping to be re-recorded, so this runs again over
// the same round, and a literal re-derivation would hand every claim a fresh
// entry with no `set_aside_note` — discarding the reviewer's §4.1.8 decisions
// and silently re-imposing §10.2.3's block. A claim that is still unmapped
// therefore keeps the note the round already recorded against it. A claim that
// has since been mapped raises no entry at all, so its stamp goes with the
// entry it belonged to.
// That second case is where the second return value comes from: the stamps
// this derivation did not carry forward, so that a drop the reviewer would
// otherwise learn about only from a completeness block is stated by the run
// that performed it. The two are returned together because they are one answer
// — what the round's gaps are now, and which of the reviewer's decisions that
// answer cost.
//
// Only the round's own entries are consulted, for the reason ClaimsOf scopes
// its pairs: §9.3.5 makes an earlier round history, and a set-aside recorded
// against one round is a judgement about the diff that round was formed from.
func Gaps(claims []string, pairs []Pair, round int, recorded []Gap) ([]*Gap, []DroppedSetAside) {
	mapped := make(map[string]bool, len(pairs))
	for i := range pairs {
		if pairs[i].Round == round {
			mapped[pairs[i].Claim] = true
		}
	}
	aside := make(map[string]string, len(recorded))
	for i := range recorded {
		if recorded[i].Round == round {
			aside[recorded[i].Claim] = recorded[i].SetAsideNote
		}
	}
	gaps := make([]*Gap, 0, len(claims))
	for _, id := range claims {
		if !mapped[id] {
			gaps = append(gaps, &Gap{Claim: id, SetAsideNote: aside[id]})
		}
	}
	return gaps, dropped(recorded, gaps, round)
}

// DroppedSetAside is one §4.1.8 stamp this derivation did not carry forward:
// the claim it stood against, and the note it named.
//
// It is reported rather than returned silently because a set-aside is the
// reviewer's judgement, not cr's, and §10.2.3 reads it as the one thing that
// stops an unimplemented claim blocking completeness. Round 9's
// `derived-state-clobbers-decision` asks for the drop to be reported for
// exactly that reason: dropping the stamp is correct when the claim no longer
// needs it, but a correct drop that happens in silence is indistinguishable
// from the clobber round 12's `derived-file-erases-recorded-decision` names.
//
// It is not a record and is never written to a file. The entry it stood on is
// gone by construction, so there is nowhere in intent-gaps.ndjson to put it;
// what carries it is the result of the run that dropped it.
type DroppedSetAside struct {
	// Claim is the claim id whose stamp was dropped.
	Claim string `json:"claim"`
	// SetAsideNote is the note id the dropped stamp named.
	SetAsideNote string `json:"set_aside_note"`
}

// dropped is the round's recorded set-asides that no entry of this derivation
// carries any more, in the order the file recorded them.
//
// It is computed from the derived entries rather than from the mapping, which
// makes it exactly the set of stamps this run lost. The case §4.1.7 is about is
// a claim that has since become mapped and so raises no entry at all; a claim
// the round no longer has — §3.3.1 replaces the round's claims and clears the
// mapping — reaches the same report, because the loss to the reviewer is the
// same one and a stamp that vanished for a second reason would otherwise vanish
// in silence.
//
// An empty note is not a stamp. §4.1.3 writes the field on every entry, so a
// claim nobody has judged carries an empty one, and reporting it dropped would
// report a decision that was never made.
func dropped(recorded []Gap, gaps []*Gap, round int) []DroppedSetAside {
	// A claim that still raises an entry kept its stamp, by construction of
	// the loop above: the entry is built with the note the round recorded
	// against the claim. So raising an entry is the whole test, and nothing
	// here re-reads the note to decide it.
	raised := make(map[string]bool, len(gaps))
	for _, gap := range gaps {
		raised[gap.Claim] = true
	}
	lost := make([]DroppedSetAside, 0, len(recorded))
	for i := range recorded {
		if recorded[i].Round == round && recorded[i].SetAsideNote != "" && !raised[recorded[i].Claim] {
			lost = append(lost, DroppedSetAside{
				Claim:        recorded[i].Claim,
				SetAsideNote: recorded[i].SetAsideNote,
			})
		}
	}
	return lost
}
