package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/run"
	"github.com/deligoez/cr/internal/state"
)

// roundEvidence is the stored half of §6.2.1's input set: the probe records a
// finding may reference, the run records §5.5's `baseline` column points into,
// and §4.1.6's mapping.
//
// The three are read once and handed to both readers of them, so §5.4.4's
// support and §6.2's grade are answered about the same files. Reading them
// twice would leave two answers that agree only while nothing writes between
// the reads, which is exactly the mutability round 13's mutable-grading-input
// finding is about.
//
// They are read without a lock, per §2.3.2, and before the write lock is taken,
// so the whole of §6.2.1's input is fixed before anything is stored.
type roundEvidence struct {
	// probes is probes.ndjson, whole. §5.5.3 decides which of them grade
	// in this round, and it is asked of each record rather than filtered
	// here, so a record naming a probe from another head is told that.
	probes []probe.Record
	// runs is runs.ndjson, which §5.2.6 admits baselines out of.
	runs []run.Record
	// pairs is mapping.ndjson: §4.1.6's claim-to-unit mapping.
	pairs []mapping.Pair
}

// readRoundEvidence reads the three files §6.2.1 names that a record does not
// carry.
func readRoundEvidence(l state.Layout, owner, repo string, pr int) (*roundEvidence, error) {
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
	return &roundEvidence{probes: probes, runs: runs, pairs: pairs}, nil
}

// referenced is the probe record §5.5.2 has this finding point at, of either
// kind, and nil when it names none or names one no record holds.
//
// It does not filter by kind, which is the difference between it and
// gapProbeOf. §5.4's severity bounds are about a gap probe alone and gapProbeOf
// says so by returning nil for anything else; §6.2's `probed` row is about
// both, and probe.Establishes is what routes the record to the ladder its own
// kind belongs to.
func (e *roundEvidence) referenced(id string) *probe.Record {
	if id == "" {
		return nil
	}
	for i := range e.probes {
		if stored := &e.probes[i]; stored.ID == id {
			return stored
		}
	}
	return nil
}

// resolveCitations applies §6.2.3 to every record `cr record` is about to
// store: each entry of `citations` is resolved against the current head, an
// entry the head cannot open is rejected with exit code 1, and an entry that
// resolves is stamped with the content hash of the line it names.
//
// It runs before anything is written, as §6.1.3's rejections do, so a file with
// one unopenable citation stores none of its records.
//
// The checkout is asked for only once some record carries a citation. `cr
// record` otherwise needs no repository at all — every other input is state cr
// wrote — and a command that resolved the working directory regardless would
// fail outside a checkout for a file that names no location.
//
// The line numbers come from state.RecordLines rather than from the records'
// position in the slice, because §6.2.3's rejection names the line the user
// must open and the decode counted blank lines to get there. The two slices are
// indexed together, which is total rather than lucky: both come from the one
// function that decides which lines of an NDJSON body carry a record, and
// state.DecodeStamped appends exactly one record for each line it names — a
// line it refuses returns an error instead, so records reaching here at all
// means every named line produced one.
func resolveCitations(file string, body []byte, head string, records []*finding.Finding) error {
	at := state.RecordLines(body)
	dir := ""
	for i, record := range records {
		if len(record.Citations) == 0 {
			continue
		}
		if dir == "" {
			checkout, err := repoDir()
			if err != nil {
				return err
			}
			dir = checkout
		}
		read := func(path string) ([]string, bool, error) {
			return git.FileAtRevision(dir, head, path)
		}
		if err := finding.ResolveCitations(read, file, at[i], record.Citations); err != nil {
			return err
		}
	}
	return nil
}

// roundGrading is everything §6.2.1 and §4.1.4 read about a round from outside
// the records themselves: its units, and roundEvidence's three files.
//
// §6.3.1 applies its forcing three times over one round and §7.2.2 recomputes
// every grade again at post time, so the two commands after `cr record` —
// `cr draft` and `cr post` — need the same inputs more than once in a single
// run. They are read once and handed to each application, for the reason
// roundEvidence gives about its own files: two reads are two answers that agree
// only while nothing writes between them, which is exactly the mutability round
// 13's mutable-grading-input finding is about.
type roundGrading struct {
	// formed are the round's units, as §3.4.6 recorded them.
	formed []roundUnit
	// roundEvidence is the stored half of §6.2.1's input set.
	*roundEvidence
	// withdrawn are the round's claim ids whose note no longer stands, as
	// withdrawnClaims read the context store for this run. They are not a
	// grading input — §6.2.1 closes that set — but they take the same
	// register away within a round, so they are read once beside it.
	withdrawn map[string]bool
	// forced are the ids an earlier moment of §6.3.1 moved from finding to
	// question, as the round summary kept them. A record they name is
	// stored as a question already, so forceQuestions counts it from here.
	forced []string
}

// readRoundGrading reads both halves, §3.6.6's withdrawn claims, and the ids
// §6.3.1 has moved, without a lock and before any write, as §2.3.2 has every
// read of cr's own state.
func readRoundGrading(l state.Layout, owner, repo string, pr int, round *state.Meta) (*roundGrading, error) {
	formed, err := roundUnitsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return nil, err
	}
	found, err := readRoundEvidence(l, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	withdrawn, err := withdrawnClaims(l, owner, repo, pr, round)
	if err != nil {
		return nil, err
	}
	forced, err := forcedRecords(l, round)
	if err != nil {
		return nil, err
	}
	return &roundGrading{formed: formed, roundEvidence: found, withdrawn: withdrawn, forced: forced}, nil
}

// forceQuestions is §6.3.1 applied over the records a draft or a payload holds,
// with §6.3.2's count per class taken over the records the round's forcing
// moved: the ones an earlier moment kept, and the ones this application moves,
// whose ids it returns for the caller to keep.
func (g *roundGrading) forceQuestions(records []*finding.Finding) (forced finding.Forcings, moved []string) {
	return finding.ForceQuestions(records, g.forced)
}

// holdWithdrawn is §3.6.6 applied over the records a draft or a payload holds:
// every one resting on a claim whose note was retracted or is no longer held
// is held as a question, and the count per class is returned for the report.
//
// The store was read when the command ran, so a retraction after `cr record`
// bites the next `cr draft` and the next `cr post` of the same round, and the
// record itself is retained so the decision stays auditable.
func (g *roundGrading) holdWithdrawn(records []*finding.Finding) finding.Withdrawn {
	return finding.ForceWithdrawn(records, g.withdrawn)
}

// regrade is §7.2.2's recomputation: every record's grade computed again from
// §6.2.1's inputs as they now stand, before the payload is built.
//
// It is the same computation `cr record` made, through the same ratchet, so a
// recomputation can lower a record to `argued` and never raise it within a
// round — which is what makes the forcing below meaningful at post time rather
// than a second chance to assert.
func (g *roundGrading) regrade(meta *state.Meta, records []*finding.Finding) {
	gradeRecords(meta, g.formed, g.roundEvidence, records)
}

// forceUnmapped is §4.1.4 applied again after record time.
//
// §4.1.6 replaces the mapping and §3.3.1 clears it, both inside a round, so a
// unit that was mapped when an intent finding was recorded can be unmapped by
// the time the draft is rendered or the payload built. A forcing applied only
// at record time would leave that finding asserting against a claim the round
// no longer maps to it — which is the finding intent-finding-on-unmapped-unit
// carried here, and the reason §6.3.1's own three moments are not enough.
func (g *roundGrading) forceUnmapped(round int, records []*finding.Finding) {
	forceUnmappedIntent(round, g.formed, g.pairs, records)
}

// unitOf is the unit a record sits on, as §3.4.6 recorded it for this round,
// and nil when the round holds no unit by that id.
//
// finding.Decode has already refused a record naming a unit the round does not
// hold, so nil is unreachable through `cr record`. It is returned rather than
// reported all the same, because a unit that is not there answers §6.2's
// containment question no better than one that is: finding.Evidence reads nil
// as containing nothing, which leaves every citation outside the record's own
// unit, and a grade reached that way would be the strongest the `cited` row can
// give for the least reason. Naming the case is what keeps it from being
// answered by accident.
func unitOf(formed []roundUnit, id string) finding.Containment {
	for i := range formed {
		if formed[i].ID == id {
			return &formed[i].Unit
		}
	}
	return nil
}

// stampAxes writes §6.1's `axis` row onto every record: the axis of the record's
// `role`, written by cr.
//
// It is stamped rather than accepted because §6.1.4 refuses the field on the
// wire, and §6.2's `cited` row turns on it — an agent that could write `axis`
// could name any axis but its own and buy the grade §4.4.2 withholds from the
// test axis. The value comes from §2.5.5's resolved corpus, which is the same
// corpus `cr cells record` reads a role's axis out of and the same one §6.4.2
// orders duplicate groups by.
//
// The whole corpus is read rather than §4.5.1's active set. Activation decides
// which roles a round asks for prompts from (§4.5.1) and which cells §4.5.6
// accepts; it does not decide what axis a role serves, and a record from a role
// this round did not activate is refused — if it is refused at all — for naming
// that role, not by being handed an axis it does not have.
//
// A role the corpus does not hold leaves the field empty, which is not an
// omission: §6.1.3 lists the rejections and an unresolvable role is not among
// them, and finding.ComputeGrade reads an axis cr never computed as one it
// cannot certify is not `test`. So the record keeps its grade honest instead of
// borrowing an axis.
func stampAxes(corpus []role.Resolved, records []*finding.Finding) {
	axes := make(map[string]string, len(corpus))
	for i := range corpus {
		axes[corpus[i].Role.ID] = corpus[i].Role.Axis
	}
	for _, record := range records {
		record.Axis = axes[record.Role]
	}
}

// gradeRecords writes §6.2's grade onto every record `cr record` is storing.
//
// The inputs are §6.2.1's and no others: the record's own `axis`, `unit`,
// `anchor`, `claim` and `citations`, the round's mapping, and the probe record
// the finding references. Nothing here reads `evidence`, `summary` or
// `severity`, and there is no argument through which a caller could offer an
// answer of its own — finding.Resolved takes a probe.Baseline and a
// probe.ClaimMapping, and neither can be built except out of the stored run
// §5.2.6 admits and the mapping §4.1.6 holds.
//
// It runs after resolveCitations, which is what makes §6.2's "that `cr`
// resolved against the current head" a fact about each entry rather than an
// assumption: an entry that resolved carries the hash cr stamped, and one that
// did not stopped the command.
func gradeRecords(
	meta *state.Meta, formed []roundUnit, found *roundEvidence, records []*finding.Finding,
) {
	for _, record := range records {
		referenced := found.referenced(record.Probe)
		var baseline probe.Baseline
		if referenced != nil {
			// §5.2.6's fence is inside ResolveBaseline: a run
			// carrying a probe is not one, whatever names it, and
			// an unresolved baseline stays the zero value, which
			// did not pass.
			baseline, _ = referenced.ResolveBaseline(found.runs)
		}
		claim := probe.MapClaim(found.pairs, meta.Round, record.Claim, record.Unit)
		finding.Regrade(record, finding.Resolved(
			unitOf(formed, record.Unit), &record.Anchor, referenced, meta.Head, baseline, claim,
		))
	}
}
