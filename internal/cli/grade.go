package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/probe"
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
// must open and the decode counted blank lines to get there.
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
			unitOf(formed, record.Unit), referenced, meta.Head, baseline, claim,
		))
	}
}
