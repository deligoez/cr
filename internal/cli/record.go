package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// recordResult is what `cr record` has to report: the records it stored, whole.
//
// They are printed rather than counted because cr has just written fields the
// agent could not: §6.1.4 reserves `state`, and §2.3.3 reserves head and round,
// so the record the agent handed in and the record findings.ndjson now holds
// are not the same document. Handing back what was stored is how the caller
// learns what cr made of its input without reading the state tree.
type recordResult struct {
	// Recorded are the records as they were written, in file order.
	Recorded []*finding.Finding `json:"recorded"`
	// Probes is §5.4's answer for every record resting on a gap probe:
	// whether that probe supports it, and which condition decided. It is
	// empty, and never nil, when no record names one.
	Probes []gapSupport `json:"probes"`
	// Honesty carries §4.5.4's disclosure when the intent axis being
	// unavailable is what kept a gap probe from supporting a finding,
	// rendered as the sentences §11.1 exempts from `--quiet`.
	Honesty []string `json:"honesty"`
}

// Text names how many records were stored and the state §9.1 brought them into,
// then what §5.4 made of every gap probe they rest on. The records themselves
// came from the caller's own file, so printing them into a terminal would repeat
// what the caller already has; what §5.4 answered is not in that file.
func (r *recordResult) Text(w *writer) string {
	var out strings.Builder
	out.WriteString("recorded " + w.accent(strconv.Itoa(len(r.Recorded))) +
		" in state " + finding.StateDraft.String())
	for _, answered := range r.Probes {
		fmt.Fprintf(&out, "\n  %s %s %s (%s): %s",
			answered.Record, w.accent(answered.Probe), supported[answered.Supports],
			answered.Result, answered.Reason)
	}
	for _, disclosed := range r.Honesty {
		fmt.Fprintf(&out, "\n%s", disclosed)
	}
	return out.String()
}

// newRecordResult renders what was stored together with what §5.4 made of the
// gap probes those records rest on.
//
// The disclosures are asked of the entry that owns them rather than assembled
// here, as `cr brief` asks its own: probe.GapUnmappable holds the sentence the
// coupling is reported in, so a wording built at the call site cannot come to
// disagree with the data beside it.
func newRecordResult(records []*finding.Finding, found *gapEvidence) *recordResult {
	honesty := make([]string, 0, len(found.unmappable))
	for _, entry := range found.unmappable {
		honesty = append(honesty, entry.Disclosure())
	}
	return &recordResult{Recorded: records, Probes: found.support, Honesty: honesty}
}

// supported is how §5.4.4's answer is said, in the section's own verb. It is
// deliberately not "proves" or "confirms": §5.4.4 lets a supported probe be what
// a `probed` grade rests on and says nothing stronger, and §6.2.4 already
// forbids cr to describe evidence it merely validated as verified or proven.
var supported = map[bool]string{
	true:  "supports the finding",
	false: "supports no probed grade",
}

// roundUnit is one line of units.ndjson: §3.4.6's unit record, and the head and
// round §2.3.3 stamps onto it.
//
// The unit is embedded whole rather than re-declared with the two fields this
// command reads. A second declaration of a stored record's shape is a second
// thing to keep in agreement with the writer, and it agrees silently — a field
// renamed on the way out decodes as a zero value here, and `cr record` would
// then check every record's `unit` against a set of empty ids rather than
// report anything.
type roundUnit struct {
	unit.Unit
	state.Stamp
}

// roundUnitsOf reads the units of one round out of units.ndjson.
//
// The round is part of the question rather than context around it. §3.4.6 makes
// a unit id round-scoped and forbids carrying it across rounds, so `u1` of round
// 2 is a different unit from `u1` of round 1, and a set drawn from the whole
// file would accept a record anchored in a unit this round never formed. §9.3.5
// says the same from the other side: a command reads only the current round's
// records.
//
// It is one reader for both commands that ask after the round's units — §6.1.3
// wants their ids and §4.5.5 wants their hashes as well — so `cr record` and
// `cr cells record` cannot come to disagree about which units a round formed.
func roundUnitsOf(l state.Layout, owner, repo string, pr, round int) ([]roundUnit, error) {
	stored, err := state.ReadRecords[roundUnit](l, owner, repo, pr, state.FileUnits)
	if err != nil {
		return nil, err
	}
	units := make([]roundUnit, 0, len(stored))
	for i := range stored {
		if stored[i].Round == round {
			units = append(units, stored[i])
		}
	}
	return units, nil
}

// roundUnitIDs is the set §6.1.3 checks a record's `unit` against: the ids of
// the units of the round this pull request is in.
func roundUnitIDs(units []roundUnit) []string {
	ids := make([]string, 0, len(units))
	for i := range units {
		ids = append(ids, units[i].ID)
	}
	return ids
}

// newRecordCmd records a round's merged findings (§11, §6.1.3, §9.1).
//
// The whole file is validated before anything is written, and that ordering is
// the command's contract rather than an implementation detail: §6.1.3 rejects a
// record with exit code 1, and a rejection that had already stored the lines
// above it would leave findings.ndjson holding half of a file the agent is
// about to correct and hand in again. finding.Decode refuses the first faulty
// line and returns no records at all, so the single append below either happens
// whole or does not happen — there is no partial write to roll back, and no
// rollback that could fail.
//
// Nothing here re-reads §6.1's table. finding.Decode is the one door: it holds
// every line to §6.1.3 and §6.1.4, names the file, the one-based line and the
// field in what it refuses, and internal/cli maps that onto §11.2's code 1. A
// second reading of the same section here would be a second thing to keep in
// agreement with the spec.
func newRecordCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "record " + prPlaceholder + " <file>",
		Short: "Record a round's merged findings",
		Args:  prArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pr, err := parsePR(args[0])
			if err != nil {
				return err
			}
			owner, repo, err := repoOf(cmd)
			if err != nil {
				return err
			}
			layout, err := state.Default()
			if err != nil {
				return err
			}
			// Briefed rather than ReadMeta: §6.1.3 checks every
			// record's unit against the units of the round, and
			// units.ndjson is `cr brief`'s to write per §3.7. A
			// pull request no round has been opened on has an
			// empty one, so every record would be refused for
			// naming an unknown unit rather than for the reason
			// it was actually refused. §11.2 codes that 4.
			round, err := layout.Briefed(owner, repo, pr)
			if err != nil {
				return err
			}
			formed, err := roundUnitsOf(layout, owner, repo, pr, round.Round)
			if err != nil {
				return err
			}
			units := roundUnitIDs(formed)
			body, err := os.ReadFile(args[1])
			if err != nil {
				return err
			}
			// §6.5.1: the input is `cr merge`'s output, which is the
			// one file allowed to carry `duplicate_of` — §6.4.3 has
			// `cr merge` mark a suppressed duplicate there and `cr
			// record` apply it from there. Every other computed
			// field is refused whatever the source says.
			records, err := finding.Decode(args[1], body, units, finding.SourceMerge)
			if err != nil {
				return err
			}
			// §9.1's first row: a new record enters `draft`, and the
			// move is asked of the table rather than assumed, so the
			// one command §9.1 lists as a producer is the one that
			// produces here.
			for _, record := range records {
				if err := finding.MayTransition(
					record.ID, finding.Creation, finding.StateDraft, finding.ActorRecord,
				); err != nil {
					return err
				}
				record.State = finding.StateDraft
			}
			// §5.4.4 and §5.4.5, read before the write and off
			// files no lock is needed for (§2.3.2). It is asked of
			// the records rather than of the probes because the
			// question is about a finding: a gap probe's support is
			// conditional on the record's own `claim`, which is why
			// `cr probe run` cannot answer it at the experiment.
			found, err := resolveGapSupport(layout, owner, repo, pr, &round, records)
			if err != nil {
				return err
			}
			held, err := layout.LockPR(owner, repo, pr)
			if err != nil {
				return err
			}
			stamp := state.Stamp{Head: round.Head, Round: round.Round}
			if err := state.AppendStamped(held, state.FileFindings, stamp, records); err != nil {
				// The lock is released on the way out of every
				// branch, and the write's own failure is what
				// the caller is told about.
				_ = held.Unlock()
				return err
			}
			if err := held.Unlock(); err != nil {
				return err
			}
			return out.emit(newRecordResult(records, found))
		},
	}
}
