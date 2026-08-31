package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
)

// gapSupport is §5.4.4 and §5.4.5 read over one record `cr record` is storing:
// the gap probe it names, and whether that probe supports it.
//
// It is reported rather than left implicit because the answer is not one the
// agent can re-derive from the result. §5.4.4's two conditions are read off
// files the agent does not write — the baseline's own verdict in runs.ndjson,
// and §4.1.6's mapping — so a `failed` result and an unsupported record look
// from the outside exactly like a `failed` result and a supported one.
type gapSupport struct {
	// Record is the finding or question the probe was named by.
	Record string `json:"record"`
	// Probe is §5.5's id of the gap probe it names.
	Probe string `json:"probe"`
	// Result is that probe's §5.5 `result`, as it was recorded.
	Result string `json:"result"`
	// Supports is §5.4.4's answer: this probe may be what a `probed`
	// grade rests on, or it may not.
	Supports bool `json:"supports"`
	// Reason says which condition decided it, in the words §5.4.4 and
	// §5.4.5 use.
	Reason string `json:"reason"`
	// unmapped says §5.4.4's second condition is the one that decided,
	// which is the one condition §4.6.6 can make unreachable. It is not
	// reported: the reader is owed the reason, and this is which sentence
	// the reason was — a question the caller answers by construction
	// rather than by reading the prose back.
	unmapped bool
}

// gapEvidence is what one `cr record` invocation found about the gap probes its
// records rest on: the per-record answers, and §4.5.4's disclosure when the
// answer was settled before the experiment ran.
type gapEvidence struct {
	// support holds one entry per record naming a gap probe, in the order
	// the records arrived.
	support []gapSupport
	// unmappable is round 12's axis-availability-coupling, present when
	// §4.6.6's empty mapping is what decided a `failed` gap probe.
	unmappable []finding.HonestyDisclosure
}

// resolveGapSupport applies §5.4.4 and §5.4.5 to the records `cr record` is
// about to store.
//
// The three files it reads are the three §6.2.1 names for this question and no
// others: the probe record the finding references, the run record §5.5's
// `baseline` column has that probe point at, and §4.1.6's mapping. `evidence`
// prose is never parsed, and nothing here reads the summary, the class or the
// agent's own account of what the experiment showed.
//
// A record naming a probe that is not a gap probe is left alone. §5.3.5's
// conditions are a different question about a different ladder, and the
// mutation half of grading is the grade computation's to make.
func resolveGapSupport(
	l state.Layout, owner, repo string, pr int, meta *state.Meta, records []*finding.Finding,
) (*gapEvidence, error) {
	probes, err := state.ReadRecords[probe.Record](l, owner, repo, pr, state.FileProbes)
	if err != nil {
		return nil, err
	}
	runs, err := state.ReadRecords[run.Record](l, owner, repo, pr, state.FileRuns)
	if err != nil {
		return nil, err
	}
	pairs, err := state.ReadRecords[mapping.Pair](l, owner, repo, pr, state.FileMapping)
	if err != nil {
		return nil, err
	}
	found := &gapEvidence{
		support:    make([]gapSupport, 0, len(records)),
		unmappable: make([]finding.HonestyDisclosure, 0, 1),
	}
	unmapped := make([]string, 0, len(records))
	for _, record := range records {
		gap := gapProbeOf(probes, record.Probe)
		if gap == nil {
			continue
		}
		answered := answerGapSupport(gap, meta, runs, pairs, record)
		found.support = append(found.support, answered)
		if answered.unmapped {
			unmapped = append(unmapped, gap.ID)
		}
	}
	// §4.5.4: the coupling is disclosed only where it is what decided the
	// question. A repository with no tracker whose gap probes all passed
	// was told nothing by the mapping either way, and a disclosure there
	// would report a lens as blocked that nothing was asked of.
	if len(unmapped) > 0 && meta.IssueKey == "" {
		found.unmappable = append(found.unmappable, probe.GapUnmappable{Probes: unmapped})
	}
	return found, nil
}

// gapProbeOf returns the gap probe record the finding names, and nil when it
// names none, names a probe no record holds, or names a mutation probe.
//
// A `probe` field naming nothing is not refused here. §5.5.2 has a finding
// reference at most one probe by id and §6.2.2 rejects a record claiming
// `probed` without a valid same-head probe; both are the grading's to enforce,
// and a second refusal written into this reader would be a second reading of
// sections it does not own.
func gapProbeOf(probes []probe.Record, id string) *probe.Record {
	if id == "" {
		return nil
	}
	for i := range probes {
		if stored := &probes[i]; stored.ID == id && stored.Kind == probe.Gap {
			return stored
		}
	}
	return nil
}

// namedClaim is how a record's `claim` field is said in a refusal: the id when
// it has one, and the absence itself when it does not.
//
// The two are different faults and read differently. A record naming a claim
// the mapping does not join to its unit is one the agent can fix by mapping or
// by re-filing; a record naming no claim at all never met §5.4.4's condition in
// the first place, and a message that said the mapping lacked "" would send the
// reader to the wrong file.
func namedClaim(claim string) string {
	if claim == "" {
		return "the claim the record does not name"
	}
	return "claim " + claim
}

// answerGapSupport is §5.4.4 and §5.4.5 over one record and the gap probe it
// names.
//
// The head is asked first, per §5.5.3: a probe record from an earlier head must
// not be used to grade a finding in the current round, whatever its result was.
// §5.4.5's five results come next, because they settle the question without
// looking at anything else — the run itself said the probe establishes nothing
// a `probed` grade could rest on. Only `failed` reaches §5.4.4's two
// conditions.
func answerGapSupport(
	gap *probe.Record, meta *state.Meta, runs []run.Record, pairs []mapping.Pair,
	record *finding.Finding,
) gapSupport {
	answered := gapSupport{
		Record: record.ID, Probe: gap.ID, Result: string(gap.Result),
	}
	switch {
	case gap.Head != meta.Head:
		answered.Reason = fmt.Sprintf(
			"the probe ran at %s and this round's head is %s; §5.5.3 keeps a probe from "+
				"another head out of the current round's grading",
			gap.Head, meta.Head)
	case !probe.Reproduces(gap):
		answered.Reason = fmt.Sprintf(
			"§5.4.5: a %s gap probe supports no probed grade, so a record resting on it "+
				"stays argued (§6.2) and is asked as a question (§6.3)", gap.Result)
	default:
		answered.Supports, answered.unmapped, answered.Reason = failedGapSupport(
			gap, runs, pairs, meta.Round, record)
	}
	return answered
}

// failedGapSupport reads §5.4.4's two conditions over a failed gap probe.
//
// The baseline is asked before the claim because §5.4.4 writes them in that
// order and because a repository whose suite is already red makes every
// supplied test look like a finding — the same reason §5.3.5 asks it first.
//
// The sentence a supported probe carries says what §5.4.4 states and stops.
// Round 12's unearned-probed-grade finding is why it stops where it does: the
// claim condition establishes that the agent mapped some claim to this unit,
// never that the supplied test asserts that claim, and `target` is the
// `--target` the agent typed rather than anything cr derived. So the report
// says the record may rest on the probe, and leaves what the probe showed to
// the output tail §8.1.7 renders verbatim.
func failedGapSupport(
	gap *probe.Record, runs []run.Record, pairs []mapping.Pair, round int,
	record *finding.Finding,
) (supports, unmapped bool, reason string) {
	baseline, resolved := gap.ResolveBaseline(runs)
	switch {
	case !resolved:
		return false, false, fmt.Sprintf(
			"§5.2.6 admits no stored run as this probe's baseline %s, so §5.4.4's first "+
				"condition cannot be read", gap.Baseline)
	case !baseline.Passed():
		return false, false, fmt.Sprintf(
			"the baseline run %s did not pass per §5.2.5, so the suite was already red "+
				"when the supplied test failed", baseline.ID())
	}
	claim := probe.MapClaim(pairs, round, record.Claim, record.Unit)
	if !probe.Supports(gap, baseline, claim) {
		return false, true, fmt.Sprintf(
			"this round's mapping.ndjson does not join %s to unit %s, so §5.4.4's second "+
				"condition is unmet", namedClaim(record.Claim), record.Unit)
	}
	return true, false, fmt.Sprintf(
		"the baseline run %s passed per §5.2.5 and claim %s is mapped to unit %s, so §5.4.4 "+
			"lets the record rest on this probe; it establishes no more than that the "+
			"supplied test failed, which is either the behaviour or the test, and target %s "+
			"is the one the probe was given",
		baseline.ID(), record.Claim, record.Unit, gap.Target)
}
