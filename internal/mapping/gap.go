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
// `round` are §2.3.3's stamp, written by state.WriteStamped and never by the
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
//
// Only the round's own entries are consulted, for the reason ClaimsOf scopes
// its pairs: §9.3.5 makes an earlier round history, and a set-aside recorded
// against one round is a judgement about the diff that round was formed from.
func Gaps(claims []string, pairs []Pair, round int, recorded []Gap) []*Gap {
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
	return gaps
}
