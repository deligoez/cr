package cli

import (
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/sandbox"
)

// experimentHeader is what `cr test` and `cr probe run` say on standard error
// before the runner starts: what runs, where, and how the sandbox's environment
// files stand against the clone's.
//
// It is printed before the run rather than reported after it, because a suite
// that reads the wrong environment has already done its damage by the time a
// result says so. Standard error keeps §12.1's document on standard output
// whole, and the header goes through the writer's disclose, so `--quiet` does
// not take the line naming an uncopied file.
type experimentHeader struct {
	// command is the runner argv of the run the command was asked for.
	command []string
	// sandbox is the worktree it runs in.
	sandbox string
	// recreated is §5.1.6's recreation of that worktree before this run,
	// and nil when the check admitted it as it stood.
	recreated *sandbox.Recreated
	// env is the clone root's gitignored `.env*` files beside the sandbox.
	env *sandbox.EnvFiles
	// baseline is §5.2.2's unfiltered argv when that baseline runs first,
	// and nil when it does not.
	baseline []string
}

// lines renders the header one line per fact, uncopied files one line each.
func (h *experimentHeader) lines() []string {
	lines := []string{
		"experiment " + strings.Join(h.command, " "),
		"  sandbox    " + h.sandbox,
	}
	if h.recreated != nil {
		lines = append(lines, "  recreated  per §5.1.6: "+h.recreated.Reason)
	}
	lines = append(lines,
		"  env files  "+listed(h.env.Ignored)+" gitignored at the clone root",
		"  in sandbox "+listed(h.env.Held),
	)
	for _, sentence := range h.env.Disclosures() {
		lines = append(lines, "  not copied "+sentence)
	}
	if h.baseline != nil {
		lines = append(lines, "  baseline   the whole suite runs first as the §5.2.2 baseline: "+
			strings.Join(h.baseline, " "))
	}
	return lines
}

// announce writes the header to stderr.
func (h *experimentHeader) announce(out *writer, stderr io.Writer) error {
	_, err := io.WriteString(stderr, out.disclose("", "\n", h.lines()...))
	return err
}

// announceExperiment compares the sandbox ready admitted with src's clone and
// prints the header for a run of command, and of baseline first when it is not
// nil. It returns the header's env-file sentences, which the command's document
// carries under honesty too, so a caller that keeps only standard output is
// told what the header said.
func announceExperiment(
	cmd *cobra.Command, out *writer, src *sandbox.Sources, ready *sandbox.Ready, command, baseline []string,
) ([]string, error) {
	env, err := sandbox.CompareEnvFiles(src, ready.Path)
	if err != nil {
		return nil, err
	}
	header := &experimentHeader{
		command: command, sandbox: ready.Path, recreated: ready.Recreated, env: env, baseline: baseline,
	}
	return env.Disclosures(), header.announce(out, cmd.ErrOrStderr())
}

// ensureAnnounced is `cr test`'s §5.1.6 check, which rebuilds a sandbox that
// fails it, followed by the header for a run of command in the sandbox it
// admitted, and returns that sandbox with the header's env-file sentences. A
// probe announces later, once §5.6.1's lock says which baselines the head still
// lacks.
func ensureAnnounced(
	cmd *cobra.Command, out *writer, src *sandbox.Sources, glob string, command []string,
) (*sandbox.Ready, []string, error) {
	ready, err := sandbox.Ensure(src, glob)
	if err != nil {
		return nil, nil, headNotFetched(cmd, src.Owner, src.Repo, src.PR, err)
	}
	uncopied, err := announceExperiment(cmd, out, src, ready, command, nil)
	return ready, uncopied, err
}
