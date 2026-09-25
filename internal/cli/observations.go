package cli

import (
	"errors"
	"io/fs"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/observation"
	"github.com/deligoez/cr/internal/state"
)

// observationsRecordResult is what `cr observations record` reports: the
// observations it stored, whole, and the round they were stored in.
//
// They are printed rather than counted for the reason proposalsRecordResult
// gives: cr writes the head and round onto every line, so what the agent handed
// in and what observations.ndjson now holds are not the same document.
type observationsRecordResult struct {
	// Recorded are the observations as they were written, in file order.
	Recorded []*observation.Observation `json:"recorded"`
	// Round is the round they were stored in.
	Round int `json:"round"`
}

// Text names how many observations were stored and the round they stand in.
// The observations themselves came from the caller's own file.
func (r *observationsRecordResult) Text(w *writer) string {
	return "recorded " + w.accent(strconv.Itoa(len(r.Recorded))) +
		" observation(s) in round " + strconv.Itoa(r.Round) +
		"; `cr draft` and `cr status` show them, and nothing posts them"
}

// newObservationsCmd groups the observation commands of §11. It runs nothing
// itself, so an invocation naming no subcommand prints the help.
func newObservationsCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "observations",
		Short: "Manage what the roles saw outside their units",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newObservationsRecordCmd(out))
	return cmd
}

// newObservationsRecordCmd stores what the roles saw outside their units (§11,
// §4.6.9).
//
// The whole file is validated before anything is written, for the reason
// `cr proposals record` gives: a refused line leaves the store as it was, so
// the agent corrects the file and records it whole. It is appended rather than
// replaced, as proposals are, because an observation has no key a later file
// replaces: two roles that saw the same thing wrote two lines.
//
// The append runs even for a file holding no observation, and that is what
// `cr next` reads: observations.ndjson is last written by the round's last
// `cr observations record`, and a role's file written after it is one
// §10.4.5 still owes a recording.
func newObservationsRecordCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "record " + prPlaceholder + " <file>",
		Short: "Store what the roles saw outside their units",
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
			// Briefed rather than ReadMeta, as `cr proposals record`
			// is: §4.6.9 resolves every path:line against the head the
			// round was opened at, which is `cr brief`'s to record.
			round, err := briefedRound(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			// §9.3.2: the lines are stamped with the round's head, and
			// resolved against it.
			if err := round.RefuseStale(); err != nil {
				return err
			}
			observed, err := decodeObservations(&round.Meta, args[1])
			if err != nil {
				return headNotFetched(cmd, owner, repo, pr, err)
			}
			for _, seen := range observed {
				seen.Stamp = state.Stamp{Head: round.Head, Round: round.Round}
			}
			held, err := layout.LockPR(owner, repo, pr)
			if err != nil {
				return err
			}
			if err := state.AppendRecords(held, state.FileObservations, observed); err != nil {
				// The lock is released on the way out of every
				// branch, and the write's own failure is what
				// the caller is told about.
				_ = held.Unlock()
				return err
			}
			if err := held.Unlock(); err != nil {
				return err
			}
			return out.emit(&observationsRecordResult{Recorded: observed, Round: round.Round})
		},
	}
}

// decodeObservations reads the file an agent handed `cr observations record`
// and holds every line to §4.6.9 at the round's head.
func decodeObservations(round *state.Meta, file string) ([]*observation.Observation, error) {
	body, err := readInput(file,
		"§4.6.9 has the roles write what they saw outside their units to this "+
			"file before `cr observations record` reads it")
	if err != nil {
		return nil, err
	}
	dir, err := repoDir()
	if err != nil {
		return nil, err
	}
	return observation.Decode(file, body, func(path string) ([]string, bool, error) {
		return git.FileAtRevision(dir, round.Head, path)
	})
}

// roundObservationsOf is every observation the round holds, in the order they
// were recorded. A state directory older than §4.6.9's row has no file, which
// holds as many observations as an empty one.
func roundObservationsOf(l state.Layout, owner, repo string, pr, round int) ([]observation.Observation, error) {
	stored, err := state.ReadRecords[observation.Observation](l, owner, repo, pr, state.FileObservations)
	if errors.Is(err, fs.ErrNotExist) {
		return make([]observation.Observation, 0), nil
	}
	if err != nil {
		return nil, err
	}
	kept := make([]observation.Observation, 0, len(stored))
	for i := range stored {
		if stored[i].Round == round {
			kept = append(kept, stored[i])
		}
	}
	return kept, nil
}
