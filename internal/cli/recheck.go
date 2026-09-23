package cli

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/migrate"
	"github.com/deligoez/cr/internal/state"
)

// postedState is what §9.5.2 and §9.5.3 report about one posted record: where
// GitHub now holds its thread, and what has been said in it.
type postedState struct {
	// ID is the record, and Thread its GitHub node id.
	ID     string `json:"id"`
	Thread string `json:"thread"`
	// Kind is the record's register, so a reader can tell which of §9.5.5's
	// verbs is even available: `answered` is a question's.
	Kind string `json:"kind"`
	// Outdated is GitHub's own answer to whether the head has moved past
	// the thread's code.
	Outdated bool `json:"outdated"`
	// Resolved reports a thread already closed on GitHub, which cr may not
	// have been the one to close.
	Resolved bool `json:"resolved"`
	// StartLine and Line are where GitHub places the thread now, and zero
	// when it places it nowhere. OriginalStartLine and OriginalLine
	// survive that, per §9.5.2.
	StartLine         int `json:"start_line"`
	Line              int `json:"line"`
	OriginalStartLine int `json:"original_start_line"`
	OriginalLine      int `json:"original_line"`
	// Replies are the comments in the thread after the opening one,
	// offered as candidate context notes per §9.5.3.
	Replies []gh.Comment `json:"replies"`
}

// recheckResult is §9.5's report. It carries no verdict and no recommendation:
// §9.5.6 leaves the judgement to the agent, and a field here called anything
// like `likely_addressed` would be that judgement with a hedge in front of it.
type recheckResult struct {
	// Round is the round the report was made in.
	Round int `json:"round"`
	// Concerns is one entry per record in state `posted`.
	//
	// The field is not called `posted`: §12.6 gives that name to the flag
	// `posting` declares, and a second field spelling it would make two
	// different questions look like one in a payload.
	Concerns []postedState `json:"concerns"`
	// Migrated is §9.5.7's report of the migrations.ndjson lines `cr brief`
	// wrote when it opened this round: every record §9.4 migrated, and
	// whether §9.4.5 carried it.
	Migrated []migrate.Record `json:"migrated"`
	// Preview is what §9.4 would place at the current head while that head
	// differs from the round's, and is empty otherwise. It is computed and
	// written nowhere: the move is `cr brief`'s to make, and until then no
	// record has moved.
	Preview []migrate.Outcome `json:"preview"`
	// Honesty carries §9.3.1's head comparison.
	Honesty []string `json:"honesty"`
}

func (r *recheckResult) Text(w *writer) string {
	text := "round " + strconv.Itoa(r.Round) + ": " +
		strconv.Itoa(len(r.Concerns)) + " posted record(s)"
	for _, one := range r.Concerns {
		text += "\n  " + w.accent(one.ID) + " " + one.Kind
		if one.Outdated {
			text += ", outdated"
		}
		if one.Resolved {
			text += ", already resolved"
		}
		if len(one.Replies) > 0 {
			text += ", " + strconv.Itoa(len(one.Replies)) + " repl(ies)"
		}
	}
	for i := range r.Migrated {
		one := &r.Migrated[i]
		text += "\n  " + w.accent(one.Record) + " " + migrationText(&one.Outcome)
		if one.Carried {
			text += ", carried from round " + strconv.Itoa(one.FromRound)
		} else {
			text += ", staled"
		}
	}
	for _, one := range r.Preview {
		text += "\n  " + w.accent(one.Record) + " " + migrationText(&one) + ", if briefed now"
	}
	return text + w.disclose("\n", "", r.Honesty...)
}

// migrationText is one §9.4.7 line: where the anchor was, and where it went or
// why it did not.
func migrationText(one *migrate.Outcome) string {
	if one.Placed {
		return one.From + " → " + one.To + " (" + one.Key + ")"
	}
	return one.From + " unplaced: " + strconv.Itoa(one.Candidates) + " candidate(s) by " + one.Key
}

// newRecheckCmd reports what came back (§9.5).
//
// It performs no network write, changes no record's state, and reaches no
// verdict. That last one is the design and not an omission: §9.5.6 spells out
// why cr may not conclude from an outdated thread or a silent probe that a
// concern was addressed, and the tempting shape for this command is exactly
// the one that would.
func newRecheckCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "recheck " + prPlaceholder,
		Short: "Report what came back: thread state, replies, migrated anchors",
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
			var opened gh.PullRequest
			round, err := briefedRound(layout, owner, repo, pr, &opened)
			if err != nil {
				return err
			}
			// §9.5.1: an unresolved post is settled before anything is
			// read back. A round that may or may not have posted its
			// records is one where "what came back" cannot be read: a
			// record reported as posted-with-no-replies and a record
			// that never went out look identical from here.
			if err := refuseUnresolvedPost(&round.Meta, recheckUnresolvedWhy); err != nil {
				return err
			}
			report, err := recheckRound(layout, owner, repo, pr, &round, opened.Base)
			if err != nil {
				// §9.4.2's read is of the head's tree, so a clone
				// that never received it fails here rather than
				// at a refusal — and the step is a fetch, not a
				// retyped command line.
				return headNotFetched(cmd, owner, repo, pr, err)
			}
			return out.emit(report)
		},
	}
}

// recheckRound assembles §9.5's report: the posted concerns, and §9.5.7's
// migrations — the ones `cr brief` made for this round, or, while the head has
// moved past it, the ones it would make. base is the pull request's base, which
// the preview's diff is taken against.
func recheckRound(
	l state.Layout, owner, repo string, pr int, round *state.Round, base string,
) (*recheckResult, error) {
	records, err := state.ReadStamped[*finding.Finding](
		l, owner, repo, pr, state.FileFindings, round.Round)
	if err != nil {
		return nil, err
	}
	// A posted record stays in the round that posted it, so the concerns
	// are read from every round; the migrations below are the current
	// round's own work.
	posted, err := postedConcerns(l, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	// §9.5.2: what GitHub says now, read through §3.5's ingestion. The
	// threads `cr brief` stored are what GitHub said when the round opened,
	// before the review this report is about was posted at all.
	threads, err := ghClient().Threads(owner, repo, pr)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*gh.Thread, len(threads))
	for i := range threads {
		byID[threads[i].ID] = &threads[i]
	}

	report := &recheckResult{
		Round:    round.Round,
		Concerns: make([]postedState, 0),
		Migrated: make([]migrate.Record, 0),
		Preview:  make([]migrate.Outcome, 0),
		Honesty:  []string{round.Disclosure()},
	}
	for _, record := range posted {
		// A record whose thread cr has not ingested reports what it
		// knows and nothing invented, which is postedOf's job; the zero
		// value is what it is handed to say so.
		thread := byID[record.ThreadID]
		if thread == nil {
			thread = &gh.Thread{}
		}
		report.Concerns = append(report.Concerns, postedOf(record, thread))
		if record.Probe != "" {
			report.Honesty = append(report.Honesty, fmt.Sprintf(
				"record %s names probe %s, and §9.5.4's re-run of it at the current head is not "+
					"implemented in this release: whether it still reproduces was not read",
				record.ID, record.Probe))
		}
	}
	if round.Stale() {
		if report.Preview, err = previewMigrations(round, base, records); err != nil {
			return nil, err
		}
		return report, nil
	}
	migrated, err := state.ReadRecords[migrate.Record](l, owner, repo, pr, state.FileMigrations)
	if err != nil {
		return nil, err
	}
	for i := range migrated {
		if migrated[i].Round == round.Round {
			report.Migrated = append(report.Migrated, migrated[i])
		}
	}
	return report, nil
}

// postedOf reads §9.5.2's and §9.5.3's facts off one ingested thread.
//
// A record whose thread cr has not ingested reports what it knows and nothing
// invented: the zero values say "no current line" exactly as GitHub's nulls do
// for an outdated thread, which is the honest answer when the read has not
// happened rather than a claim that the thread is gone.
func postedOf(record *finding.Finding, thread *gh.Thread) postedState {
	return postedState{
		ID: record.ID, Thread: record.ThreadID, Kind: string(record.Kind),
		Outdated: thread.Outdated, Resolved: thread.Resolved,
		StartLine: thread.Anchor.StartLine, Line: thread.Anchor.Line,
		OriginalStartLine: thread.Anchor.OriginalStartLine, OriginalLine: thread.Anchor.OriginalLine,
		Replies: thread.Replies,
	}
}

// previewMigrations is §9.5.7's second half: while the current head differs
// from the round's, what §9.4 would place at it for each of the round's unsent
// records, written nowhere.
//
// The files searched are §9.4.3's: the record's own, then every file the diff
// at the current head touches, which is the diff `cr brief` would take. Every
// file is read at round.Current rather than the recorded head: the recorded
// head is the commit the record was stamped against, and a migration read from
// it finds every anchor exactly where it was. Measured 2026-09-22 on
// deligoez/cr-qa#24: three lines inserted above a queued record reported
// `33 → 33` against the recorded head.
func previewMigrations(round *state.Round, base string, records []*finding.Finding) ([]migrate.Outcome, error) {
	preview := make([]migrate.Outcome, 0)
	unsent := make([]*finding.Finding, 0, len(records))
	for _, record := range records {
		// §9.4.1 migrates `draft` and `queued` and never a posted
		// record: GitHub holds a posted record's place and reports it.
		if record.State.Unsent() && record.Anchor.Side == git.Right {
			unsent = append(unsent, record)
		}
	}
	if len(unsent) == 0 {
		return preview, nil
	}
	dir, err := repoDir()
	if err != nil {
		return nil, err
	}
	touched, err := touchedAt(dir, base, round.Current)
	if err != nil {
		return nil, err
	}
	head := &headTree{dir: dir, head: round.Current, read: make(map[string]*migrate.File)}
	for _, record := range unsent {
		files, err := head.searched(append([]string{record.Anchor.Path}, touched...))
		if err != nil {
			return nil, err
		}
		preview = append(preview, migrate.Anchor(record.ID, &record.Anchor, files))
	}
	return preview, nil
}

// touchedAt are the files the diff from base's merge base to head changes, the
// ones §9.4.3 searches after an anchor's own. A pull request whose base GitHub
// did not report has none to add.
func touchedAt(dir, base, head string) ([]string, error) {
	touched := make([]string, 0)
	if base == "" {
		return touched, nil
	}
	mergeBase, err := git.MergeBase(dir, base, head)
	if err != nil {
		return nil, err
	}
	changed, err := git.ChangedFiles(dir, mergeBase, head)
	if err != nil {
		return nil, err
	}
	for _, file := range changed {
		if file.Changed && !file.Binary {
			touched = append(touched, file.Path)
		}
	}
	return touched, nil
}

// headTree reads files at one head, each once.
type headTree struct {
	dir, head string
	// read holds every path asked for, nil for one the head does not hold.
	read map[string]*migrate.File
}

// searched are the files paths name that the head holds, each once, in the
// order given.
func (h *headTree) searched(paths []string) ([]migrate.File, error) {
	files := make([]migrate.File, 0, len(paths))
	for i, path := range paths {
		if slices.Contains(paths[:i], path) {
			continue
		}
		file, done := h.read[path]
		if !done {
			lines, exists, err := git.FileAtRevision(h.dir, h.head, path)
			if err != nil {
				return nil, err
			}
			if exists {
				file = &migrate.File{Path: path, Lines: lines}
			}
			h.read[path] = file
		}
		if file != nil {
			files = append(files, *file)
		}
	}
	return files, nil
}
