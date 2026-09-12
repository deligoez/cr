package cli

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// newWaiversCmd groups §11's waiver commands. §7.4.4 requires both scopes to be
// readable and removable, so neither disposition is a one-way door, and these
// two are that door in the other direction.
func newWaiversCmd(out *writer) *cobra.Command {
	cmd := groupCmd("waivers", "Inspect and edit waivers in either scope")
	cmd.AddCommand(newWaiversListCmd(out), newWaiversRemoveCmd())
	return cmd
}

// listedWaiver is one waiver as §7.4.7 prints it: the stored line whole — its
// id, §7.4.1's key, §7.4.5's disposition and §7.4.8's provenance — and the scope
// that disposition gives it.
//
// The scope is derived here at print time and never read from the file, for
// the reason finding.WaiverScope gives: a stored scope that disagreed with the
// disposition would be believed.
type listedWaiver struct {
	finding.WaiverRecord
	// Scope is §7.4.1's scope, as finding.Waiver.Scope derives it.
	Scope string `json:"scope"`
}

// waiversListResult is what `cr waivers list` reports: the scopes whose files
// it read, every waiver they hold, and §9.3.1's comparison when a pull
// request's file was one of them.
//
// The scopes are reported because §7.4.7 makes what was read depend on `--pr`:
// a listing without it that printed no pull-request waiver is not a pull
// request with none, and a reader handed only the waivers could not tell the two
// apart.
type waiversListResult struct {
	// Scopes are the scopes read, in §7.4.4's order: the repository-wide
	// file first, then the pull request's when `--pr` named one.
	Scopes []string `json:"scopes"`
	// Waivers are every waiver those files hold, in the same order and in
	// each file's own order within it.
	Waivers []listedWaiver `json:"waivers"`
	// Honesty carries §9.3.1's head comparison when a pull request's
	// `waivers.ndjson` — per-PR state under §2.3 — was read, and nothing
	// otherwise.
	Honesty []string `json:"honesty"`
}

// Text names how many waivers the scopes hold, then one line per waiver and its
// reason when one was given, then the disclosure.
func (r *waiversListResult) Text(w *writer) string {
	scopes := strings.Join(r.Scopes, " and ") + " scope(s)"
	if len(r.Waivers) == 0 {
		return "no active waiver in the " + scopes + w.disclose("\n", "", r.Honesty...)
	}
	// A capacity hint and nothing more: at most two lines per waiver plus
	// the heading.
	lines := make([]string, 0, 2*len(r.Waivers)+1)
	lines = append(lines, strconv.Itoa(len(r.Waivers))+" active waiver(s) in the "+scopes)
	for i := range r.Waivers {
		listed := &r.Waivers[i]
		lines = append(lines, "  "+w.accent(listed.ID)+" "+listed.Scope+" "+string(listed.Disposition)+
			": "+listed.Class+" at "+listed.Path+" "+string(listed.Side)+" "+listed.ContentHash+
			" (round "+strconv.Itoa(listed.Round)+", pr "+strconv.Itoa(listed.PR)+
			", head "+listed.Head+")")
		if listed.Reason != "" {
			lines = append(lines, "    reason: "+listed.Reason)
		}
	}
	return strings.Join(lines, "\n") + w.disclose("\n", "", r.Honesty...)
}

// newWaiversListCmd runs §11's `cr waivers list --repo <owner/repo> [--pr <n>]`,
// which §7.4.7 has print active waivers with their scope and disposition.
//
// `--pr` selects how many of §7.4.4's two files are read, not which pull
// request a single file is filtered to: with it, both the repository-wide file
// and that pull request's `waivers.ndjson`; without it, the repository-wide
// file alone.
//
// Every waiver a file holds is active. §7.4.7's removal deletes the line rather
// than marking it, so the files are the live set of silences and there is no
// inactive waiver to filter out.
func newWaiversListCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print active waivers with their scope and disposition",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			listed, err := listWaivers(cmd)
			if err != nil {
				return err
			}
			return out.emit(listed)
		},
	}
	cmd.Flags().Int("pr", 0, "also read this pull request's waivers (§7.4.4)")
	return cmd
}

// listWaivers reads the files §7.4.7 names for this invocation.
//
// The pull request's file is read through its round, because it is §2.3's
// per-PR state and §9.3.1 binds every command that reads per-PR state to the
// head comparison. §9.3.5 exempts waivers from being scoped to the current
// round — every waiver in the file is listed whatever round wrote it — and it
// exempts them from nothing else, so a listing over a moved head still says so.
// The repository-wide file is §2.2 state and no pull request's, so a listing
// without `--pr` reads no round and compares nothing.
func listWaivers(cmd *cobra.Command) (*waiversListResult, error) {
	owner, repo, err := repoOf(cmd)
	if err != nil {
		return nil, err
	}
	layout, err := state.Default()
	if err != nil {
		return nil, err
	}
	listed := &waiversListResult{
		Scopes:  []string{finding.ScopeRepository.String()},
		Waivers: make([]listedWaiver, 0),
		Honesty: make([]string, 0),
	}
	wide, err := finding.RepositoryWaivers(layout, owner, repo)
	if err != nil {
		return nil, err
	}
	if !cmd.Flags().Changed("pr") {
		return listed.add(wide)
	}
	pr, err := prFlagOf(cmd)
	if err != nil {
		return nil, err
	}
	round, err := briefedRound(layout, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	here, err := finding.PullRequestWaivers(layout, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	listed.Scopes = append(listed.Scopes, finding.ScopePullRequest.String())
	listed.Honesty = append(listed.Honesty, round.Disclosure())
	return listed.add(append(wide, here...))
}

// add lists each stored waiver beside the scope its disposition gives it, and
// returns the listing it added to.
//
// A stored disposition §7.2 does not have is refused rather than listed under a
// guessed scope: the line was written by something other than cr, and printing
// it as either scope would tell the reader how far a silence reaches on the
// strength of nothing.
func (r *waiversListResult) add(records []finding.WaiverRecord) (*waiversListResult, error) {
	for i := range records {
		scope, err := records[i].Scope()
		if err != nil {
			return nil, err
		}
		r.Waivers = append(r.Waivers, listedWaiver{WaiverRecord: records[i], Scope: scope.String()})
	}
	return r, nil
}

// newWaiversRemoveCmd registers §11's
// `cr waivers remove --repo <owner/repo> [--pr <n>]`, which §7.4.7 has delete
// one waiver from either scope.
//
// It takes the waiver id as a positional even though §11's row does not name
// one, for the reason `cr brief` registers `--intent-file`: §7.4.7 writes the
// invocation as `cr waivers remove <id> --repo <owner/repo> [--pr <number>]`,
// and a surface that followed §11 into the gap would be a `remove` with nothing
// to remove. TestTheCommandSurfaceIsTheSpecTable records it as a deliberate
// addition rather than letting it pass as an oversight.
//
// The behaviour belongs to waivers-remove-command; what is registered here is
// the shape.
func newWaiversRemoveCmd() *cobra.Command {
	cmd := stubCmd("remove <id>", "Remove one waiver from either scope", cobra.ExactArgs(1))
	cmd.Flags().Int("pr", 0, "look for the waiver in this pull request's scope too (§7.4.4)")
	return cmd
}
