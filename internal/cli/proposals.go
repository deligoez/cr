package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/proposal"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// proposalsRecordResult is what `cr proposals record` reports: the proposals it
// stored, whole, and the round they were proposed in.
//
// They are printed rather than counted for the reason cellsRecordResult gives:
// §2.3.3 stamps head and round and §5.7's table computes `state`, so the
// proposal the agent handed in and the one proposals.ndjson now holds are not
// the same document.
type proposalsRecordResult struct {
	// Recorded are the proposals as they were written, in file order.
	Recorded []*proposal.Proposal `json:"recorded"`
	// Round is the round they were proposed in.
	Round int `json:"round"`
	// Honesty carries §2.4.6's stale shipped profile and §2.5.2's stale
	// ejected roles, both of which this command loads: the role corpus lays
	// out §4.6.2's id blocks and the profile is read on the way to the
	// round.
	Honesty []string `json:"honesty"`
}

// Text names how many proposals were stored and the round they stand in. The
// proposals themselves came from the caller's own file.
func (r *proposalsRecordResult) Text(w *writer) string {
	return "recorded " + w.accent(strconv.Itoa(len(r.Recorded))) +
		" proposal(s) in round " + strconv.Itoa(r.Round) +
		w.disclose("\n", "", r.Honesty...)
}

// newProposalsCmd groups the proposal commands of §11. It runs nothing itself,
// so an invocation naming no subcommand prints the help.
func newProposalsCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proposals",
		Short: "Manage the experiments the roles proposed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newProposalsRecordCmd(out))
	return cmd
}

// newProposalsRecordCmd stores the experiments the roles proposed (§11, §5.7.1).
//
// The whole file is validated before anything is written, for the reason
// `cr cells record` gives: §5.7.1 rejects a proposal with exit code 1 and says
// so in as many words — "storing nothing" — and a rejection that had already
// appended the proposals above it would leave the round holding half a file the
// agent is about to correct.
//
// Appended rather than replaced. A proposal is a role's ask and §5.7 gives it
// no key a later file replaces, so a second recording of the round adds to what
// the first stored; a repeated id is refused instead, here and in the decoder.
func newProposalsRecordCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "record " + prPlaceholder + " <file>",
		Short: "Store the experiments the roles proposed",
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
			// Briefed rather than ReadMeta, as `cr cells record`
			// is: §5.7.1 holds a proposal's unit against the units
			// of the round and its target against the head those
			// units were formed at, and both are `cr brief`'s to
			// write.
			round, err := briefedRound(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			// §9.3.2: this command writes proposals.ndjson, whose
			// records §2.3.3 stamps with the round's head.
			if err := round.RefuseStale(); err != nil {
				return err
			}
			// §5.7's targets resolve against the head, so a clone
			// that lacks it fails here rather than at a bad line:
			// the git failure goes through headNotFetched, which
			// answers it with the fetch that would fix it.
			proposals, body, err := decodeProposals(layout, &round.Meta, args[1])
			if err != nil {
				return headNotFetched(cmd, owner, repo, pr, err)
			}
			if err := refuseDepartedProposalIDs(
				layout, owner, repo, pr, round.Round, args[1], body, proposals,
			); err != nil {
				return err
			}
			held, err := layout.LockPR(owner, repo, pr)
			if err != nil {
				return err
			}
			stamp := state.Stamp{Head: round.Head, Round: round.Round}
			if err := state.AppendStamped(held, state.FileProposals, stamp, proposals); err != nil {
				// The lock is released on the way out of every
				// branch, and the write's own failure is what
				// the caller is told about.
				_ = held.Unlock()
				return err
			}
			if err := held.Unlock(); err != nil {
				return err
			}
			honesty := append(make([]string, 0, 2), staleProfile(layout.Profile(round.ProfileID))...)
			return out.emit(&proposalsRecordResult{
				Recorded: proposals, Round: round.Round,
				Honesty: append(honesty, staleRoles(layout, owner, repo)...),
			})
		},
	}
}

// decodeProposals reads the file an agent handed `cr proposals record` and
// holds it to the round: its units, its active roles, the records it holds, and
// the head its targets resolve against. It returns the file's body beside the
// proposals, because §5.7.1's id refusal names the line the user must open and
// state.RecordLines is what counts them.
func decodeProposals(
	l state.Layout, round *state.Meta, file string,
) ([]*proposal.Proposal, []byte, error) {
	formed, err := roundUnitsOf(l, round.Owner, round.Repo, round.PR, round.Round)
	if err != nil {
		return nil, nil, err
	}
	records, err := roundFindingsOf(l, round.Owner, round.Repo, round.PR, round.Round)
	if err != nil {
		return nil, nil, err
	}
	body, err := readInput(file,
		"§5.7.1 has the roles write the experiments they proposed to this "+
			"file before `cr proposals record` reads it")
	if err != nil {
		return nil, nil, err
	}
	// The checkout is asked for unconditionally, unlike `cr record`'s
	// citation resolution: §5.7's table requires `target` on every
	// proposal, so a file holding one proposal already needs the head.
	dir, err := repoDir()
	if err != nil {
		return nil, nil, err
	}
	units := make([]proposal.Unit, 0, len(formed))
	for i := range formed {
		units = append(units, proposal.Unit{ID: formed[i].ID, In: &formed[i].Unit})
	}
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	proposals, err := proposal.DecodeInRound(file, body, &proposal.Round{
		Units:   units,
		Active:  round.ActiveRoles,
		Records: ids,
		Head: func(path string) ([]string, bool, error) {
			return git.FileAtRevision(dir, round.Head, path)
		},
	})
	return proposals, body, err
}

// refuseDepartedProposalIDs refuses a proposal whose id lies outside the block
// §4.6.2 gave its role on its unit, or that a stored proposal already holds,
// naming the block and its next free id.
//
// It lives here rather than in the decoder for the reason refuseHeldIDs does:
// the blocks are laid out over the pull request's stored proposals and the
// resolved role corpus, and the decoder is handed a file and the round, never
// the store.
func refuseDepartedProposalIDs(
	l state.Layout, owner, repo string, pr, round int,
	file string, body []byte, proposals []*proposal.Proposal,
) error {
	if len(proposals) == 0 {
		return nil
	}
	stored, err := state.ReadRecords[proposal.Proposal](l, owner, repo, pr, state.FileProposals)
	if err != nil {
		return err
	}
	corpus, err := role.Resolve(l.RepoRolesDir(owner, repo), l.RolesDir())
	if err != nil {
		return err
	}
	formed, err := roundUnitsOf(l, owner, repo, pr, round)
	if err != nil {
		return err
	}
	units := roundUnitIDs(formed)
	at := state.RecordLines(body)
	for i, p := range proposals {
		for j := range stored {
			if stored[j].ID != p.ID {
				continue
			}
			return &proposal.RejectedError{
				File: file, Line: at[i], Field: "id",
				Problem: fmt.Sprintf("%q is already held by the proposal stored in round %d at head %s; "+
					"give this proposal an id no stored one holds", p.ID, stored[j].Round, stored[j].Head),
			}
		}
		block, given := review.ProposalIDs(stored, round, corpus, units, p.Role, p.Unit)
		n, spelled := proposal.IDSuffix(p.ID)
		if !given || (spelled && block.Holds(n)) {
			continue
		}
		// §5.7.1 refuses an id outside "the block §4.6.2 gave", and a
		// block is given by a prompt. A round `cr review` has emitted
		// nothing for on this unit told no role which ids to write, so
		// no prompt was ignored and no block handed out has moved. It
		// is §6.1's own condition for a record id, read off the unit's
		// fan-out directory the same way.
		emitted, err := l.FannedOut(owner, repo, pr, round, p.Unit)
		if err != nil {
			return err
		}
		if !emitted {
			continue
		}
		return &proposal.RejectedError{
			File: file, Line: at[i], Field: "id",
			Problem: fmt.Sprintf("%q lies outside %s..%s, the block §4.6.2 gave role %s on unit %s; "+
				"the next free id in that block is %s",
				p.ID, proposal.IDOf(block.Start()), proposal.IDOf(block.Last),
				p.Role, p.Unit, proposal.IDOf(block.First)),
		}
	}
	return nil
}
