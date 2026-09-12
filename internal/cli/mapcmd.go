package cli

import (
	"errors"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/state"
)

// mapRecordResult is what `cr map record` has to report: the pairs it stored,
// whole, and the round they replaced.
//
// They are printed rather than counted for the reason claimsRecordResult gives:
// §2.3.3 stamps head and round, so the pair the agent handed in and the pair
// mapping.ndjson now holds are not the same document, and handing back what was
// stored is how the caller learns what cr made of its input.
type mapRecordResult struct {
	// Recorded are the pairs as they were written, in file order.
	Recorded []*mapping.Pair `json:"recorded"`
	// Round is the round whose mapping was replaced. §4.1.6 replaces
	// mapping.ndjson and §9.3.5 scopes that to the round, so it is
	// reported: it is the difference between a run that replaced this
	// round's mapping and one that would have replaced the whole file.
	Round int `json:"round"`
}

// Text names how many pairs were stored and the round they stand in. The pairs
// themselves came from the caller's own file, so printing them into a terminal
// would repeat what the caller has.
func (r *mapRecordResult) Text(w *writer) string {
	return "recorded " + w.accent(strconv.Itoa(len(r.Recorded))) +
		" mapping(s) in round " + strconv.Itoa(r.Round)
}

// newMapCmd groups the mapping commands of §11. It runs nothing itself, so an
// invocation naming no subcommand prints the help rather than doing something
// the user did not ask for.
func newMapCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "map",
		Short: "Manage the claim-to-unit mapping",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newMapRecordCmd(out))
	return cmd
}

// newMapRecordCmd stores the claim-to-unit mapping the agent judged (§11,
// §4.1.6).
//
// The whole file is validated before anything is written, for the reason
// `cr claims record` gives: §4.1.6 rejects a pair with exit code 1, and a
// rejection that had already replaced mapping.ndjson would have thrown away the
// round's previous mapping on the strength of a file the agent is about to
// correct. mapping.Decode refuses the first faulty line and returns no pairs at
// all, so the write below either happens whole or does not happen.
//
// The replacement is the whole of this round's mapping rather than the pairs
// the file names, which is where §4.1.6 and §4.5.6 differ and the difference is
// deliberate. A cell is filled per `(unit, role)` by many roles handing in many
// files, so §4.5.6 replaces by key; the mapping is one judgement about the
// whole diff, made by the intent role in one pass per §4.6.5, and a pair the
// new file leaves out is a pair the agent no longer stands behind. Keeping it
// would leave §4.1.2 reading a unit as mapped on the strength of a mapping that
// was withdrawn.
func newMapRecordCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "record " + prPlaceholder + " <file>",
		Short: "Store the claim-to-unit mapping",
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
			// Briefed rather than ReadMeta: §4.1.6 checks every
			// pair's unit against the units of the round, and
			// units.ndjson is `cr brief`'s to write per §3.7. A
			// pull request no round has been opened on has an empty
			// one, so every pair would be refused for naming an
			// unknown unit rather than for the reason it was
			// actually refused. §11.2 codes that 4.
			round, err := layout.Briefed(owner, repo, pr)
			if err != nil {
				return err
			}
			formed, err := roundUnitsOf(layout, owner, repo, pr, round.Round)
			if err != nil {
				return err
			}
			claims, err := roundClaimIDs(layout, owner, repo, pr, round.Round)
			if err != nil {
				return err
			}
			body, err := os.ReadFile(args[1])
			if err != nil {
				return err
			}
			pairs, err := mapping.Decode(args[1], body, claims, roundUnitIDs(formed))
			if err != nil {
				return err
			}
			if err := storeMapping(
				layout, owner, repo, pr,
				state.Stamp{Head: round.Head, Round: round.Round}, pairs, claims,
			); err != nil {
				return err
			}
			return out.emit(&mapRecordResult{Recorded: pairs, Round: round.Round})
		},
	}
}

// storeMapping publishes one `cr map record`: §4.1.6's mapping for the round,
// and §4.1.3's unimplemented-claim entries derived from it.
//
// The two are one critical section under one hold of the §2.3.1 lock because
// the second is a function of the first. A reader arriving between them would
// find this round's mapping beside the gaps derived from the previous one, and
// §10.2.3 would block completeness on a claim the file next to it already maps.
func storeMapping(
	l state.Layout, owner, repo string, pr int,
	at state.Stamp, pairs []*mapping.Pair, claims []string,
) error {
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	// Joined rather than branched: the lock is released whether or not the
	// writes succeeded, and neither failure is traded away for the other.
	return errors.Join(writeMapping(l, held, owner, repo, pr, at, pairs, claims), held.Unlock())
}

// writeMapping publishes the two files through the held lock.
//
// The gaps are written with ReplaceStamped rather than WriteStamped for the
// reason the mapping is: §9.3.5 scopes a replacement to the current round, and
// an entry raised in an earlier round is history that round's report was
// written against.
//
// The previously recorded entries are read back and handed to mapping.Gaps,
// which is what carries a §4.1.8 set-aside through a re-recorded mapping. The
// read takes no lock of its own and needs none — this holds the §2.3.1 lock, so
// no other writer can be between it and the write that follows.
func writeMapping(
	l state.Layout, held *state.Lock, owner, repo string, pr int,
	at state.Stamp, pairs []*mapping.Pair, claims []string,
) error {
	if err := state.ReplaceStamped(held, state.FileMapping, at, pairs); err != nil {
		return err
	}
	recorded, err := state.ReadRecords[mapping.Gap](l, owner, repo, pr, state.FileIntentGaps)
	if err != nil {
		return err
	}
	return state.ReplaceStamped(
		held, state.FileIntentGaps, at, mapping.Gaps(claims, storedPairs(pairs), at.Round, recorded))
}

// storedPairs is the round's mapping as mapping.Gaps reads it. The pairs were
// stamped by the write above, so they carry the round the derivation scopes by.
func storedPairs(pairs []*mapping.Pair) []mapping.Pair {
	stored := make([]mapping.Pair, 0, len(pairs))
	for _, pair := range pairs {
		stored = append(stored, *pair)
	}
	return stored
}

// roundClaimIDs is the set §4.1.6 checks a pair's `claim` against: the ids of
// the claims recorded for the round this pull request is in.
//
// The round scoping is §9.3.5's, and it is not the same scoping the units get.
// §9.3.4 carries claims forward across a round increment unchanged, so a claim
// of round 1 is re-recorded into round 2 rather than expiring — but each round's
// claims are still that round's records, and a mapping written in round 2 joins
// round 2's claims to round 2's units.
func roundClaimIDs(l state.Layout, owner, repo string, pr, round int) ([]string, error) {
	stored, err := state.ReadRecords[intent.Claim](l, owner, repo, pr, state.FileClaims)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(stored))
	for i := range stored {
		if stored[i].Round == round {
			ids = append(ids, stored[i].ID)
		}
	}
	return ids, nil
}
