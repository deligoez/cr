package cli

import (
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/intent"
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
}

// Text names how many claims were stored, the round they stand in, and the
// mapping that went with them. The claims themselves came from the caller's own
// file, so printing them into a terminal would repeat what the caller has.
//
// The cleared mapping is named because nothing else says it happened. §3.3.1
// clears mapping.ndjson as part of recording claims, and a caller that had
// recorded a mapping is entitled to learn from this command that it is gone
// rather than from the next one that finds none.
func (r *claimsRecordResult) Text(w *writer) string {
	return "recorded " + w.accent(strconv.Itoa(len(r.Recorded))) +
		" claim(s) in round " + strconv.Itoa(r.Round) + "; mapping cleared"
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
	return cmd
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
			// it.
			recorded, err := layout.ReadMeta(owner, repo, pr)
			if err != nil {
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
			body, err := os.ReadFile(args[1])
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
			stamp := state.Stamp{Head: recorded.Head, Round: recorded.Round}
			if err := storeClaims(held, stamp, claims); err != nil {
				// The lock is released on the way out of every
				// branch, and the write's own failure is what
				// the caller is told about.
				_ = held.Unlock()
				return err
			}
			if err := held.Unlock(); err != nil {
				return err
			}
			return out.emit(&claimsRecordResult{Recorded: claims, Round: recorded.Round})
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
func storeClaims(k *state.Lock, at state.Stamp, claims []*intent.Claim) error {
	if err := state.ReplaceStamped(k, state.FileClaims, at, claims); err != nil {
		return err
	}
	return state.ClearStamped(k, state.FileMapping, at.Round)
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
