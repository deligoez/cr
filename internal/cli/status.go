package cli

import (
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/reinvention"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/symbol"
	"github.com/deligoez/cr/internal/testadequacy"
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
// It answers §10.1.1 through §10.1.3 and stops there. §10.1.4 to §10.1.6 —
// findings and questions by state, probes, waivers and suppressed duplicates —
// and §10.2's completeness verdict are the next tasks' to add here, and a
// report that guessed at them now would be a report of counts nothing computes.
type statusResult struct {
	// Round and Head are the round this report is about.
	Round int    `json:"round"`
	Head  string `json:"head"`
	// Coverage is §10.1.1, counted by coverage.RowsOf.
	Coverage coverage.Rows `json:"coverage"`
	// Intent is §10.1.2.
	Intent intentCoverage `json:"intent"`
	// Axes is §10.1.3's first clause and two of §4.5.4's four kinds: the
	// active axes, the disabled ones, and the unavailable ones.
	Axes activation.Activation `json:"axes"`
	// Skipped is §4.5.4's fourth kind, §4.6.4's roles.
	Skipped []coverage.SkippedRole `json:"skipped_roles"`
	// Honesty carries §9.3.1's comparison of the round's head against the
	// pull request's current one, and then every entry of §4.5.4's report,
	// rendered as the sentences §11.1 exempts from `--quiet`.
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

// Text prints §10.1's three sections in the order §10.1 numbers them, then the
// honesty channel.
func (r *statusResult) Text(w *writer) string {
	var out strings.Builder
	out.WriteString("round " + w.accent(strconv.Itoa(r.Round)) + " at " + r.Head + "\n")
	out.WriteString("units: " + strconv.Itoa(r.Coverage.Units) + " total, " +
		strconv.Itoa(r.Coverage.Complete) + " with a complete row of " +
		strconv.Itoa(r.Coverage.Roles) + " active role(s), " +
		strconv.Itoa(r.Coverage.Gaps) + " with gaps, " +
		strconv.Itoa(r.Coverage.Oversized) + " oversized\n")
	out.WriteString("claims: " + strconv.Itoa(r.Intent.Claims) + " total, " +
		strconv.Itoa(r.Intent.Mapped) + " mapped to a unit, " +
		strconv.Itoa(len(r.Intent.Gaps)) + " unimplemented (" +
		strconv.Itoa(r.Intent.SetAside) + " set aside)\n")
	for i := range r.Intent.Gaps {
		out.WriteString("  " + r.Intent.Gaps[i].Claim + gapNote(&r.Intent.Gaps[i]) + "\n")
	}
	out.WriteString("axes active: " + axisList(r.Axes.Active) + "\n")
	for _, entry := range r.Honesty {
		out.WriteString(entry + "\n")
	}
	return strings.TrimRight(out.String(), "\n")
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

// statusOf assembles §10.1.1 through §10.1.3 for one round.
func statusOf(
	l state.Layout, owner, repo string, pr int, round *state.Round,
) (*statusResult, error) {
	rows, err := roundCoverage(l, owner, repo, pr, &round.Meta)
	if err != nil {
		return nil, err
	}
	covered, err := intentCoverageOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return nil, err
	}
	axes, lenses, err := lensesOf(l, owner, repo, pr, &round.Meta)
	if err != nil {
		return nil, err
	}
	return &statusResult{
		Round:    round.Round,
		Head:     round.Head,
		Coverage: rows,
		Intent:   covered,
		Axes:     axes,
		Skipped:  lenses.Roles,
		Honesty:  statusHonesty(round, lenses),
	}, nil
}

// statusHonesty is the disclosure channel §11.1 exempts from `--quiet`:
// §9.3.1's comparison first, then every lens of §4.5.4 that did not run.
//
// The lenses are rendered through coverage.Lenses rather than appended kind by
// kind here, so a kind added to that collector reaches this report with it.
func statusHonesty(round *state.Round, lenses coverage.Lenses) []string {
	disclosed := lenses.Disclosures()
	honesty := make([]string, 0, 1+len(disclosed))
	honesty = append(honesty, round.Disclosure())
	for _, entry := range disclosed {
		honesty = append(honesty, entry.Disclosure())
	}
	return honesty
}

// intentCoverageOf counts §10.1.2 over the round's claims, its mapping, and
// the §4.1.3 entries derived from the two.
//
// The gaps are read rather than re-derived. §4.1.7 makes the derivation
// `cr map record`'s, and that is also where §4.1.8's set-aside stamps are
// carried forward — so a report deriving its own would show entries with no
// stamp on them and would contradict the file it is reporting about.
func intentCoverageOf(
	l state.Layout, owner, repo string, pr, round int,
) (intentCoverage, error) {
	claims, err := roundClaimIDs(l, owner, repo, pr, round)
	if err != nil {
		return intentCoverage{}, err
	}
	pairs, err := state.ReadRecords[mapping.Pair](l, owner, repo, pr, state.FileMapping)
	if err != nil {
		return intentCoverage{}, err
	}
	stored, err := state.ReadRecords[mapping.Gap](l, owner, repo, pr, state.FileIntentGaps)
	if err != nil {
		return intentCoverage{}, err
	}
	report := intentCoverage{Claims: len(claims), Gaps: make([]mapping.Gap, 0, len(stored))}
	for i := range stored {
		if stored[i].Round != round {
			continue
		}
		report.Gaps = append(report.Gaps, stored[i])
		if stored[i].SetAsideNote != "" {
			report.SetAside++
		}
	}
	report.Mapped = mappedClaims(claims, pairs, round)
	return report, nil
}

// mappedClaims is §10.1.2's middle number: how many of the round's claims the
// round's mapping maps to at least one unit.
//
// It counts claims and not pairs. §4.1.6 lets the agent map one claim to
// several units, and a count of pairs would report more of the issue covered
// than the round's claims can account for.
func mappedClaims(claims []string, pairs []mapping.Pair, round int) int {
	mapped := make(map[string]bool, len(pairs))
	for i := range pairs {
		if pairs[i].Round == round {
			mapped[pairs[i].Claim] = true
		}
	}
	count := 0
	for _, id := range claims {
		if mapped[id] {
			count++
		}
	}
	return count
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
	halves, err := roundHalves(owner, repo, pr, p, round.Head, resolved)
	if err != nil {
		return activation.Activation{}, coverage.Lenses{}, err
	}
	axes := roundAxes(p, round, resolved.String(intentKeyPattern))
	return axes, coverage.Lenses{
		Axes:   axes.Disclosures(),
		Halves: halves,
		Roles:  skippedRoles(axes, corpus, round),
	}, nil
}

// skippedRoles is §4.5.4's fourth kind: every role of the corpus the round left
// out, each carrying the reason it could not look.
//
// It is the negative of activation.ActiveRoles and it is derived rather than
// stored, because meta.json records the active set and §4.6.4 asks for the
// other one — "a role whose prerequisites are unmet MUST be reported as skipped
// with its reason, never omitted silently". A reader handed only the active set
// has to subtract two lists and then invent the reason, which is the silence
// that sentence forbids.
//
// The active set is meta.json's rather than a recomputation, for the reason
// roundCoverage gives: `cr brief` settles it once per round, and a report
// deriving its own could call a role skipped that the round counted active —
// which §10.2.2 then demands a cell for.
//
// The order is the corpus's, so a role's line here and its cells elsewhere are
// read in the one order §2.5.5 fixes.
func skippedRoles(
	axes activation.Activation, corpus []role.Resolved, round *state.Meta,
) []coverage.SkippedRole {
	out := make([]coverage.SkippedRole, 0, len(corpus))
	for i := range corpus {
		// Indexed rather than ranged by value: a Resolved carries a
		// whole Role, which is what gocritic's rangeValCopy is about.
		r := &corpus[i].Role
		if slices.Contains(round.ActiveRoles, r.ID) {
			continue
		}
		out = append(out, coverage.SkippedRole{Role: r.ID, Reason: skipReason(axes, r, round.ProfileID)})
	}
	return out
}

// skipReason says why one inactive role did not look.
//
// The axis is asked first because activation.ActiveRoles asks it first, so the
// reason a reader is given is the clause that actually decided. A role on an
// axis that did not run is out however its `profiles` list reads, and sending
// the reader to that list would send them to a fix that changes nothing.
//
// Where the axis did run, the axis's own entry is reused verbatim rather than
// reworded, so a reader acting on the role's line and one acting on the axis's
// line are sent to the same fix.
func skipReason(axes activation.Activation, r *role.Role, profileID string) string {
	if slices.Contains(axes.Active, r.Axis) {
		return "its profiles list names " + strings.Join(r.Profiles, ", ") +
			" and this round resolved profile " + namedProfile(profileID) +
			"; the role looks only under a profile it names"
	}
	for _, entry := range axes.Disclosures() {
		if strings.HasPrefix(entry.Disclosure(), "axis "+r.Axis+" ") {
			return entry.Disclosure()
		}
	}
	return "its axis " + r.Axis + " did not run this round"
}

// namedProfile names the profile the round resolved, and says plainly when
// there was none: §2.4.4's repository matched no profile, and an empty string
// in the sentence would read as a profile whose id is empty.
func namedProfile(profileID string) string {
	if profileID == "" {
		return "none, per §2.4.4"
	}
	return profileID
}

// intentKeyPattern is §3.2's `intent.key_pattern`, by the key §2.7's table
// holds it under. §4.5.3's reason names it, so the report has to read it.
const intentKeyPattern = "intent.key_pattern"

// roundAxes applies §4.5.1 to §4.5.3 to the round as meta.json recorded it.
//
// It is brief.axesOf's second reader and answers §2.4.4 the same way for the
// same reason: activation.Activate requires a resolved profile, so a round
// opened on a repository no profile matched has no axis decision to re-derive
// and what is still true is §4.5.3 alone. A second answer for that case would
// let `cr brief`'s report and this one disagree about which axes looked.
func roundAxes(p *profile.Profile, round *state.Meta, pattern string) activation.Activation {
	recorded := intent.Recorded(round.IssueKey, pattern)
	if round.ProfileID != "" {
		return activation.Activate(p, recorded)
	}
	axes := activation.Activation{
		Active:      []string{},
		Disabled:    []activation.Disabled{},
		Unavailable: []intent.Unavailable{},
	}
	if unavailable, marked := recorded.Unavailability(); marked {
		axes.Unavailable = append(axes.Unavailable, unavailable)
	}
	return axes
}

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
// Both are computed the way `cr review` computes them — the same index over the
// round's head, the same hunks, the same ranking — because §4.5.4's obligation
// is about the round rather than about the command asking. A shortcut reading
// only the profile would answer two of the reinvention half's three states and
// report the third, an index that was asked for and did not arrive, as a lens
// that ran.
func roundHalves(
	owner, repo string, pr int, p *profile.Profile, head string, resolved config.Config,
) ([]finding.HonestyDisclosure, error) {
	dir, err := repoDir()
	if err != nil {
		return nil, err
	}
	index, _, err := symbol.Head(dir, head, p)
	if err != nil {
		return nil, err
	}
	hunks, err := roundHunks(owner, repo, pr, head)
	if err != nil {
		return nil, err
	}
	candidates := reinvention.Attach(p, index, hunks, reinvention.Ranking{
		MinSimilarity: resolved.Float("reinvention.min_similarity"),
		MaxCandidates: resolved.Int("reinvention.max_candidates"),
	})
	tests := testadequacy.Attach(p, nil, hunks)
	out := make([]finding.HonestyDisclosure, 0,
		len(candidates.Unavailable)+len(tests.Unavailable))
	for _, entry := range candidates.Unavailable {
		out = append(out, entry)
	}
	for _, entry := range tests.Unavailable {
		out = append(out, entry)
	}
	return out, nil
}
