package cli

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// claimsRecordResult is what `cr claims record` has to report: the claims it
// stored, whole, and the round they replaced.
//
// They are printed rather than counted for the reason recordResult gives about
// findings: §3.3 makes `span_hash` and `issue_hash` computed and §2.3.3 makes
// head and round stamped, so the claim the agent handed in and the claim
// claims.ndjson now holds are not the same document. Handing back what was
// stored is how the caller learns what cr made of its input — and `issue_hash`
// in particular is the value §3.3.3 will compare against next round, which no
// caller can derive from the file it wrote.
type claimsRecordResult struct {
	// Recorded are the claims as they were written, in file order.
	Recorded []*intent.Claim `json:"recorded"`
	// Round is the round the extraction replaced. §9.3.5 scopes both the
	// replacement and the mapping clearing to it, so it is reported: it is
	// the difference between a run that replaced this round's claims and
	// one that would have replaced the whole file.
	Round int `json:"round"`
	// DroppedSetAsides are the §4.1.8 stamps the round's intent-gaps.ndjson
	// carried and this run cleared with it, in the shape and for the reason
	// `cr map record` reports its own: a set-aside is the agent's judgement,
	// and the entry that held it is gone once the run ends, so this is the
	// only place its loss can be stated.
	DroppedSetAsides []mapping.DroppedSetAside `json:"dropped_set_asides"`
}

// Text names how many claims were stored, the round they stand in, and the
// mapping that went with them. The claims themselves came from the caller's own
// file, so printing them into a terminal would repeat what the caller has.
//
// The cleared mapping is named because nothing else says it happened. §3.3.1
// clears mapping.ndjson as part of recording claims, and a caller that had
// recorded a mapping is entitled to learn from this command that it is gone
// rather than from the next one that finds none. A cleared set-aside is named
// by its claim for the same reason, as `cr map record` names the ones it drops.
func (r *claimsRecordResult) Text(w *writer) string {
	line := "recorded " + w.accent(strconv.Itoa(len(r.Recorded))) +
		" claim(s) in round " + strconv.Itoa(r.Round) + "; mapping cleared"
	if len(r.DroppedSetAsides) == 0 {
		return line
	}
	claims := make([]string, 0, len(r.DroppedSetAsides))
	for _, drop := range r.DroppedSetAsides {
		claims = append(claims, drop.Claim)
	}
	return line + "; dropped the set-aside of " + w.accent(strings.Join(claims, ", "))
}

// newClaimsCmd groups the claim commands of §11. It runs nothing itself, so an
// invocation naming no subcommand prints the help rather than doing something
// the user did not ask for.
func newClaimsCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claims",
		Short: "Manage the claims extracted from the issue",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newClaimsRecordCmd(out))
	cmd.AddCommand(newClaimsSetAsideCmd(out))
	return cmd
}

// newClaimsSetAsideCmd registers §11's
// `cr claims set-aside <pr> <claim-id> --note <note-id>`, which §4.1.8 has
// stamp `set_aside_note` on an intent gap entry after checking the note exists
// for the pull request's issue key.
//
// `--note` is not optional decoration. §4.1.3 writes an unimplemented claim to
// `intent-gaps.ndjson` and §10.2.3 lets it block completeness until it is
// mapped or set aside, so the note is the whole of what a set-aside rests on:
// judging a claim out of scope is the agent's call, never cr's, and the note id
// is where that call is recorded. A set-aside with nothing behind it would be
// cr forming the opinion §4.1.8 denies it.
//
// The behaviour belongs to claims-set-aside-command; what is registered here is
// the shape.
func newClaimsSetAsideCmd(out *writer) *cobra.Command {
	var noteID string

	cmd := &cobra.Command{
		Use:   "set-aside " + prPlaceholder + " <claim-id>",
		Short: "Mark an unimplemented claim out of scope",
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
			// Briefed rather than ReadMeta, for the reason
			// `cr map record` gives: §4.1.7 derives the entry this
			// stamps from the round's claims and mapping, and a
			// pull request no round has been opened on raises no
			// entry to stamp. §11.2's code 4 names `cr brief`.
			recorded, err := briefedRound(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			// §9.3.2: this writes intent-gaps.ndjson, so a head
			// that moved under the round refuses here. The entry
			// being stamped was derived from that round's units,
			// and §9.3.4 is about to clear the mapping it came
			// from — a set-aside recorded in between would be a
			// judgement about a diff nobody is reviewing any more.
			if err := recorded.RefuseStale(); err != nil {
				return err
			}
			if err := stampSetAside(layout, &recorded, args[1], noteID); err != nil {
				return err
			}
			return out.emit(&claimsSetAsideResult{
				Claim: args[1], Note: noteID, Round: recorded.Round,
			})
		},
	}
	cmd.Flags().StringVar(&noteID, "note", "",
		"the note id recording why the claim is out of scope (§4.1.8)")
	// Required rather than defaulted, for the reason the doc comment
	// gives: a set-aside with no note behind it is cr forming the opinion
	// §4.1.8 denies it. A missing flag is a malformed invocation, which
	// cobra refuses before RunE and §11.2 codes 2.
	_ = cmd.MarkFlagRequired("note")

	return cmd
}

// claimsSetAsideResult is what `cr claims set-aside` reports: the claim it set
// aside, the note the decision rests on, and the round the entry stands in.
//
// The note is echoed rather than assumed. It was checked against the store
// before anything was written, and a caller that mistyped an id learns from the
// refusal; a caller that did not is told which note the entry now carries,
// which is the field §10.2.3 will read when it decides whether the claim still
// blocks completeness.
type claimsSetAsideResult struct {
	// Claim is the claim id §4.1.3 raised the entry for.
	Claim string `json:"claim"`
	// Note is the note id §4.1.8 stamped onto it.
	Note string `json:"note"`
	// Round is the round the entry belongs to, per §9.3.5.
	Round int `json:"round"`
}

func (r *claimsSetAsideResult) Text(w *writer) string {
	return "set " + w.accent(r.Claim) + " aside in round " + strconv.Itoa(r.Round) +
		" on note " + r.Note
}

// RetractedSetAsideNoteError is `cr claims set-aside` naming a note §3.6.6 has
// retracted. The id names a note the store holds and the entry may well exist,
// so neither the invocation nor the state is wrong; what fails is the set-aside
// itself, which could not settle the claim, and §11.2 codes that 1 beside
// note.UnknownNoteError.
type RetractedSetAsideNoteError struct {
	// Claim is the claim id the set-aside was asked for.
	Claim string
	// Note is the retracted note id it was asked to rest on.
	Note string
}

func (e *RetractedSetAsideNoteError) Error() string {
	return "note " + e.Note + " is retracted, per §3.6.6, so it cannot set " + e.Claim +
		" aside: §10.2.3 counts a set-aside only while its note stands"
}

// stampSetAside is §4.1.8's check and its write.
//
// The note is checked before the lock is taken and the entry before anything is
// written, which is the order `cr claims record` and `cr map record` both use:
// §4.1.8's validation can refuse, and a refusal that had already republished
// intent-gaps.ndjson would have rewritten the round's entries on the strength
// of a command the caller is about to retype.
//
// §4.1.8 asks that the note exist for the PR's issue key, and a note §3.6.6 has
// already retracted is refused too: `cr status` reports a set-aside resting on a
// retracted note as needing re-evaluation and lets it block completeness again,
// so stamping one would record a decision already known not to settle anything.
// The store is the issue key's, loaded whole: §3.6.4 gives every round the same
// notes and §9.3.5 exempts the store from round scoping, so a slice narrowed by
// round or by pull request would report a note somebody recorded as one that
// does not exist.
func stampSetAside(l state.Layout, round *state.Round, claim, noteID string) error {
	// The key is settled before the store is opened, not after: §2.2 puts
	// the store at context/<ISSUE-KEY>.ndjson, so a load under no key names
	// the directory plus an extension rather than asking an empty question.
	if round.IssueKey == "" {
		return &intent.NoIssueKeyError{Owner: round.Owner, Repo: round.Repo, PR: round.PR}
	}
	notes, err := note.Load(l, round.IssueKey)
	if err != nil {
		return err
	}
	switch note.StandingOf(notes, noteID) {
	case note.StandingDangling:
		return &note.UnknownNoteError{ID: noteID, IssueKey: round.IssueKey}
	case note.StandingRetracted:
		return &RetractedSetAsideNoteError{Claim: claim, Note: noteID}
	}
	recorded, err := state.ReadStamped[mapping.Gap](
		l, round.Owner, round.Repo, round.PR, state.FileIntentGaps, round.Round)
	if err != nil {
		return err
	}
	stamped, err := mapping.SetAside(recorded, claim, noteID, round.Round)
	if err != nil {
		return err
	}
	held, err := l.LockPR(round.Owner, round.Repo, round.PR)
	if err != nil {
		return err
	}
	at := state.Stamp{Head: round.Head, Round: round.Round}
	// Joined rather than branched: the lock is released whether or not the
	// write succeeded, and neither failure is traded away for the other.
	return errors.Join(
		state.ReplaceStamped(held, state.FileIntentGaps, at, stamped), held.Unlock())
}

// newClaimsRecordCmd stores the claims the agent extracted from the issue
// (§11, §3.3.1).
//
// The whole file is validated before anything is written, for the reason `cr
// record` gives: §3.3.1 rejects a claim with exit code 1, and a rejection that
// had already replaced claims.ndjson would have thrown away the round's
// previous extraction on the strength of a file the agent is about to correct.
// intent.DecodeClaims refuses the first faulty line and returns no claims at
// all, so the write below either happens whole or does not happen.
//
// Nothing here re-reads §3.3's table. intent.DecodeClaims is the one door: it
// holds every line to the table's required rows, to its two computed rows, and
// to the conditional `note_id`, naming the file, the one-based line and the
// field in what it refuses, and internal/cli maps that onto §11.2's code 1.
//
//nolint:funlen // measured 2026-08-31 at 69 lines; refactor to clear, never raise the limit
func newClaimsRecordCmd(out *writer) *cobra.Command {
	var intentFile string

	cmd := &cobra.Command{
		Use:   "record " + prPlaceholder + " <file>",
		Short: "Store the claims extracted from the issue",
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
			// §3.2's resolution is read out of §2.3's metadata
			// rather than re-run here, for the reason note.Answer
			// reads it there: that is the key the round actually
			// resolved under, and §3.3 forms every claim id from
			// it. It is asked of Briefed rather than of ReadMeta
			// so a pull request no round has been opened on is
			// refused here, per §3.7 and §11.2's code 4, instead
			// of recording claims against round 0.
			recorded, err := briefedRound(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			// §9.3.2: this command writes claims.ndjson, so a head
			// that moved under the round refuses here rather than
			// stamping the round's head onto claims checked against
			// an issue read at another one.
			if err := recorded.RefuseStale(); err != nil {
				return err
			}
			if recorded.IssueKey == "" {
				return &intent.NoIssueKeyError{Owner: owner, Repo: repo, PR: pr}
			}
			source, err := intentSource(layout, owner, repo, intentFile)
			if err != nil {
				return err
			}
			issueText, err := intent.Read(source, recorded.IssueKey)
			if err != nil {
				return err
			}
			body, err := readInput(args[1],
				"§3.3 has the agent write the claims it extracted from the issue "+
					"to this file before `cr claims record` reads it")
			if err != nil {
				return err
			}
			// §3.6.4 loads the issue key's notes on every round, and
			// §3.3.2 has a note-sourced claim validated against the
			// one it names. The whole store is loaded rather than a
			// selection: §9.3.5 exempts it from round scoping, and a
			// narrowed slice would refuse a claim resting on a note
			// somebody recorded.
			notes, err := note.Load(layout, recorded.IssueKey)
			if err != nil {
				return err
			}
			claims, err := intent.DecodeClaims(args[1], body, recorded.IssueKey,
				intent.SpanTexts{Issue: issueText, Notes: notes})
			if err != nil {
				return err
			}
			// §3.3.1: cr computes span_hash and issue_hash itself.
			// It happens after the decode and before the write, so
			// no claim reaches claims.ndjson without them and no
			// hash is computed over a file that was going to be
			// refused.
			if err := intent.ComputeClaimHashes(claims, issueText); err != nil {
				return err
			}
			held, err := layout.LockPR(owner, repo, pr)
			if err != nil {
				return err
			}
			dropped, err := storeClaims(layout, held, &recorded.Meta, claims)
			if err != nil {
				// The lock is released on the way out of every
				// branch, and the write's own failure is what
				// the caller is told about.
				_ = held.Unlock()
				return err
			}
			if err := held.Unlock(); err != nil {
				return err
			}
			return out.emit(&claimsRecordResult{
				Recorded: claims, Round: recorded.Round, DroppedSetAsides: dropped,
			})
		},
	}
	cmd.Flags().StringVar(&intentFile, "intent-file", "",
		"read the issue text from this file instead of running the tracker command")

	return cmd
}

// storeClaims is §3.3.1's write, both halves of it, under one hold of the
// §2.3.1 lock.
//
// The two are one critical section rather than two, because §3.3.1 states them
// as one act: the mapping of §4.1.6 maps claims to units, so a mapping left
// standing beside a replaced claim set maps ids that may no longer exist. A
// reader arriving between two separately locked writes would see exactly that.
//
// Each half is scoped to the round per §9.3.5, which is what state's replace
// and clear are for; neither touches a line of any other round.
//
// What the mapping carries goes with it. meta.json's mapping stamp is what
// §4.6.5's gate reads as a mapping recorded for the round, and intent-gaps.ndjson
// is §4.1.7's derivation from the mapping just cleared: left standing, the one
// unblocks `cr review` over no mapping and the other reports gaps of a claim set
// that is gone. The stamp is cleared first, so a run that fails part way leaves
// a round §4.6.5 refuses rather than one it unblocks.
//
// The claims stamp is set last, for the same reason: it is what `cr status`
// reads as the round's claims recorded, and an empty claim set is recorded
// only there, so a run that fails before it leaves a round still asking for
// its claims.
//
// Clearing the entries clears every §4.1.8 set-aside they carried, and those
// are returned the way `cr map record` returns the ones its derivation drops:
// mapping.Gaps over no claim raises no entry, so every stamp the round recorded
// is one this run lost. They are read under the same hold, so no set-aside can
// land between the read and the clear and vanish unreported.
func storeClaims(
	l state.Layout, k *state.Lock, round *state.Meta, claims []*intent.Claim,
) ([]mapping.DroppedSetAside, error) {
	at := state.Stamp{Head: round.Head, Round: round.Round}
	recorded, err := state.ReadStamped[mapping.Gap](
		l, round.Owner, round.Repo, round.PR, state.FileIntentGaps, at.Round)
	if err != nil {
		return nil, err
	}
	_, dropped := mapping.Gaps([]string{}, nil, at.Round, recorded)
	if err := k.ClearMapping(); err != nil {
		return nil, err
	}
	if err := state.ReplaceStamped(k, state.FileClaims, at, claims); err != nil {
		return nil, err
	}
	if err := state.ClearStamped(k, state.FileMapping, at.Round); err != nil {
		return nil, err
	}
	if err := state.ClearStamped(k, state.FileIntentGaps, at.Round); err != nil {
		return nil, err
	}
	return dropped, k.StampClaims(at.Round, at.Head)
}

// intentSource is §3.1's choice of where one run's issue text comes from,
// resolved for a command that has a `--intent-file` flag.
//
// The flag is answered without reading configuration at all, which is what
// §3.1.4 asks for: the file bypasses the command, so a run given one must work
// with no tracker configured and no tracker installed. Resolving `intent.cmd`
// first and using the file only if it were absent would keep the promise by
// accident, and would break it the first time a malformed `intent.cmd` made
// config.Resolve refuse.
func intentSource(l state.Layout, owner, repo, file string) (intent.Source, error) {
	if file != "" {
		return intent.Source{File: file}, nil
	}
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: l.Config(),
		RepoConfig:   l.RepoConfig(owner, repo),
	})
	if err != nil {
		return intent.Source{}, err
	}
	return intent.Source{Cmd: resolved.Strings("intent.cmd")}, nil
}
