package cli

import (
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// staleRoles is §2.5.2's honesty report for the role corpus one command
// resolved: a sentence per ejected role file that is, byte for byte, a role an
// earlier release shipped and not this build's, naming that release and
// `cr init`.
//
// It resolves the corpus again rather than threading the one the command
// already loaded up through every call it sits behind, for the reason
// staleProfile reads the profile file again: the corpus is several calls down,
// or resolved only on some paths, and a role file is four small documents. A
// corpus that does not resolve is answered with nothing — the command's own
// resolution reports that failure with §2.5.3's exit code, and a disclosure is
// not the place to refuse a run.
func staleRoles(l state.Layout, owner, repo string) []string {
	corpus, err := role.Resolve(l.RepoRolesDir(owner, repo), l.RolesDir())
	if err != nil {
		return []string{}
	}
	return role.StaleDisclosures(corpus)
}
