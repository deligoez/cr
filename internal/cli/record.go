package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/rule"
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
	// Duplicates is how many of them §6.4.3 retired into state
	// `duplicate`, which is §10.1.6's duplicate count for this write.
	Duplicates int `json:"duplicates"`
	// Probes is §5.4's answer for every record resting on a gap probe:
	// whether that probe supports it, and which condition decided. It is
	// empty, and never nil, when no record names one.
	Probes []gapSupport `json:"probes"`
	// Honesty carries §4.5.4's disclosure when the intent axis being
	// unavailable is what kept a gap probe from supporting a finding,
	// rendered as the sentences §11.1 exempts from `--quiet`.
	Honesty []string `json:"honesty"`
}

// Text names how many records were stored and the states §9.1 brought them into,
// then what §5.4 made of every gap probe they rest on. The records themselves
// came from the caller's own file, so printing them into a terminal would repeat
// what the caller already has; what §9.1 and §5.4 made of them is not in that
// file.
//
// The states are named separately once any record was retired as a duplicate.
// §6.4.3 keeps a suppressed duplicate rather than dropping it, so a line saying
// every stored record is in `draft` would be counting records the round will
// never draft.
func (r *recordResult) Text(w *writer) string {
	var out strings.Builder
	stored := w.accent(strconv.Itoa(len(r.Recorded)))
	if r.Duplicates == 0 {
		out.WriteString("recorded " + stored + " in state " + finding.StateDraft.String())
	} else {
		fmt.Fprintf(&out, "recorded %s: %d in state %s, %d in state %s",
			stored, len(r.Recorded)-r.Duplicates, finding.StateDraft,
			r.Duplicates, finding.StateDuplicate)
	}
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
//
// The duplicate count is read off the stored records rather than passed in, so
// it counts what §9.1 actually stamped and not what a caller believed §6.4.3 had
// marked.
func newRecordResult(records []*finding.Finding, found *gapEvidence) *recordResult {
	honesty := make([]string, 0, len(found.unmappable))
	for _, entry := range found.unmappable {
		honesty = append(honesty, entry.Disclosure())
	}
	duplicates := 0
	for _, record := range records {
		if record.State == finding.StateDuplicate {
			duplicates++
		}
	}
	return &recordResult{
		Recorded: records, Duplicates: duplicates, Probes: found.support, Honesty: honesty,
	}
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

// stampStates walks §9.1's first two rows over a file `cr record` has accepted.
//
// Every record is created in `draft` by the row naming `cr record` as the
// producer. A record §6.4.3 marked as a suppressed duplicate is then moved on to
// `duplicate` by the second row, which names the same actor. §6.5.1 assigns that
// second move here and not to `cr merge`: merge's output carries `duplicate_of`
// and no `state` at all, and `cr record` is what applies §6.4.3 from it.
//
// Both moves are asked of the transition table rather than assumed, so this
// command writes a state only while §9.1 still has a row allowing it.
func stampStates(records []*finding.Finding) error {
	for _, record := range records {
		if err := finding.MayTransition(
			record.ID, finding.Creation, finding.StateDraft, finding.ActorRecord,
		); err != nil {
			return err
		}
		record.State = finding.StateDraft
		if record.DuplicateOf == "" {
			continue
		}
		if err := finding.MayTransition(
			record.ID, finding.Existing(finding.StateDraft), finding.StateDuplicate, finding.ActorRecord,
		); err != nil {
			return err
		}
		record.State = finding.StateDuplicate
	}
	return nil
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
			records, found, err := acceptRecords(
				layout, owner, repo, pr, &round, args[1])
			if err != nil {
				return err
			}
			if err := appendRecords(layout, owner, repo, pr, &round, records); err != nil {
				return err
			}
			if err := recordRuleStats(layout, owner, repo, pr, &round, records); err != nil {
				return err
			}
			return out.emit(newRecordResult(records, found))
		},
	}
}

// acceptRecords reads the file the agent handed the command and settles
// everything §6 has to say about it before a lock is taken: §6.1.3's and
// §6.1.4's refusals, §9.1's first two rows, §6.2.3's resolution, §5.4's
// severity bounds, and §6.2's grade.
//
// It is the whole of the validation, in one place, so the ordering above is a
// contract rather than the shape a command body happens to have. Every refusal
// here happens with nothing written, and the grade is computed last because it
// rests on what the two before it established.
func acceptRecords(
	l state.Layout, owner, repo string, pr int, round *state.Meta, file string,
) ([]*finding.Finding, *gapEvidence, error) {
	formed, err := roundUnitsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return nil, nil, err
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, err
	}
	// §6.5.1: the input is `cr merge`'s output, which is the one file
	// allowed to carry `duplicate_of` — §6.4.3 has `cr merge` mark a
	// suppressed duplicate there and `cr record` apply it from there.
	// Every other computed field is refused whatever the source says.
	records, err := finding.Decode(file, body, roundUnitIDs(formed), finding.SourceMerge)
	if err != nil {
		return nil, nil, err
	}
	// §9.1's first two rows, in the order stampStates walks them.
	if err := stampStates(records); err != nil {
		return nil, nil, err
	}
	// §6.2.3: every citation is resolved against the round's head and
	// stamped with the hash of the line it names, and one the head cannot
	// open refuses the whole file.
	if err := resolveCitations(file, body, round.Head, records); err != nil {
		return nil, nil, err
	}
	// §6.2.1's stored inputs, read off files no lock is needed for
	// (§2.3.2).
	evidence, err := readRoundEvidence(l, owner, repo, pr)
	if err != nil {
		return nil, nil, err
	}
	// §5.4.4 and §5.4.5. They are asked of the records rather than of the
	// probes because the question is about a finding: a gap probe's support
	// is conditional on the record's own `claim`, which is why `cr probe
	// run` cannot answer it at the experiment.
	found, err := resolveGapSupport(evidence, round, records)
	if err != nil {
		return nil, nil, err
	}
	// §6.1's `axis` row, before the grade rather than after it: §6.2's
	// `cited` row reads the axis, and §4.4.2 withholds that grade from the
	// test axis entirely.
	corpus, err := role.Resolve(l.RepoRolesDir(owner, repo), l.RolesDir())
	if err != nil {
		return nil, nil, err
	}
	stampAxes(corpus, records)
	// §6.2, last of them all, because it rests on what the others
	// established: the citations are resolved and stamped, the axis is
	// computed, and §5.4's bounds have already refused what they refuse.
	gradeRecords(round, formed, evidence, records)
	// §6.3.1's first of three moments, and invariant 4: a record graded
	// argued is forced to kind question. It is last because it reads the
	// grade, and it is here rather than in a role's instructions because
	// there is no wording that reaches a field assignment.
	//
	// The count it hands back is §6.3.2's, and this moment is not where
	// §6.3.2 is answered: §10.3 has `cr merge`, `cr draft` and `cr post`
	// accumulate the round summary, and `cr record` is none of the three.
	// The forcing is what is wanted here.
	finding.ForceQuestions(records)
	return records, found, nil
}

// appendRecords is the single write, under §2.3.1's lock: the accepted records
// reach findings.ndjson whole, stamped with the round's head and round.
func appendRecords(
	l state.Layout, owner, repo string, pr int, round *state.Meta, records []*finding.Finding,
) error {
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}
	if err := state.AppendStamped(held, state.FileFindings, stamp, records); err != nil {
		// The lock is released on the way out of every branch, and
		// the write's own failure is what the caller is told about.
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}

// recordRuleStats is `cr record`'s share of §2.6.1.6: a record event for every
// record it stored that names a rule, and a dismissal for every hit of the
// round that no record of the round confirms — round 9's
// rule-stats-event-producer, which names this command as the observable moment
// §2.6.1.5's drop otherwise lacks.
//
// It runs after the records are stored and reads the round's records back, so
// a hit an earlier `cr record` of the round confirmed counts as confirmed here
// too. The read takes no lock, per §2.3.2.
func recordRuleStats(
	l state.Layout, owner, repo string, pr int, round *state.Meta, recorded []*finding.Finding,
) error {
	stored, err := state.ReadRecords[finding.Finding](l, owner, repo, pr, state.FileFindings)
	if err != nil {
		return err
	}
	ofRound := make([]*finding.Finding, 0, len(stored))
	for i := range stored {
		if stored[i].Round == round.Round {
			ofRound = append(ofRound, &stored[i])
		}
	}
	return rule.RecordRecords(l, owner, repo, recorded, ofRound, &rule.Occasion{
		PR: pr, Round: round.Round, Head: round.Head, At: time.Now(),
	})
}
