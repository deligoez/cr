package probe

import (
	"strings"

	"github.com/deligoez/cr/internal/mapping"
)

// ClaimMapping is §5.4.4's second condition, answered against mapping.ndjson:
// does the finding's `claim` field name a claim mapped to the finding's unit?
//
// Its field is unexported and MapClaim is its only constructor, which is
// Baseline's shape for Baseline's reason. §5.4.4 puts the whole of the gap
// probe's support on two facts, and this is the one an agent is closest to: the
// `claim` field is the agent's to write, and a caller holding a bare bool could
// be handed one computed from the record's own word about which claim covers
// it. A value of this type can only have come from the round's stored mapping —
// §4.1.6's file, the one join the intent axis reads — so the answer Supports
// reads is the mapping's rather than the record's.
//
// It carries the answer and not the pairs, because §4.1.6 makes the mapping the
// agent's judgement and cr forms none of it: there is nothing here to search
// for a near match, a claim whose text resembles the finding's, or a unit that
// would have been mapped had the agent noticed. The question is closed and
// mechanical, and it is asked once.
type ClaimMapping struct {
	// mapped is §5.4.4's answer: the round's mapping holds the pair.
	mapped bool
}

// MapClaim asks the round's mapping whether claim is mapped to unit.
//
// pairs is mapping.ndjson as it stands, and round is the round the finding is
// being recorded in. The round is part of the question rather than context
// around it, for the reason roundUnitsOf gives: §3.4.6 makes a unit id
// round-scoped, so `u1` of round 2 is a different unit from `u1` of round 1,
// and a pair drawn from the whole file would license a probe on the strength of
// a mapping some earlier round made about other code.
//
// An empty claim needs no guard of its own. §4.1.6 rejects a pair naming an
// unknown claim or unit and §4.1.6's form is `{claim, unit}`, so no stored pair
// holds an empty id, and a record that names no claim matches nothing — which
// is §5.4.4's answer for it: a finding with no claim field has not named a
// claim mapped to its unit.
func MapClaim(pairs []mapping.Pair, round int, claim, unit string) ClaimMapping {
	for i := range pairs {
		// Indexed rather than ranged by value, as elsewhere: the pair
		// carries a stamp as well as its two ids.
		if pair := &pairs[i]; pair.Round == round &&
			pair.Claim == claim && pair.Unit == unit {
			return ClaimMapping{mapped: true}
		}
	}
	return ClaimMapping{}
}

// Reproduces is §5.4.4's first half: this gap probe's supplied test failed.
//
// That is the whole of what the run established. "A `failed` gap probe means
// either the behaviour is wrong or the supplied test is wrong, and `cr` cannot
// distinguish the two" — so what the record carries is the result and the
// output tail, offered to the author as a reproduction, and nothing here reads
// it as a verdict on the code. §8.1.7's evidence region prints both verbatim
// for the same reason.
//
// Whether the probe supports a finding is a second question, and Supports is
// where it is asked: it is asked of a finding, and no finding exists at the
// moment the experiment runs.
func Reproduces(record *Record) bool {
	return record.Kind == Gap && record.Result == resultFailed
}

// Present is §5.4.5's first sentence: this gap probe's supplied test passed, so
// the behaviour that test asserts is present at this head.
//
// It is a fact about the code and not about the suite, and §5.4.5 draws the line
// itself: a passing supplied test establishes the behaviour "not that the suite
// lacks a test — only §5.3's `no-test-failed` establishes that". The two look
// alike from a distance and are opposite in what they license, which is why the
// gap ladder's last two rungs read the other way round from the mutation
// ladder's.
//
// Supports asks nothing of this. §5.4.5 refuses a `probed` grade to `passed`
// exactly as it refuses one to `timeout`, `error`, `no-tests-selected` and
// `inconclusive`, and Reproduces already keeps all five out by naming the one
// result that is not among them. What this exists for is the report: a run that
// established something and a run that established nothing are different
// answers to the agent holding them, and §5.4.5 states the difference.
func Present(record *Record) bool {
	return record.Kind == Gap && record.Result == resultPassed
}

// Supports is §5.4.4's condition: this gap probe may be what a record graded
// `probed` rests on under §6.2.
//
// Three conditions and no others. The result has to be `failed`, the baseline
// run has to have passed per §5.2.5, and the finding's `claim` field has to
// name a claim mapped to the finding's unit. Otherwise the probe supports
// nothing, which leaves the record `argued` under §6.2 and a question under
// §6.3 — and §5.4.5 keeps the other five results out by the first condition
// alone, since none of them is `failed`.
//
// It reads the probe record rather than the Outcome Proves reads, and the
// difference is the two sections' own. §5.3.5's proof is settled by the run, so
// the mutation side can be asked at the moment Decide produces an Outcome.
// §5.4.4's support is settled against a finding that does not exist then, so
// this is asked later — of "the referenced probe record", which is the input
// §6.2.1 names for exactly this. The record's `result` is always the value
// Decide wrote (§5.5.1 makes it immutable afterwards), so §5.1.7 has had its
// say here as much as there: a voided probe arrives carrying `error`.
//
// The other two arguments are what makes the refusal structural rather than
// remembered. A Baseline exists only where a stored run record §5.2.6 admits
// was resolved, so the verdict read here is §5.2.5's on the run this probe was
// measured against; a ClaimMapping exists only where §4.1.6's stored mapping
// was asked. Neither can be assembled out of a value a caller preferred, and
// neither is a field the agent writes.
//
// What this does not establish is worth stating, because §5.4.4 states no more.
// It does not establish that the behaviour is wrong — the supplied test may be.
// It does not establish that the suite lacks a test; §5.4.5 says only §5.3's
// `no-test-failed` establishes that. And the mapping condition is a check that
// the agent mapped some claim to this unit, not that the supplied test asserts
// that claim, which is a judgement cr cannot make and §4.1.5 forbids it to make
// by matching text. The `target` the probe carries is the agent's own
// `--target` (§5.5), and it is §6.2.2's containment check that binds it to the
// record's anchor rather than anything here.
func Supports(record *Record, baseline Baseline, claim ClaimMapping) bool {
	return Reproduces(record) && baseline.Passed() && claim.mapped
}

// GapUnmappable is the honesty disclosure round 12's axis-availability-coupling
// finding requires.
//
// The coupling is between two sections that never mention each other. §4.5.3
// marks the intent axis unavailable when no issue key resolves; §4.6.6 then
// makes the mapping empty and neither requires nor accepts one; and §5.4.4's
// second condition is read off that mapping. So in a repository that has tests
// and no tracker, every gap probe runs, records its result, and can support no
// finding at all — however carefully the experiment was designed.
//
// That is a lens cr cannot fill, and §4.5.4's obligation is that such a lens
// appears with its reason rather than passing in silence. Saying nothing would
// leave the agent to conclude from a `failed` result and an `argued` record
// that its evidence was judged weak, when in fact the condition it failed was
// unreachable before it started.
//
// It satisfies finding.HonestyDisclosure without importing internal/finding,
// as intent.Unavailable and testadequacy.Unavailable do: the interface is the
// union §4.5.4's report collects over, and the disclosure implements it where
// the fact is known.
type GapUnmappable struct {
	// Probes are the ids of the gap probes the coupling reaches, in the
	// order the records naming them arrived. They are named because the
	// experiment is the thing the reader is holding: an id says which run
	// this is about, where a count would leave them to guess.
	Probes []string `json:"probes"`
}

// Disclosure is the sentence §11.1 exempts from `--quiet`. It is derived from
// the field, so what is printed and what a caller reads as data cannot drift.
func (g GapUnmappable) Disclosure() string {
	return "gap probe support unavailable, per §4.6.6: no issue key resolved, so §4.5.3 " +
		"marks the intent axis unavailable and the round's mapping is empty; §5.4.4 lets a " +
		"gap probe support a finding only when a claim is mapped to that finding's unit, so " +
		strings.Join(g.Probes, ", ") + " can support none, and every record resting on one " +
		"stays argued (§6.2) and is asked as a question (§6.3); " +
		"re-run `cr brief <pr> --issue <KEY>` to name the issue this pull request implements"
}
