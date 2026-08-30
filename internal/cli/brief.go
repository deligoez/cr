package cli

import (
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/brief"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
)

// The two seams `cr brief` reaches the outside world through.
//
// They are variables rather than calls written into the command for one
// reason: §3.7's payload is assembled out of a pull request cr cannot invent
// and a repository cr cannot create, so a test of the command that could not
// substitute either would be a test that either reached the network or tested
// nothing. internal/gh already makes the transport injectable through
// gh.WithRunner; this is the same seam, one level up, and the working
// directory beside it.
//
// Neither widens what cr can do. gh.New is the read client, and §2.1.2's
// boundary refuses a write whichever runner is behind it.
var (
	// ghClient reads the pull request and its threads.
	ghClient = gh.New
	// repoDir is the repository under review: the directory cr was run
	// from. §11.1 makes `--repo` the override for repository detection and
	// not for the checkout, and §2.2 forbids writing inside it either way.
	repoDir = os.Getwd
)

// briefResult is what `cr brief` reports: §3.7's payload, whole.
//
// The payload is embedded rather than summarised because §3.7 has cr assemble
// and print six things, and every one of them is a fact the agent orients on.
// A count of the units would leave the agent to re-derive the units themselves,
// which §3.7 wrote this command to stop it doing.
type briefResult struct {
	*brief.Brief
}

// Text names what the run recorded: the pull request, the round it stands in,
// and the head that round is oriented on.
func (r *briefResult) Text(w *writer) string {
	return "briefed " + w.accent(r.Owner+"/"+r.Repo+"#"+strconv.Itoa(r.PR)) +
		" in round " + strconv.Itoa(r.Round) +
		" at head " + r.Head +
		": " + strconv.Itoa(len(r.Units)) + " unit(s), " +
		strconv.Itoa(len(r.Threads)) + " thread(s)"
}

// newBriefCmd assembles and prints the orientation payload of §3.7, and records
// the derived inputs the rest of cr reads as authoritative.
//
// The command itself resolves nothing: it names the pull request, the state
// root, the configuration of §2.7, the repository directory, and §3.1's issue
// source, and hands all five to internal/brief. §3.7's six items and the writes
// that go with them are one act, and splitting them across a command and a
// package would put half of §3.7 in a file that also parses flags.
func newBriefCmd(out *writer) *cobra.Command {
	var issue, intentFile string

	cmd := &cobra.Command{
		Use:   "brief " + prPlaceholder,
		Short: "Print the orientation payload for a pull request",
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
			resolved, err := config.Resolve(config.Sources{
				Environ:      os.Environ(),
				GlobalConfig: layout.Config(),
				RepoConfig:   layout.RepoConfig(owner, repo),
			})
			if err != nil {
				return err
			}
			dir, err := repoDir()
			if err != nil {
				return err
			}
			// §3.1.4's file bypasses the tracker command rather than
			// outranking it, which intentSource is where cr settles.
			source, err := intentSource(layout, owner, repo, intentFile)
			if err != nil {
				return err
			}
			assembled, err := brief.Run(&brief.Sources{
				Layout:    layout,
				GH:        ghClient(),
				Config:    resolved,
				Owner:     owner,
				Repo:      repo,
				PR:        pr,
				RepoDir:   dir,
				IssueFlag: issue,
				Intent:    source,
			})
			if err != nil {
				return err
			}
			return out.emit(&briefResult{Brief: assembled})
		},
	}
	cmd.Flags().StringVar(&issue, "issue", "",
		"the issue key, which §3.2 consults before the branch, title, and body")
	cmd.Flags().StringVar(&intentFile, "intent-file", "",
		"read the issue text from this file instead of running the tracker command")

	return cmd
}
