package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// reviewResult is what `cr review` reports: §4.6.1's prompts for the round,
// whole, and §4.5.4's report of the lenses that did not run.
//
// The prompts are printed whole rather than counted, because they are the
// command's output in the way §3.7's payload is `cr brief`'s: an agent runs
// them, and a summary would leave it nothing to run.
type reviewResult struct {
	*review.Fanout
}

// Text prints every prompt under a line naming its role and unit, and then the
// lenses that did not run. Nothing is ranked or trimmed: the order is §4.6.1's
// emission order, and every prompt is the text the JSON document carries.
func (r *reviewResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s round %d at head %s: %s prompt(s)\n",
		w.accent("review"), r.Round, r.Head, strconv.Itoa(len(r.Prompts)))
	for i := range r.Prompts {
		prompt := &r.Prompts[i]
		fmt.Fprintf(&out, "\n%s %s on %s\n\n%s", w.accent("prompt"), prompt.Role, prompt.Unit, prompt.Text)
	}
	// §4.6.3's set is printed rather than counted. A count says how many
	// cells are owed and not which, and the reason §4.6.3 has `cr review`
	// report the set at all is that §10.2.2 is checked against it once the
	// roles return.
	fmt.Fprintf(&out, "\n%s %d cell(s) for §10.2.2\n", w.accent("expects"), len(r.Expected))
	for _, cell := range r.Expected {
		fmt.Fprintf(&out, "  %s %s%s%s\n", cell.Unit, cell.Role,
			recordedMark[cell.Recorded], staleMark[cell.StaleByNote])
	}
	out.WriteString(w.disclose("\n", "\n", r.Honesty...))
	return strings.TrimRight(out.String(), "\n")
}

// recordedMark is what follows an expected cell coverage.ndjson already holds
// for the round and head, and nothing for one it does not.
var recordedMark = map[bool]string{true: " (recorded)", false: ""}

// staleMark is what follows an expected cell §4.6.1's note condition holds for.
var staleMark = map[bool]string{true: " (stale by note)", false: ""}

// unknownAxisFlagError reports an `--axis` naming no axis of §1.5.
//
// It is a malformed invocation rather than a malformed file, which is why it is
// not axis.InvalidError: that one names a profile, role, or rule file and
// §11.2 codes it 3, while this is the command line the user just typed. Nothing
// maps it in exit.go, so it takes exitCodeFor's fallback of §11.2's code 2.
type unknownAxisFlagError struct {
	// Value is the flag's value as it was written.
	Value string
}

func (e *unknownAxisFlagError) Error() string {
	return fmt.Sprintf("--axis %q is not an axis id; v0.2 has exactly %s, per §1.5",
		e.Value, strings.Join(axis.IDs(), ", "))
}

// shardFlagError reports a `--shard` that is not `<k>/<n>` with 1 <= k <= n,
// and unitsWithShardError one given together with `--units`. §4.6.1 aborts
// both with exit code 2; nothing maps them in exit.go, so they take
// exitCodeFor's fallback of §11.2's code 2, as unknownAxisFlagError does.
type shardFlagError struct {
	// Value is the flag's value as it was written.
	Value string
}

func (e *shardFlagError) Error() string {
	return fmt.Sprintf("--shard %q is not <k>/<n> with whole numbers 1 <= k <= n, per §4.6.1", e.Value)
}

type unitsWithShardError struct{}

func (*unitsWithShardError) Error() string {
	return "--units and --shard both narrow the units, and §4.6.1 accepts one of them at a time"
}

// parseShard reads `--shard <k>/<n>`.
func parseShard(value string) (*review.Shard, error) {
	k, n, found := strings.Cut(value, "/")
	shard := review.Shard{}
	var errK, errN error
	shard.K, errK = strconv.Atoi(k)
	shard.N, errN = strconv.Atoi(n)
	if !found || errK != nil || errN != nil || shard.K < 1 || shard.K > shard.N {
		return nil, &shardFlagError{Value: value}
	}
	return &shard, nil
}

// narrowing reads §4.6.1's `--units` and `--shard`: the unit ids the run is
// narrowed to, nil when the flag was not given, and the shard beside them.
//
// A `--units` holding no id names the empty id, which is no unit of any round,
// so §4.6.1 rejects it rather than reading it as no flag and emitting every
// unit's prompts.
func narrowing(cmd *cobra.Command, units []string, shard string) ([]string, *review.Shard, error) {
	flags := cmd.Flags()
	if flags.Changed("units") && flags.Changed("shard") {
		return nil, nil, &unitsWithShardError{}
	}
	if flags.Changed("shard") {
		parsed, err := parseShard(shard)
		return nil, parsed, err
	}
	if !flags.Changed("units") {
		return nil, nil, nil
	}
	named := make([]string, 0, len(units))
	for _, id := range units {
		named = append(named, strings.TrimSpace(id))
	}
	if len(named) == 0 {
		named = append(named, "")
	}
	return named, nil, nil
}

// newReviewCmd emits §4.6's fan-out: one prompt per active role and unit.
//
// `cr review` emits the prompts an agent runs, performs no judgement of its
// own, and calls no model (§4.6, invariant 1): the prompts go to the output and
// nothing is sent anywhere. It reads the round `cr brief` recorded, the
// repository it was run from, and the pull request's base. It writes nothing
// inside the repository under review (§2.2); in the state tree it writes the
// fan-out directories, §2.6.1.6's ledger entries for the hits it attaches, and
// one emissions.ndjson line per prompt.
//
// `--axis` narrows the fan-out to one axis's roles, which is the control §4.6.5's
// two passes are run through; `--units` and `--shard` narrow it to units, and
// `--all` lifts §4.6.1's default narrowing to cells the round does not hold.
func newReviewCmd(out *writer) *cobra.Command {
	var only, shard string
	var units []string
	var all bool
	cmd := &cobra.Command{
		Use:   "review " + prPlaceholder,
		Short: "Emit per-role, per-unit review prompts",
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
			// Changed rather than a non-empty value: `--axis ''` names no
			// axis either, and reading it as no flag would emit every prompt.
			if cmd.Flags().Changed("axis") && !axis.Valid(only) {
				return &unknownAxisFlagError{Value: only}
			}
			named, sharded, err := narrowing(cmd, units, shard)
			if err != nil {
				return err
			}
			src, err := reviewSources(owner, repo, pr)
			if err != nil {
				return err
			}
			src.Axis, src.All, src.Units, src.Shard = only, all, named, sharded
			fan, err := review.Run(src)
			if err != nil {
				return headNotFetched(cmd, owner, repo, pr, err)
			}
			return out.emit(&reviewResult{Fanout: fan})
		},
	}
	cmd.Flags().StringVar(&only, "axis", "", "emit only this axis's pass (§4.6.5)")
	cmd.Flags().BoolVar(&all, "all", false, "emit every prompt, cells the round holds included (§4.6.1)")
	cmd.Flags().StringSliceVar(&units, "units", nil, "emit only these unit ids, comma-separated (§4.6.1)")
	cmd.Flags().StringVar(&shard, "shard", "", "emit only shard k of n of the units, given as k/n (§4.6.1)")
	return cmd
}

// reviewSources names what one fan-out reads: the state root, the
// configuration of §2.7, the GitHub client, and the repository directory.
func reviewSources(owner, repo string, pr int) (*review.Sources, error) {
	layout, err := state.Default()
	if err != nil {
		return nil, err
	}
	resolved, err := config.Resolve(config.Sources{
		Environ:      os.Environ(),
		GlobalConfig: layout.Config(),
		RepoConfig:   layout.RepoConfig(owner, repo),
	})
	if err != nil {
		return nil, err
	}
	dir, err := repoDir()
	if err != nil {
		return nil, err
	}
	return &review.Sources{
		Layout:  layout,
		GH:      ghClient(),
		Config:  resolved,
		Owner:   owner,
		Repo:    repo,
		PR:      pr,
		RepoDir: dir,
	}, nil
}
