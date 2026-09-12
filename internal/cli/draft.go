package cli

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
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
	// Warnings are §8.2.3's, one per suggestion indented unlike the line it
	// replaces. They are warnings and not refusals: the reviewer is shown
	// both lines and decides, and a block left in place posts as written.
	Warnings []string `json:"warnings"`
}

// Text names the count, the round, and the file to open, then what the
// regeneration took from the draft it replaced, and then §6.3.2's forcing
// count.
//
// The forcing line is printed on every run, whatever the flags say. §11.1
// exempts it from `--quiet` by name, and a disclosure that is only printed
// sometimes is a disclosure the reader cannot rely on: it is how they tell a
// draft whose questions cr forced from one whose questions the agent chose.
func (r *draftResult) Text(w *writer) string {
	text := "drafted " + w.accent(strconv.Itoa(r.Queued)) + " record(s) for round " +
		strconv.Itoa(r.Round) + " to " + r.Path + "\n"
	for _, triaged := range r.Triaged {
		text += triaged.ID + ": " + string(triaged.Outcome)
		if triaged.CountsAgainstClass {
			text += ", counted against its class"
		}
		text += "\n"
	}
	for _, edit := range r.Retriaged {
		text += edit.ID + ": " + edit.moved() + "\n"
	}
	if len(r.Preserved) > 0 {
		text += "kept the edited body of: " + strings.Join(r.Preserved, ", ") + "\n"
	}
	for _, warning := range r.Warnings {
		text += warning + "\n"
	}
	return text + r.Forced.Disclosure()
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
			round, err := layout.Briefed(owner, repo, pr)
			if err != nil {
				return err
			}
			return produceDraft(out, layout, owner, repo, pr, &round)
		},
	}
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
	triage, err := ingestDraft(l, owner, repo, pr, round, records)
	if err != nil {
		return err
	}
	queued, err := queueRecords(records)
	if err != nil {
		return err
	}
	queued = retypeForDraft(queued, &triage.Triage)
	// §6.3.1's second moment, applied over the records this draft holds
	// and before they are rendered: the block a reviewer reads carries the
	// register in its marker, so a forcing applied after the rendering
	// would be a forcing the draft does not show.
	forced := finding.ForceQuestions(queued)
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
	if err := publishDraft(l, owner, repo, pr, round, records, rendered, forced); err != nil {
		return err
	}
	return out.emit(&draftResult{
		Path:      l.RoundFile(owner, repo, pr, round.Round, state.FileDraft),
		Round:     round.Round,
		Queued:    len(queued),
		Triaged:   triage.report(),
		Retriaged: triage.retriaged(),
		Preserved: preservedIDs(queued, triage.Preserved),
		Forced:    forced,
		Warnings:  warnings,
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

// draftSettings are the three §2.7 settings a draft is rendered under.
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
	cells, err := state.ReadRecords[coverage.Cell](l, owner, repo, pr, state.FileCoverage)
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
	stored, err := state.ReadRecords[*finding.Finding](l, owner, repo, pr, state.FileFindings)
	if err != nil {
		return nil, err
	}
	current := make([]*finding.Finding, 0, len(stored))
	for _, record := range stored {
		if record.Round == round {
			current = append(current, record)
		}
	}
	return current, nil
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
func queueRecords(records []*finding.Finding) ([]*finding.Finding, error) {
	queued := make([]*finding.Finding, 0, len(records))
	for _, record := range records {
		switch record.State {
		case finding.StateDraft:
			if err := finding.MayTransition(
				record.ID, finding.Existing(finding.StateDraft),
				finding.StateQueued, finding.ActorDraft,
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
// §7.1.5's rendered.json beside it, and §6.3.2's forcing count in the round
// summary.
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
	records []*finding.Finding, out drafted, forced finding.Forcings,
) error {
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}
	writes := []func() error{
		func() error { return state.ReplaceStamped(held, state.FileFindings, stamp, records) },
		func() error { return held.WriteRound(round.Round, state.FileDraft, []byte(out.file)) },
		func() error { return writeRendered(held, round.Round, out.rendered) },
		func() error { return writeForcingCounts(held, round.Round, forced) },
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

// summaryForcedToQuestion is §10.3's "forced to question" count, by the key
// summary.json holds it under.
const summaryForcedToQuestion = "forced_to_question"

// writeForcingCounts puts §6.3.2's count per class into the round's
// summary.json, as the one section of that document `cr draft` owns.
//
// §10.3 has `cr merge`, `cr draft` and `cr post` each accumulate their own
// counts into one file, so the write goes through UpdateRoundSection, which
// leaves every field this command does not own byte for byte.
func writeForcingCounts(held *state.Lock, round int, forced finding.Forcings) error {
	return state.UpdateRoundSection(
		held, round, state.FileSummary, summaryForcedToQuestion, forced)
}
