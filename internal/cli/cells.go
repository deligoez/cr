package cli

import (
	"slices"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// cellKeyFields is §4.5.6's key, by the JSON names coverage.ndjson holds them
// under: a cell sits at one `(unit, role)`, and that pair is what a recording
// replaces. It is declared here, beside the command §4.5.6 gives the rule to,
// rather than inside internal/state, which is generic over all eight files of
// §2.3.3 and knows nothing about what makes a cell a cell.
var cellKeyFields = []string{"unit", "role"}

// cellsRecordResult is what `cr cells record` has to report: the cells it
// stored, whole, and the round they were filled in.
//
// They are printed rather than counted for the reason claimsRecordResult gives:
// §2.3.3 stamps head and round and §4.5.5's `unit_hash` is cr's to write, so
// the cell the agent handed in and the cell coverage.ndjson now holds are not
// the same document. `unit_hash` in particular is the value §10.2.2 compares
// against next, and no caller can derive it from the file it wrote.
type cellsRecordResult struct {
	// Recorded are the cells as they were written, in file order.
	Recorded []*coverage.Cell `json:"recorded"`
	// Round is the round whose cells were replaced. §4.5.6 replaces only
	// the named `(unit, role)` pairs and §9.3.5 scopes that to the round,
	// so it is reported: it is the difference between a run that replaced
	// four cells and one that replaced a file.
	Round int `json:"round"`
}

// Text names how many cells were stored and the round they stand in. The cells
// themselves came from the caller's own file, so printing them into a terminal
// would repeat what the caller has.
func (r *cellsRecordResult) Text(w *writer) string {
	return "recorded " + w.accent(strconv.Itoa(len(r.Recorded))) +
		" cell(s) in round " + strconv.Itoa(r.Round)
}

// newCellsCmd groups the coverage-cell commands of §11. It runs nothing itself,
// so an invocation naming no subcommand prints the help rather than doing
// something the user did not ask for.
func newCellsCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cells",
		Short: "Manage the coverage cells the roles filled",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newCellsRecordCmd(out))
	return cmd
}

// newCellsRecordCmd stores the coverage cells the roles filled (§11, §4.5.6).
//
// The whole file is validated before anything is written, for the reason
// `cr record` and `cr claims record` give: §4.5.6 rejects a cell with exit code
// 1, and a rejection that had already replaced the cells above it would have
// discarded a role's earlier answer on the strength of a file the agent is
// about to correct. coverage.Decode refuses the first faulty line and returns
// no cells at all, so the write below either happens whole or does not happen.
//
// Nothing here re-reads §4.5.5's field list. coverage.Decode is the one door:
// it holds every line to the fields, to §4.5.6's two rejections, to §2.3.3's
// reserved head and round, and to §4.5.5's computed `unit_hash`, naming the
// file, the one-based line and the field in what it refuses, and internal/cli
// maps that onto §11.2's code 1.
func newCellsRecordCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "record " + prPlaceholder + " <file>",
		Short: "Store the coverage cells the roles filled",
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
			// Briefed rather than ReadMeta: §4.5.6 checks every
			// cell's unit against the units of the round and its
			// role against §4.5.1's active set, and units.ndjson
			// and meta.json are both `cr brief`'s to write per
			// §3.7. A pull request no round has been opened on
			// would refuse every cell for naming an unknown unit
			// rather than for the reason it was actually refused.
			// §11.2 codes that 4.
			round, err := briefedRound(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			// §9.3.2: this command writes coverage.ndjson, and
			// §4.5.5's `unit_hash` is stamped from units the round
			// formed at its own head. A head that moved under it
			// refuses here.
			if err := round.RefuseStale(); err != nil {
				return err
			}
			formed, err := roundUnitsOf(layout, owner, repo, pr, round.Round)
			if err != nil {
				return err
			}
			active, err := activeRoles(layout, owner, repo, round.ActiveRoles)
			if err != nil {
				return err
			}
			body, err := readInput(args[1],
				"§4.5.6 has the roles write the coverage cells they filled to this "+
					"file before `cr cells record` reads it")
			if err != nil {
				return err
			}
			cells, err := coverage.Decode(args[1], body, roundUnitIDs(formed), active)
			if err != nil {
				return err
			}
			// §4.5.5's `unit_hash` is the hash the cell was filled
			// for, and §10.2.2 compares it against the unit's
			// current one. It is taken from units.ndjson rather
			// than from the line, because a hash the agent supplied
			// would be the guard and the thing being guarded at
			// once. The unit is known to be one of the round's:
			// coverage.Decode refused the file otherwise.
			stampUnitHashes(cells, formed)
			held, err := layout.LockPR(owner, repo, pr)
			if err != nil {
				return err
			}
			stamp := state.Stamp{Head: round.Head, Round: round.Round}
			if err := state.ReplaceStampedKeys(
				held, state.FileCoverage, stamp, cells, cellKeyFields,
			); err != nil {
				// The lock is released on the way out of every
				// branch, and the write's own failure is what
				// the caller is told about.
				_ = held.Unlock()
				return err
			}
			if err := held.Unlock(); err != nil {
				return err
			}
			return out.emit(&cellsRecordResult{Recorded: cells, Round: round.Round})
		},
	}
}

// activeRoles is §4.5.1's active set as this round settled it: the role ids
// meta.json carries, resolved against §2.5.5's corpus so each one arrives with
// the axis §4.5.5 routes its `coverage` object on.
//
// The set is read out of meta.json rather than recomputed here, which is the
// whole of what makes `cr cells record` agree with `cr status` and with
// `cr review`: `cr brief` settles it once per round, and a command that derived
// its own answer could reject a cell the round's own fan-out had asked for.
//
// A stored id the corpus no longer resolves is left out rather than reported.
// It is a role file that was edited or removed between the brief and the
// recording, so there is no axis to hold its cells to and no prompt it could
// still be filling; a cell naming it is refused by §4.5.6 with the active set
// named, which is the message that leads the user to re-run `cr brief`.
func activeRoles(l state.Layout, owner, repo string, active []string) ([]role.Role, error) {
	corpus, err := role.Resolve(l.RepoRolesDir(owner, repo), l.RolesDir())
	if err != nil {
		return nil, err
	}
	roles := make([]role.Role, 0, len(active))
	for i := range corpus {
		if slices.Contains(active, corpus[i].Role.ID) {
			roles = append(roles, corpus[i].Role)
		}
	}
	return roles, nil
}

// stampUnitHashes writes §4.5.5's `unit_hash` onto every cell from the unit it
// sits at, so the value §10.2.2 compares has one author and it is never the
// agent.
func stampUnitHashes(cells []*coverage.Cell, formed []roundUnit) {
	hashes := make(map[string]string, len(formed))
	for i := range formed {
		hashes[formed[i].ID] = formed[i].Hash
	}
	for _, cell := range cells {
		cell.UnitHash = hashes[cell.Unit]
	}
}
