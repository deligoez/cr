package cli

import (
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/brief"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/reinvention"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/symbol"
	"github.com/deligoez/cr/internal/unit"
)

// intentCoverage is §10.1.2's half of the coverage report: how much of the
// issue the round's mapping accounts for, and what it does not.
//
// The gaps are listed and not counted, because this is the only place they
// appear. §4.1.3 keeps an unimplemented claim out of findings.ndjson and out of
// every draft — v0.1 has no unanchored comment channel per §1.6.1 — so a reader
// given a number here would have no second place to go and read which claims it
// stands for.
type intentCoverage struct {
	// Claims is how many claims the round recorded.
	Claims int `json:"claims"`
	// Mapped is how many of them §4.1.6's mapping maps to at least one
	// unit.
	Mapped int `json:"mapped"`
	// Gaps are §4.1.3's unimplemented-claim entries for the round, whole.
	Gaps []mapping.Gap `json:"gaps"`
	// SetAside is how many of those entries still carry the §4.1.8 stamp.
	//
	// It is the surviving half of §4.1.7's carry-forward, and it is
	// reported because `cr map record` reports only the other half: that
	// command names the stamps a re-derivation dropped, and nothing else
	// in cr says how many are still standing. §10.2.3 reads exactly these
	// entries as the ones that no longer block completeness, so a reviewer
	// checking whether their set-asides survived a re-recorded mapping has
	// one number to look at rather than a diff of two runs.
	SetAside int `json:"set_aside"`
}

// statusResult is §10.1's coverage report for one round.
//
// It answers §10.1.1 through §10.1.6, §3.6.6's report of what the round rests
// on that no longer stands, and §10.2's completeness verdict.
type statusResult struct {
	// Round and Head are the round this report is about.
	Round int    `json:"round"`
	Head  string `json:"head"`
	// Coverage is §10.1.1, counted by coverage.RowsOf.
	Coverage coverage.Rows `json:"coverage"`
	// Files is §3.4.2's excluded count and §3.4.7's listing, derived
	// again from the diff at the round's head by statusFiles.
	Files unit.Files `json:"files"`
	// Intent is §10.1.2.
	Intent intentCoverage `json:"intent"`
	// Axes is §10.1.3's first clause and two of §4.5.4's four kinds: the
	// active axes, the disabled ones, and the unavailable ones.
	Axes activation.Activation `json:"axes"`
	// Skipped is §4.5.4's fourth kind, §4.6.4's roles.
	Skipped []coverage.SkippedRole `json:"skipped_roles"`
	// Records is §10.1.4.
	Records recordReport `json:"records"`
	// Probes is §10.1.5.
	Probes probeReport `json:"probes"`
	// Unstanding is §3.6.6's report: the notes this round's cells and
	// records rest on that no longer stand, and what rests on each.
	//
	// It sits beside the §10.1 counts rather than inside Honesty because
	// §11.1 does not list it among the seven disclosures `--quiet` may
	// never suppress, and a report that quietly widened that list would be
	// making a claim about §11.1 rather than about the round.
	Unstanding []unstandingNote `json:"unstanding_notes"`
	// Completeness is §10.2's verdict and, when it is false, every
	// condition that did not hold.
	//
	// It is here as well as in Honesty's sentence for the reason Axes and
	// Skipped keep their fields: a reader of the JSON document should not
	// have to parse prose to learn whether the round is finished. The
	// sentence is not rendered from this field — §10.2 requires the verdict
	// to be printed together with every lens of §4.5.4 that did not run,
	// and coverage.Lenses.Verdict is the only thing that can say it, so
	// there is no way to print the claim without printing the list that
	// qualifies it.
	Completeness coverage.Completeness `json:"completeness"`
	// Honesty carries §9.3.1's comparison of the round's head against the
	// pull request's current one, then §10.2's verdict together with every
	// entry of §4.5.4's report, and then §10.1.6's waiver and duplicate
	// counts — rendered as the sentences §11.1 exempts from `--quiet`.
	//
	// The verdict is in this channel rather than beside it because §10.2
	// has it printed together with the lenses that did not run, and §11.1
	// is what keeps those printed: a verdict `--quiet` could suppress while
	// the lens list stayed would break the pairing from the other side.
	//
	// It is where §4.5.4's third kind lives and has no field of its own.
	// The halves of §4.3.1 and §4.4.1 are declared in two packages that
	// each word their own sentence — "per §4.3.1" against "per §4.4.1" —
	// and a field here would either hold the interface, which prints as
	// `null` the moment an entry is missing, or a flattened copy this file
	// reworded. §4.5.4 asks for the lens and its reason to appear with the
	// report, and the sentence each package derives from its own two
	// fields is that, said once.
	//
	// The axes and the roles keep their fields as well as their sentences
	// because §10.2 reads them as data: a completeness verdict counts
	// cells per active role, and a reader of the JSON document should not
	// have to parse prose to learn which roles those are.
	Honesty []string `json:"honesty"`
}

// Text prints §10.1's sections in the order §10.1 numbers them, then §3.6.6's
// report, then the honesty channel — which is where §10.2's verdict is said,
// beside the lenses that qualify it.
func (r *statusResult) Text(w *writer) string {
	var out strings.Builder
	out.WriteString("round " + w.accent(strconv.Itoa(r.Round)) + " at " + r.Head + "\n")
	out.WriteString("units: " + strconv.Itoa(r.Coverage.Units) + " total, " +
		strconv.Itoa(r.Coverage.Complete) + " with a complete row of " +
		strconv.Itoa(r.Coverage.Roles) + " active role(s), " +
		strconv.Itoa(r.Coverage.Gaps) + " with gaps, " +
		strconv.Itoa(r.Coverage.Oversized) + " oversized\n")
	out.WriteString(filesLines(&r.Files, ""))
	out.WriteString("claims: " + strconv.Itoa(r.Intent.Claims) + " total, " +
		strconv.Itoa(r.Intent.Mapped) + " mapped to a unit, " +
		strconv.Itoa(len(r.Intent.Gaps)) + " unimplemented (" +
		strconv.Itoa(r.Intent.SetAside) + " set aside)\n")
	for i := range r.Intent.Gaps {
		out.WriteString("  " + r.Intent.Gaps[i].Claim + gapNote(&r.Intent.Gaps[i]) + "\n")
	}
	out.WriteString("axes active: " + axisList(r.Axes.Active) + "\n")
	out.WriteString(r.recordLines())
	out.WriteString(r.noteLines())
	out.WriteString(w.disclose("", "\n", r.Honesty...))
	return strings.TrimRight(out.String(), "\n")
}

// recordLines is §10.1.4 and §10.1.5 for the terminal: the record total with
// its three tallies, then the probe counts.
//
// The tallies are printed whole, zeros included, because that is what the
// document holds and §12.1 gives the two shapes one payload: a terminal reader
// shown only the non-empty buckets would be reading a different report from the
// one a piped reader parses.
func (r *statusResult) recordLines() string {
	var out strings.Builder
	out.WriteString("records: " + strconv.Itoa(r.Records.Total) + " total\n")
	out.WriteString(tallyLine("state", r.Records.ByState) + "\n")
	out.WriteString(tallyLine("severity", r.Records.BySeverity) + "\n")
	out.WriteString(tallyLine("grade", r.Records.ByGrade) + "\n")
	out.WriteString("probes: " + strconv.Itoa(r.Probes.Run) + " run, " +
		strconv.Itoa(r.Probes.Graded) + " standing behind a graded record\n")
	return out.String()
}

// noteLines is §3.6.6's report for the terminal, and nothing at all when every
// note the round rests on still stands: an empty heading would read as a
// retraction nobody made.
func (r *statusResult) noteLines() string {
	if len(r.Unstanding) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("notes no longer standing, per §3.6.6: " +
		strconv.Itoa(len(r.Unstanding)) + "\n")
	for i := range r.Unstanding {
		out.WriteString(unstandingLine(&r.Unstanding[i]) + "\n")
	}
	return out.String()
}

// gapNote names the §4.1.8 note a gap entry was set aside by, and nothing at
// all when it carries none: §4.1.3 writes the field on every entry, so an empty
// one is a claim nobody has judged rather than a set-aside without a note.
func gapNote(gap *mapping.Gap) string {
	if gap.SetAsideNote == "" {
		return ""
	}
	return " (set aside by " + gap.SetAsideNote + ")"
}

// axisList renders §10.1.3's active axes, saying plainly when there are none:
// an empty line would read as a report that was not produced.
func axisList(active []string) string {
	if len(active) == 0 {
		return "none"
	}
	return strings.Join(active, ", ")
}

// newStatusCmd registers §11's `cr status <pr>`: §10.1's coverage report.
//
// It carries no flag of its own. §10.2 has the verdict printed together with
// every lens of §4.5.4 that did not run, and §11.1 forbids `--quiet` to
// suppress that report, so there is nothing here for a flag to select: what
// `cr status` prints is fixed by the round rather than by the invocation.
//
// §9.3 makes it a reader and not a writer. It writes no per-PR state — every
// number below is counted out of files other commands wrote — so §9.3.2's
// refusal does not reach it and §9.3.1's report does: a moved head is disclosed
// and the command runs, because a report of where a round stands is exactly
// what a reader whose head has moved needs before they open the next one.
func newStatusCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "status " + prPlaceholder,
		Short: "Report coverage, record states, and completeness",
		Args:  prArgs(1),
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
			round, err := briefedRound(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			report, err := statusOf(layout, owner, repo, pr, &round)
			if err != nil {
				return err
			}
			return out.emit(report)
		},
	}
}

// statusOf assembles §10.1.1 through §10.1.6, and §3.6.6's report, for one
// round.
func statusOf(
	l state.Layout, owner, repo string, pr int, round *state.Round,
) (*statusResult, error) {
	rows, missing, err := statusCoverage(l, owner, repo, pr, &round.Meta)
	if err != nil {
		return nil, err
	}
	covered, unsettled, err := intentCoverageOf(l, owner, repo, pr, &round.Meta)
	if err != nil {
		return nil, err
	}
	axes, lenses, err := lensesOf(l, owner, repo, pr, &round.Meta)
	if err != nil {
		return nil, err
	}
	records, err := roundFindingsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return nil, err
	}
	probes, err := state.ReadStamped[probe.Record](
		l, owner, repo, pr, state.FileProbes, round.Round)
	if err != nil {
		return nil, err
	}
	unstanding, err := unstandingNotesOf(l, owner, repo, pr, &round.Meta, covered.Gaps)
	if err != nil {
		return nil, err
	}
	files, drift, err := statusFiles(l, owner, repo, pr, &round.Meta)
	if err != nil {
		return nil, err
	}
	verdict := coverage.Complete(&coverage.Conditions{
		HeadMoved:  round.Stale(),
		Rows:       rows,
		Missing:    missing,
		Intent:     intentPassOf(axes, covered.Claims, &round.Meta),
		Unstanding: unstandingCells(unstanding),
		Unsettled:  unsettled,
		Records:    records,
	})
	disclosed, err := statusHonesty(
		l, owner, repo, pr, round, lenses, records, verdict)
	if err != nil {
		return nil, err
	}
	return &statusResult{
		Round:        round.Round,
		Head:         round.Head,
		Coverage:     rows,
		Files:        files,
		Intent:       covered,
		Axes:         axes,
		Skipped:      lenses.Roles,
		Records:      recordTalliesOf(records),
		Probes:       probesOf(probes, records),
		Unstanding:   unstanding,
		Completeness: verdict,
		Honesty:      append(disclosed, drift...),
	}, nil
}

// statusHonesty is the disclosure channel §11.1 exempts from `--quiet`:
// §9.3.1's comparison first, then §10.2's verdict together with every lens of
// §4.5.4 that did not run, then §10.1.6's waiver and duplicate counts.
//
// The verdict and the lenses arrive as one block from coverage.Lenses.Verdict,
// and that is the whole of why §10.2's sentence is reachable nowhere else:
// §10.2 requires a verdict of complete to be printed together with every lens
// that did not run — completeness across three axes is not completeness across
// four, and neither is completeness with a role that never looked — and a
// renderer holding only the verdict could print one without the list, which
// reads as a stronger claim than the round supports. Rendering the lenses
// through that collector also keeps the property statusHonesty had before:
// a kind added to it reaches this report with it.
//
// §10.1.6's two are appended after the block because §11.1 lists them as their
// own pair rather than as lenses, and each is a count over the round's records
// rather than a lens that did or did not look.
func statusHonesty(
	l state.Layout, owner, repo string, pr int,
	round *state.Round, lenses coverage.Lenses, records []*finding.Finding,
	verdict coverage.Completeness,
) ([]string, error) {
	said := lenses.Verdict(verdict.Complete, verdict.Reason())
	honesty := make([]string, 0, 3+len(said))
	honesty = append(honesty, round.Disclosure())
	honesty = append(honesty, said...)
	waived, err := waiverDisclosure(l, owner, repo, pr, round.Round)
	if err != nil {
		return nil, err
	}
	return append(honesty, waived, duplicateDisclosure(records)), nil
}

// statusCoverage is §10.1.1's counts, as roundCoverage gives `cr draft`'s
// header, together with the seats §10.2.2's reason names: both are taken from
// one read of the round's units and cells, so the count and the list cannot
// describe two different files.
func statusCoverage(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
) (coverage.Rows, []string, error) {
	formed, err := roundUnitsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return coverage.Rows{}, nil, err
	}
	cells, err := state.ReadStamped[coverage.Cell](
		l, owner, repo, pr, state.FileCoverage, round.Round)
	if err != nil {
		return coverage.Rows{}, nil, err
	}
	units := make([]unit.Unit, 0, len(formed))
	for i := range formed {
		units = append(units, formed[i].Unit)
	}
	return coverage.RowsOf(round.Round, units, round.ActiveRoles, cells),
		coverage.MissingSeats(round.Round, units, round.ActiveRoles, cells), nil
}

// intentPassOf is what §4.6.5's intent pass has stored for the round, and nil
// when the round's intent axis is not active: §4.6.6 asks for no claims and no
// mapping then. The axes are lensesOf's, the ones the report prints.
func intentPassOf(axes activation.Activation, claims int, round *state.Meta) *coverage.IntentPass {
	if !slices.Contains(axes.Active, axis.Intent) {
		return nil
	}
	return &coverage.IntentPass{Claims: claims, Mapped: round.MappingRecorded()}
}

// intentCoverageOf counts §10.1.2 over the round's claims, its mapping, and
// the §4.1.3 entries derived from the two, and returns §10.2.3's blocking set
// beside them.
//
// The gaps are read rather than re-derived. §4.1.7 makes the derivation
// `cr map record`'s, and that is also where §4.1.8's set-aside stamps are
// carried forward — so a report deriving its own would show entries with no
// stamp on them and would contradict the file it is reporting about.
//
// §10.2.3's set comes back from here rather than from a reader of its own
// because it is settled by the same three files, read once: a second reader
// could report a claim mapped in §10.1.2 and blocking in §10.2 out of the same
// round. The issue key's notes are read beside them, because a set-aside
// settles an entry only while the note it rests on stands.
func intentCoverageOf(
	l state.Layout, owner, repo string, pr int, meta *state.Meta,
) (intentCoverage, []string, error) {
	round := meta.Round
	claims, err := roundClaimIDs(l, owner, repo, pr, round)
	if err != nil {
		return intentCoverage{}, nil, err
	}
	pairs, err := state.ReadStamped[mapping.Pair](l, owner, repo, pr, state.FileMapping, round)
	if err != nil {
		return intentCoverage{}, nil, err
	}
	stored, err := state.ReadStamped[mapping.Gap](l, owner, repo, pr, state.FileIntentGaps, round)
	if err != nil {
		return intentCoverage{}, nil, err
	}
	notes := make([]note.Note, 0)
	if meta.IssueKey != "" {
		if notes, err = note.Load(l, meta.IssueKey); err != nil {
			return intentCoverage{}, nil, err
		}
	}
	report := intentCoverage{Claims: len(claims), Gaps: make([]mapping.Gap, 0, len(stored))}
	for i := range stored {
		report.Gaps = append(report.Gaps, stored[i])
		if stored[i].SetAsideNote != "" {
			report.SetAside++
		}
	}
	mapped := mappedSet(pairs, round)
	report.Mapped = mappedClaims(claims, mapped)
	return report, unsettledClaims(claims, mapped, stored, notes), nil
}

// mappedSet is which of the round's claims its mapping maps to at least one
// unit. §9.3.5 scopes it to the round, because a pair written in an earlier
// round joined that round's claims to that round's units.
func mappedSet(pairs []mapping.Pair, round int) map[string]bool {
	mapped := make(map[string]bool, len(pairs))
	for i := range pairs {
		if pairs[i].Round == round {
			mapped[pairs[i].Claim] = true
		}
	}
	return mapped
}

// mappedClaims is §10.1.2's middle number: how many of the round's claims the
// round's mapping maps to at least one unit.
//
// It counts claims and not pairs. §4.1.6 lets the agent map one claim to
// several units, and a count of pairs would report more of the issue covered
// than the round's claims can account for.
func mappedClaims(claims []string, mapped map[string]bool) int {
	count := 0
	for _, id := range claims {
		if mapped[id] {
			count++
		}
	}
	return count
}

// unsettledClaims is §10.2.3's blocking set: the round's claims that its
// mapping maps to no unit and that §4.1.8 has not set aside on a note that
// still stands.
//
// Both halves are read, and neither is inferred from the other. §10.2.3 asks
// first that every claim be mapped, so a claim no pair names blocks whether or
// not §4.1.3 has raised an entry for it — a round whose mapping was never
// recorded has no entries at all, and a set derived from the entries alone
// would report it settled. And §4.1.8's stamp is what stops an entry blocking,
// so a claim carrying one is out of the set however little of it is
// implemented — while its note stands. §3.6.6 has what rests on a retracted
// note re-evaluated rather than silently retained, so a set-aside on one blocks
// again, and `cr status` names it under the note in its §3.6.6 report. notes
// MUST be the issue key's whole store, for the reason note.StandingOf gives.
func unsettledClaims(claims []string, mapped map[string]bool, gaps []mapping.Gap, notes []note.Note) []string {
	aside := make(map[string]bool, len(gaps))
	for i := range gaps {
		if gaps[i].SetAsideNote != "" && note.StandingOf(notes, gaps[i].SetAsideNote).Stands() {
			aside[gaps[i].Claim] = true
		}
	}
	blocking := make([]string, 0, len(claims))
	for _, id := range claims {
		if !mapped[id] && !aside[id] {
			blocking = append(blocking, id)
		}
	}
	return blocking
}

// lensesOf is §10.1.3: the round's axes, and §4.5.4's report of every lens that
// did not run.
//
// The axes are re-derived from what the round recorded — meta.json's profile
// and issue key — rather than resolved afresh. §2.4's selection reads the
// repository's marker files and §3.2's resolution reads the pull request's
// branch, title, and body, and both can have moved since the round was opened;
// a report drawn from them would be a report about a round nobody opened.
func lensesOf(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
) (activation.Activation, coverage.Lenses, error) {
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: l.Config(),
		RepoConfig:   l.RepoConfig(owner, repo),
	})
	if err != nil {
		return activation.Activation{}, coverage.Lenses{}, err
	}
	p, err := statusProfile(l, round.ProfileID)
	if err != nil {
		return activation.Activation{}, coverage.Lenses{}, err
	}
	corpus, err := role.Resolve(l.RepoRolesDir(owner, repo), l.RolesDir())
	if err != nil {
		return activation.Activation{}, coverage.Lenses{}, err
	}
	halves, err := roundHalves(l, owner, repo, pr, p, round, resolved)
	if err != nil {
		return activation.Activation{}, coverage.Lenses{}, err
	}
	axes := activation.OfRound(p, round.ProfileID, round.IssueKey, resolved.String(intentKeyPattern))
	return axes, coverage.RoundLenses(axes, halves, corpus, round.ActiveRoles, round.ProfileID), nil
}

// intentKeyPattern is §3.2's `intent.key_pattern`, by the key §2.7's table
// holds it under. §4.5.3's reason names it, so the report has to read it.
const intentKeyPattern = "intent.key_pattern"

// statusProfile loads the profile the round resolved, and the empty profile for
// §2.4.4's repository where none did.
func statusProfile(l state.Layout, id string) (*profile.Profile, error) {
	if id == "" {
		return &profile.Profile{}, nil
	}
	loaded, err := profile.Load(l.Profile(id))
	if err != nil {
		return nil, err
	}
	return &loaded, nil
}

// roundHalves is §4.5.4's third kind: the reinvention half of §4.3.1 and the
// symbol half of §4.4.1, each reported when it could not run.
//
// They are brief.Halves', the computation `cr brief` reports them from, over
// the same index at the round's head, the round's hunks, the configured
// ranking and the files of units.ndjson — the unit set `cr review` emits its
// prompts over — because §4.5.4's obligation is about the round rather than
// about the command asking.
func roundHalves(
	l state.Layout, owner, repo string, pr int, p *profile.Profile, round *state.Meta, resolved config.Config,
) ([]finding.HonestyDisclosure, error) {
	dir, err := repoDir()
	if err != nil {
		return nil, err
	}
	index, _, err := symbol.Head(dir, round.Head, p)
	if err != nil {
		return nil, err
	}
	hunks, err := halfHunks(owner, repo, pr, round.Head, index)
	if err != nil {
		return nil, err
	}
	units, err := state.ReadStamped[unit.Record](l, owner, repo, pr, state.FileUnits, round.Round)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(units))
	for i := range units {
		paths = append(paths, units[i].Path)
	}
	return brief.Halves(dir, round.Head, p, index, hunks, paths, reinvention.Ranking{
		MinSimilarity: resolved.Float("reinvention.min_similarity"),
		MaxCandidates: resolved.Int("reinvention.max_candidates"),
	})
}

// halfHunks reads the round's diff for the two halves above, and reads nothing
// when no index arrived.
//
// This is not the shortcut roundHalves rejects. The index is still built and
// still asked for; what is skipped is the diff it would have been compared
// against, in the one case where the comparison cannot happen. Measured on this
// tree: reinvention.Attach returns its unavailability before it reads a hunk,
// and testadequacy.HeadReferences answers a nil index with a nil References
// before it reads a hunk either — so indexReason always answers and referenced
// returns before it reads the paths testPaths built. The hunks reach neither
// answer, and roundHalves discards the Paths they produced.
//
// It matters because §8.4.3 gave this derivation a second reader. `cr post`
// composes the review body out of it, so a diff read here is a `gh` call and a
// checkout `cr post` needs — and requiring them on a round whose profile
// declares no symbols.lang would be requiring them for an answer the profile
// alone already settled.
func halfHunks(
	owner, repo string, pr int, head string, index *symbol.Index,
) ([]git.Hunk, error) {
	if index == nil {
		return nil, nil
	}
	return roundHunks(owner, repo, pr, head)
}
