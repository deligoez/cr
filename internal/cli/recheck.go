package cli

import (
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
	// Line is where GitHub places the thread now, and zero when it places
	// it nowhere. OriginalLine survives that, per §9.5.2.
	Line         int `json:"line"`
	OriginalLine int `json:"original_line"`
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
	// Migrated is §9.4.7's line per anchor cr moved or declined to move.
	Migrated []migrate.Outcome `json:"migrated"`
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
	for _, one := range r.Migrated {
		text += "\n  " + w.accent(one.Record) + " " + one.From
		if one.Placed {
			text += " → " + one.To + " (" + one.Key + ")"
			continue
		}
		text += " unplaced: " + strconv.Itoa(one.Candidates) + " candidate(s) by " + one.Key
	}
	return text + w.disclose("\n", "", r.Honesty...)
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
			round, err := briefedRound(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			// §9.5.1: an unresolved post is settled before anything is
			// read back. A round that may or may not have posted its
			// records is one where "what came back" cannot be read: a
			// record reported as posted-with-no-replies and a record
			// that never went out look identical from here.
			if err := refuseUnresolvedPost(&round.Meta); err != nil {
				return err
			}
			report, err := recheckRound(layout, owner, repo, pr, &round)
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

// recheckRound assembles §9.5's report and §9.4's migrations.
func recheckRound(
	l state.Layout, owner, repo string, pr int, round *state.Round,
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
	threads, err := state.ReadRecords[gh.Thread](l, owner, repo, pr, state.FileThreads)
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
		Migrated: make([]migrate.Outcome, 0),
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
	}
	for _, record := range records {
		// §9.4.1 migrates what cr owns, which is every non-terminal
		// record other than a posted one: GitHub holds a posted
		// record's place and reports it above.
		if record.State.Unsent() && record.Anchor.Path != "" {
			outcome, err := migrateOne(round, record)
			if err != nil {
				return nil, err
			}
			report.Migrated = append(report.Migrated, outcome)
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
		Line: thread.Anchor.Line, OriginalLine: thread.Anchor.OriginalLine,
		Replies: thread.Replies,
	}
}

// migrateOne runs §9.4 for one record against the current head.
//
// The files it searches are the record's own and every file the head carries
// under the paths the round's units name, which is §9.4.3's order. Reading the
// head rather than the worktree is §9.4.2's constraint in its practical form:
// the superseded commit is not required and is never asked for.
func migrateOne(round *state.Round, record *finding.Finding) (migrate.Outcome, error) {
	dir, err := repoDir()
	if err != nil {
		return migrate.Outcome{}, err
	}
	lines, exists, err := git.FileAtRevision(dir, round.Head, record.Anchor.Path)
	if err != nil {
		return migrate.Outcome{}, err
	}
	files := make([]migrate.File, 0, 1)
	if exists {
		files = append(files, migrate.File{Path: record.Anchor.Path, Lines: lines})
	}
	return migrate.Anchor(record.ID, &record.Anchor, files), nil
}
