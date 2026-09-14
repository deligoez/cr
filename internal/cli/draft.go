package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// draftResult is what `cr draft` has to report: where the draft was written,
// which round it belongs to, and how many records it holds.
//
// The blocks themselves are not handed back. §7.1 writes them to a file the
// reviewer is about to open and edit, so printing them into the caller's
// terminal would repeat a document that already exists at a path — and the path
// is the one thing the caller cannot derive, since §2.2 puts it under the state
// root rather than beside the repository.
type draftResult struct {
	// Path is the rounds/<n>/draft.md the run wrote.
	Path string `json:"path"`
	// Round is the round it belongs to. §9.3.5 makes every round's
	// artefacts its own, so which one was written is part of the answer.
	Round int `json:"round"`
	// Queued is how many records §7.1 rendered into it, which is also how
	// many §9.1 now holds in `queued`.
	Queued int `json:"queued"`
	// Triaged are the records the draft this run replaced moved away from
	// `kept` through §7.2's verbs, each with §7.3.1's outcome and whether
	// §7.3.4 counts it against the class. A discard is a decision cr acted
	// on — the record is discarded and its waiver written — so the reviewer
	// is owed the list of what it acted on rather than a count.
	Triaged []triagedRecord `json:"triaged"`
	// Retriaged are the records whose stored fields §7.2's two editable
	// rows moved: the severity the reviewer chose, and the location they
	// moved the anchor to. They are listed for the reason the discards are
	// — cr acted on the value and the reviewer is owed what it acted on.
	Retriaged []retriagedRecord `json:"retriaged"`
	// Preserved are the records whose edited body §7.1.6 carried into the
	// new draft in place of the one cr renders.
	Preserved []string `json:"preserved"`
	// Forced is §6.3.2's count per class over the records this draft
	// holds, which is also what reached summary.json. It is a field on the
	// payload rather than a sentence alone, so an agent reading the
	// document gets the numbers and not only the prose.
	Forced finding.Forcings `json:"forced_to_question"`
	// Withdrawn is §3.6.6's count per class of the records this draft
	// holds as questions because the note their claim rests on no longer
	// stands, which is also what reached summary.json.
	Withdrawn finding.Withdrawn `json:"forced_by_retraction"`
	// NewClasses is §7.3.3's report for this round — the classes the
	// repository's ledger had not held before it — which is also what
	// reached summary.json.
	//
	// It is on the payload as well as in the file because §7.3.3 asks for
	// the drift to be visible rather than silent, and the moment a
	// reworded slug can still be caught cheaply is the run that raised it.
	NewClasses []string `json:"new_classes"`
	// Warnings are §8.2.3's, one per suggestion indented unlike the line it
	// replaces. They are warnings and not refusals: the reviewer is shown
	// both lines and decides, and a block left in place posts as written.
	Warnings []string `json:"warnings"`
}

// Text names the count, the round, and the file to open, then what the
// regeneration took from the draft it replaced, and then §6.3.2's forcing
// count and §3.6.6's.
//
// The forcing line is printed on every run, whatever the flags say. §11.1
// exempts it from `--quiet` by name, and a disclosure that is only printed
// sometimes is a disclosure the reader cannot rely on: it is how they tell a
// draft whose questions cr forced from one whose questions the agent chose.
func (r *draftResult) Text(w *writer) string {
	var text strings.Builder
	text.WriteString("drafted " + w.accent(strconv.Itoa(r.Queued)) + " record(s) for round " +
		strconv.Itoa(r.Round) + " to " + r.Path + "\n")
	for _, triaged := range r.Triaged {
		text.WriteString(triaged.ID + ": " + string(triaged.Outcome))
		if triaged.CountsAgainstClass {
			text.WriteString(", counted against its class")
		}
		text.WriteString("\n")
	}
	for _, edit := range r.Retriaged {
		text.WriteString(edit.ID + ": " + edit.moved() + "\n")
	}
	if len(r.Preserved) > 0 {
		text.WriteString("kept the edited body of: " + strings.Join(r.Preserved, ", ") + "\n")
	}
	for _, warning := range r.Warnings {
		text.WriteString(warning + "\n")
	}
	// §7.3.3's drift report, printed only when there is drift to report:
	// unlike the forcing line below it, an empty list is not a fact about
	// this draft that the reader needs on every run.
	if len(r.NewClasses) > 0 {
		text.WriteString("§7.3.3: class(es) first seen in this round: " +
			strings.Join(r.NewClasses, ", ") + "\n")
	}
	return text.String() + w.disclose("", "\n", r.Forced.Disclosure()) + w.disclose("", "", r.Withdrawn.Disclosure())
}

// newDraftCmd renders the editable draft (§11, §7.1).
func newDraftCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "draft " + prPlaceholder,
		Short: "Render the editable draft",
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
			// Briefed rather than ReadMeta, for the reason
			// `cr record` gives: §7.1 writes into rounds/<n>/, and
			// a pull request no round has been opened on has no
			// <n> to write into. §11.2 codes that 4.
			round, err := briefedRound(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			if err := refuseUnresolvedPost(&round.Meta); err != nil {
				return err
			}
			// §9.3.2: this command moves records into `queued` and
			// writes the round's draft.md, so a head that moved
			// under the round refuses here.
			if err := round.RefuseStale(); err != nil {
				return err
			}
			return produceDraft(out, layout, owner, repo, pr, &round.Meta)
		},
	}
}

// refuseUnresolvedPost is §8.4.4's UnresolvedPostError for `cr draft`, over a
// round whose send cr never learned the outcome of.
//
// That send carried the round's queued records, and a draft moves them: a
// deleted block or a `wrong` would store `discarded` for a record the review
// may already hold, and §9.1 has no row taking `discarded` to `posted`, so
// `cr post --reconcile` could never adopt that review and the flag would stay.
// It is refused as `cr post --confirm` is, before the draft is read and before
// anything is written, and the way forward is the same reconciliation. A round
// whose posting is settled drafts as it always did.
func refuseUnresolvedPost(round *state.Meta) error {
	if !round.PostUnresolved {
		return nil
	}
	return fmt.Errorf(
		"cr draft moves the round's records, and a send whose outcome cr never learned "+
			"may already have posted them: %w",
		&UnresolvedPostError{Owner: round.Owner, Repo: round.Repo, PR: round.PR, Round: round.Round},
	)
}

// produceDraft renders the round's draft, regenerating the one already there.
//
// The command settles the round's records first and writes once. §9.1's move
// into `queued` and §7.1's rendering are two halves of one answer — a record is
// queued because it was rendered — so a run that stamped the states and then
// failed to write the file would leave findings.ndjson claiming a draft that
// does not exist. Both reach disk under the one lock §2.3.1 requires.
//
// A regeneration ingests the draft it replaces before anything else, per
// §7.1.6, and every refusal below is made before anything is written: a
// refused run leaves findings.ndjson, draft.md, rendered.json, summary.json and
// the waivers exactly as they were, and the reviewer's edits are still in the
// file for the next run to read.
func produceDraft(out *writer, l state.Layout, owner, repo string, pr int, round *state.Meta) error {
	records, err := roundFindingsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return err
	}
	// §9.1.1: every move this run makes is kept here and published with
	// the records it changed.
	journal := finding.NewJournal(finding.ActorDraft, round.Head, time.Now())
	triage, err := ingestDraft(l, owner, repo, pr, round, records, journal)
	if err != nil {
		return err
	}
	// §6.2 over every record the triage moved, before §6.3's forcing reads
	// its grade: a record moved off its probe's target no longer asserts.
	grading, err := regradeMoved(l, owner, repo, pr, round, &triage)
	if err != nil {
		return err
	}
	queued, err := queueRecords(records, journal)
	if err != nil {
		return err
	}
	queued = retypeForDraft(queued, &triage.Triage)
	// §4.1.4 re-applied over the records this draft holds, before
	// §6.3's forcing and for the reason roundGrading.forceUnmapped
	// gives: the mapping moves inside a round, and an intent finding on a
	// unit the round no longer maps would otherwise assert in the draft
	// the reviewer approves.
	grading.forceUnmapped(round.Round, queued)
	// §3.6.6 over the same records, for the reason holdWithdrawn gives.
	held := grading.holdWithdrawn(queued)
	// §6.3.1's second moment, applied over the records this draft holds
	// and before they are rendered: the block a reviewer reads carries the
	// register in its marker, so a forcing applied after the rendering
	// would be a forcing the draft does not show.
	forced, moved := grading.forceQuestions(queued)
	// §6.3.3, over the records that are about to be written rather than
	// over the rule that was just applied. Invariant 4 is a claim about
	// what reaches the author, and nothing here can be talked past: no
	// flag, setting, environment variable, profile field or role
	// instruction is read on the way to this line, because there is none
	// to read.
	if err := finding.RefuseArguedAssertion(queued); err != nil {
		return err
	}
	// §8.1.3's refusal of a body, a preserved one included, is made before
	// anything is written.
	rendered, err := renderDraft(l, owner, repo, pr, round, queued, triage.Preserved)
	if err != nil {
		return err
	}
	// §8.2.3, over the records the draft is about to hold and before it is
	// written, so the reviewer reads the warning beside the file it is
	// about rather than after deciding what to do with it.
	warnings, err := indentationWarnings(owner, repo, pr, round, queued)
	if err != nil {
		return err
	}
	if err := waiveDiscards(l, owner, repo, pr, triage.discarded()); err != nil {
		return err
	}
	// §10.3's counts this command owns, read off the round as this run
	// leaves it and before §7.3.1's events below add this round's own.
	summary, err := summarizeDraft(l, owner, repo, pr, round, queued)
	if err != nil {
		return err
	}
	summary.forced, summary.moved, summary.withdrawn = forced, moved, held
	if err := publishDraft(l, owner, repo, pr, round, records, rendered, &summary, journal); err != nil {
		return err
	}
	// §7.3.1's events, after the draft they describe reached disk.
	if err := recordDraftTriage(l, owner, repo, pr, round, queued, &triage); err != nil {
		return err
	}
	return out.emit(&draftResult{
		Path:       l.RoundFile(owner, repo, pr, round.Round, state.FileDraft),
		Round:      round.Round,
		Queued:     len(queued),
		Triaged:    triage.report(),
		Retriaged:  triage.retriaged(),
		Preserved:  preservedIDs(queued, triage.Preserved),
		Forced:     forced,
		Withdrawn:  held,
		NewClasses: summary.newClasses,
		Warnings:   warnings,
	})
}

// drafted is one rendering of the round, as §7.1 has it reach disk: draft.md,
// and §7.1.5's rendered.json beside it.
type drafted struct {
	// file is draft.md.
	file string
	// rendered is rendered.json's entries, each record's agent region as
	// cr generated it.
	rendered map[string]string
}

// renderDraft is §7.1's draft.md and §7.1.5's rendered.json for the queued
// records, read out of everything they rest on before anything is written:
// §2.7's settings, §8.1.6's provenance sources, and §7.1.4's coverage state.
//
// preserved reaches draft.md and never rendered.json: §7.1.5 keeps a body
// §7.1.6 preserved out of its entry, and draft.Rendered takes no preserved
// bodies to put there.
func renderDraft(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
	queued []*finding.Finding, preserved map[string]string,
) (drafted, error) {
	settings, err := resolveDraftSettings(l, owner, repo)
	if err != nil {
		return drafted{}, err
	}
	// §8.1.6: what the provenance region names from outside the records.
	sources, err := draftProvenances(l, owner, repo, pr, round, queued)
	if err != nil {
		return drafted{}, err
	}
	sources.MaxProbeInput = settings.maxProbeInput
	rows, err := roundCoverage(l, owner, repo, pr, round)
	if err != nil {
		return drafted{}, err
	}
	file, err := draft.File(queued, settings.lang, sources, preserved, draft.HeaderFacts{
		MaxComments: settings.maxComments,
		Coverage:    rows,
	})
	if err != nil {
		return drafted{}, err
	}
	rendered, err := draft.Rendered(queued, settings.lang, sources)
	if err != nil {
		return drafted{}, err
	}
	return drafted{file: file, rendered: rendered}, nil
}

// settingMaxComments is §1.6.2's cap, by the key §2.7's table holds it under.
const settingMaxComments = "post.max_comments"

// settingMaxProbes is §5.6.4's cap, by the key §2.7's table holds it under.
const settingMaxProbes = "probe.max_per_round"

// draftSettings are the §2.7 settings a draft is rendered and summarised under.
type draftSettings struct {
	// lang is §8.1.1's `render.lang`, which every §8.1.4 label in the
	// draft is built in for.
	lang render.Lang
	// maxComments is §1.6.2's cap, which §7.1.4's header counts the
	// queued comments against.
	maxComments int
	// maxProbeInput is post.max_probe_input_bytes, the cap on the probe
	// input §8.1.7's evidence region carries.
	maxProbeInput int
	// maxProbes is §5.6.4's probe.max_per_round, which §10.3's probe cap
	// state is measured against.
	maxProbes int
}

// resolveDraftSettings reads both settings out of one resolution of §2.7's
// layers for this repository, so the header and the labels are rendered under
// the same configuration.
//
// The language is the only thing a layer can say about the label. §2.7 makes
// the label itself unreadable from every layer, and Resolve refuses a name
// addressing it before this function ever sees a value, so the text reaching
// the draft is always one row of the built-in table.
func resolveDraftSettings(l state.Layout, owner, repo string) (draftSettings, error) {
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: l.Config(),
		RepoConfig:   l.RepoConfig(owner, repo),
	})
	if err != nil {
		return draftSettings{}, err
	}
	lang, err := render.ParseLang(resolved.String(render.Setting))
	if err != nil {
		return draftSettings{}, err
	}
	return draftSettings{
		lang:          lang,
		maxComments:   resolved.Int(settingMaxComments),
		maxProbeInput: resolved.Int(render.MaxProbeInputSetting),
		maxProbes:     resolved.Int(settingMaxProbes),
	}, nil
}

// roundCoverage is the round's coverage state for §7.1.4's header: its units,
// the cells filled for them, and meta.json's active roles, counted per
// §10.1.1.
//
// The active set is meta.json's rather than a recomputation, for the reason
// activeRoles gives: `cr brief` settles it once per round, and a header that
// derived its own could count a row complete against roles the round never
// asked for.
func roundCoverage(l state.Layout, owner, repo string, pr int, round *state.Meta) (coverage.Rows, error) {
	formed, err := roundUnitsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return coverage.Rows{}, err
	}
	cells, err := state.ReadStamped[coverage.Cell](
		l, owner, repo, pr, state.FileCoverage, round.Round)
	if err != nil {
		return coverage.Rows{}, err
	}
	units := make([]unit.Unit, 0, len(formed))
	for i := range formed {
		units = append(units, formed[i].Unit)
	}
	return coverage.RowsOf(round.Round, units, round.ActiveRoles, cells), nil
}

// roundFindingsOf reads the records of one round out of findings.ndjson.
//
// The round is part of the question rather than context around it, exactly as
// roundUnitsOf has it: §9.3.5 has a command read only the current round's
// records, and a draft drawn from the whole file would re-render records a
// moved head already moved to `stale`.
func roundFindingsOf(
	l state.Layout, owner, repo string, pr, round int,
) ([]*finding.Finding, error) {
	return state.ReadStamped[*finding.Finding](l, owner, repo, pr, state.FileFindings, round)
}

// queueRecords walks §9.1's `draft` → `queued` row over the round's records and
// returns the ones §7.1 renders.
//
// The move is asked of the transition table rather than assumed, so this
// command writes a state only while §9.1 still has a row allowing it.
//
// A record already in `queued` is rendered again without a transition. §7.1.6
// makes the draft regenerable, so a second run is an ordinary thing to do, and
// §9.1's table has no `queued` → `queued` row for it to ask about — the record
// is where the row already put it. Every other state is left out of the draft
// entirely, which is the whole of what makes the file the set of open records
// rather than the set of records.
func queueRecords(records []*finding.Finding, journal *finding.Journal) ([]*finding.Finding, error) {
	queued := make([]*finding.Finding, 0, len(records))
	for _, record := range records {
		switch record.State {
		case finding.StateDraft:
			if err := journal.Move(
				record.ID, finding.Existing(finding.StateDraft), finding.StateQueued,
			); err != nil {
				return nil, err
			}
			record.State = finding.StateQueued
		case finding.StateQueued:
			// Already where the row put it, per §7.1.6.
		default:
			continue
		}
		queued = append(queued, record)
	}
	return queued, nil
}

// publishDraft is the single write, under §2.3.1's lock: the round's records
// carrying the states §9.1 just stamped, the draft they were rendered into,
// §7.1.5's rendered.json beside it, and the round-summary sections `cr draft`
// owns.
//
// findings.ndjson is replaced for the current round rather than appended to.
// The records being written are the ones just read back out of it, so an append
// would store every one of them a second time; §9.3.5 scopes the replacement to
// this round, so every earlier round's line survives byte for byte.
//
// rendered.json is replaced whole, and on the same run as draft.md, so the two
// always describe the same rendering: an entry for a record the draft no longer
// holds, or a draft block with no entry, would leave §7.1.6 comparing a body
// against the wrong thing or against nothing.
func publishDraft(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
	records []*finding.Finding, out drafted, summary *draftSummary, journal *finding.Journal,
) error {
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}
	writes := []func() error{
		func() error { return journal.Write(held) },
		func() error { return state.ReplaceStamped(held, state.FileFindings, stamp, records) },
		func() error { return held.WriteRound(round.Round, state.FileDraft, []byte(out.file)) },
		func() error { return writeRendered(held, round.Round, out.rendered) },
		func() error { return writeSummary(held, round.Round, ownerDraft, summary.counts()) },
		func() error { return writeSummary(held, round.Round, ownerDiscards, discardCounts(records)) },
		func() error { return keepForced(held, l, round, summary.moved) },
	}
	for _, write := range writes {
		if err := write(); err != nil {
			// The lock is released on the way out of every branch,
			// and the write's own failure is what the caller is
			// told about.
			_ = held.Unlock()
			return err
		}
	}
	return held.Unlock()
}

// writeRendered replaces the round's rendered.json with this rendering's
// entries, whatever the document held before: §7.1.5 has it describe the draft
// it was written beside, and an entry left over from an earlier rendering would
// describe a block that draft no longer has.
func writeRendered(held *state.Lock, round int, rendered map[string]string) error {
	return state.UpdateRoundJSON(held, round, state.FileRendered, func(doc *map[string]string) {
		*doc = rendered
	})
}

// draftSummary are the sections of the round's summary.json that `cr draft`
// owns, gathered into one value so publishDraft takes the document's shape
// rather than one parameter per field.
//
// §10.3 has each writer accumulate its own counts into one file, so what a
// command owns is a fixed, small set — and a type that names it is what keeps
// the next section from arriving as a ninth positional argument nobody at the
// call site can tell from the eighth. summaryOwners is where the set is stated.
type draftSummary struct {
	// forced is §6.3.2's count per class over the records this draft
	// holds.
	forced finding.Forcings
	// moved is the ids §6.3.1's second moment moved in this run, which
	// keepForced keeps beside the ones record time moved. It is not one of
	// counts' rows: ownerForcing writes it, not ownerDraft.
	moved []string
	// withdrawn is §3.6.6's count per class over the records this draft
	// holds as questions for resting on a note that no longer stands.
	withdrawn finding.Withdrawn
	// newClasses is §7.3.3's report for this round: the classes the
	// repository's ledger had not held before it. It is written on every
	// run, empty included, because silence about a reworded slug is what
	// §7.3.3 exists to stop.
	newClasses []string
	// drafted is how many records §7.1 rendered into the draft.
	drafted int
	// comments is §1.6.2's comment count against post.max_comments.
	comments summaryCap
	// probes is §5.6.4's probe cap over the round.
	probes summaryCap
}

// counts is the summary as the rows writeSummary takes.
func (s *draftSummary) counts() []summaryCount {
	return []summaryCount{
		{key: summaryForcedToQuestion, value: s.forced},
		{key: summaryForcedByRetraction, value: s.withdrawn},
		{key: summaryNewClasses, value: s.newClasses},
		{key: summaryDrafted, value: s.drafted},
		{key: summaryComments, value: s.comments},
		{key: summaryProbeCap, value: s.probes},
	}
}

// summaryForcedToQuestion is §10.3's "forced to question" count, by the key
// summary.json holds it under.
const summaryForcedToQuestion = "forced_to_question"

// summaryForcedByRetraction is §3.6.6's count of records held as questions for
// resting on a withdrawn note, by the key summary.json holds it under.
const summaryForcedByRetraction = "forced_by_retraction"

// summaryNewClasses is §7.3.3's newly seen classes, by the key summary.json
// holds them under.
const summaryNewClasses = "new_classes"

// summarizeDraft reads `cr draft`'s share of §10.3's counts off the round as
// this run leaves it. The forcing count is the caller's to fill, since it is
// the value §6.3.1's second moment just computed over the same records.
//
// Every count is taken from the round's state rather than from what this run
// changed. The two discard counts are not among them: they are ownerDiscards'
// section, which `cr post --confirm` writes too, and publishDraft writes it
// through discardCounts.
//
// §7.3.3's classes are read off the ledger as it stands before §7.3.1's events
// add this round's own: a class is new against what the repository knew, and a
// regeneration must not be the run that tells it the class is old.
//
// The probe cap is measured here and never enforced. §5.6.4's refusal is
// `cr probe run`'s, and a draft that refused on a spent budget would make the
// round undraftable once its last experiment had run.
func summarizeDraft(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
	queued []*finding.Finding,
) (draftSummary, error) {
	fresh, err := newDraftClasses(l, owner, repo, pr, round.Round, queued)
	if err != nil {
		return draftSummary{}, err
	}
	settings, err := resolveDraftSettings(l, owner, repo)
	if err != nil {
		return draftSummary{}, err
	}
	probes, err := state.ReadStamped[probe.Record](l, owner, repo, pr, state.FileProbes, round.Round)
	if err != nil {
		return draftSummary{}, err
	}
	commented := finding.CommentCapFor(queued, settings.maxComments)
	capped := probe.RoundCapFor(probes, round.Round, settings.maxProbes)
	summary := draftSummary{
		newClasses: fresh,
		drafted:    len(queued),
		comments:   summaryCap{Count: commented.Count, Max: commented.Max},
		probes:     summaryCap{Count: capped.Count, Max: capped.Max},
	}
	return summary, nil
}
