// Package state derives every path cr writes and creates the state tree.
//
// All state lives under one root, per spec/0.1.0.md §2.2. cr never writes
// inside the repository under review, so every writer resolves its path through
// a Layout rather than joining path segments of its own.
package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// HomeEnv overrides the state root. Without it the root is <home>/.cr.
const HomeEnv = "CR_HOME"

// dirName is the state directory created inside the user's home directory.
const dirName = ".cr"

// Permissions for the state tree. Findings, drafts, and issue text are private
// working material, so nothing under the root is group- or world-readable.
const (
	dirPerm  fs.FileMode = 0o700
	filePerm fs.FileMode = 0o600
)

// emptyConfig is the initial content of a configuration file: no overrides, so
// every setting falls through to the layer below it (§2.7).
const emptyConfig = "{}\n"

// Layout resolves every path of §2.2 against one root.
type Layout struct {
	root string
}

// New returns the layout rooted at root.
func New(root string) Layout {
	return Layout{root: root}
}

// Default resolves the root from HomeEnv, falling back to <home>/.cr. The
// override keeps the root injectable, so a test never touches a real ~/.cr.
func Default() (Layout, error) {
	if root := os.Getenv(HomeEnv); root != "" {
		return New(root), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Layout{}, fmt.Errorf("cannot locate the home directory: %w", err)
	}
	return New(filepath.Join(home, dirName)), nil
}

// Root is the state root itself.
func (l Layout) Root() string { return l.root }

// Config is the global configuration file.
func (l Layout) Config() string { return filepath.Join(l.root, "config.json") }

// ProfilesDir holds the mechanical profiles (§2.4).
func (l Layout) ProfilesDir() string { return filepath.Join(l.root, "profiles") }

// Profile is one global profile file.
func (l Layout) Profile(id string) string { return filepath.Join(l.ProfilesDir(), id+".json") }

// RolesDir holds the judgement roles (§2.5).
func (l Layout) RolesDir() string { return filepath.Join(l.root, "roles") }

// Role is one global role file.
func (l Layout) Role(id string) string { return filepath.Join(l.RolesDir(), id+".json") }

// RulesDir holds the global rules (§2.6).
func (l Layout) RulesDir() string { return filepath.Join(l.root, "rules") }

// Rule is one global rule file.
func (l Layout) Rule(id string) string { return filepath.Join(l.RulesDir(), id+".json") }

// ReposDir holds the per-repository overrides.
func (l Layout) ReposDir() string { return filepath.Join(l.root, "repos") }

// RepoDir is one repository's override directory.
func (l Layout) RepoDir(owner, repo string) string {
	return filepath.Join(l.ReposDir(), owner, repo)
}

// RepoConfig is a repository's configuration overrides.
func (l Layout) RepoConfig(owner, repo string) string {
	return filepath.Join(l.RepoDir(owner, repo), "config.json")
}

// RepoRolesDir holds a repository's role overrides.
func (l Layout) RepoRolesDir(owner, repo string) string {
	return filepath.Join(l.RepoDir(owner, repo), "roles")
}

// RepoRole is one repository-scoped role override.
func (l Layout) RepoRole(owner, repo, id string) string {
	return filepath.Join(l.RepoRolesDir(owner, repo), id+".json")
}

// RepoRulesDir holds a repository's rule overrides.
func (l Layout) RepoRulesDir(owner, repo string) string {
	return filepath.Join(l.RepoDir(owner, repo), "rules")
}

// RepoRule is one repository-scoped rule override.
func (l Layout) RepoRule(owner, repo, id string) string {
	return filepath.Join(l.RepoRulesDir(owner, repo), id+".json")
}

// RepoTriage holds a repository's triage events across pull requests (§7.3).
func (l Layout) RepoTriage(owner, repo string) string {
	return filepath.Join(l.RepoDir(owner, repo), "triage.ndjson")
}

// RepoRuleStats holds a repository's per-rule hits, records, and dismissals
// (§2.6.1).
func (l Layout) RepoRuleStats(owner, repo string) string {
	return filepath.Join(l.RepoDir(owner, repo), "rule-stats.ndjson")
}

// StateDir holds the per-pull-request state of every repository.
func (l Layout) StateDir() string { return filepath.Join(l.root, "state") }

// RepoStateDir holds one repository's per-pull-request state.
func (l Layout) RepoStateDir(owner, repo string) string {
	return filepath.Join(l.StateDir(), owner, repo)
}

// PRDir is one pull request's state directory (§2.3).
func (l Layout) PRDir(owner, repo string, pr int) string {
	return filepath.Join(l.RepoStateDir(owner, repo), "pr-"+strconv.Itoa(pr))
}

// ContextDir holds the out-of-band context store (§3.6).
func (l Layout) ContextDir() string { return filepath.Join(l.root, "context") }

// ContextFile is one issue's context store.
func (l Layout) ContextFile(issueKey string) string {
	return filepath.Join(l.ContextDir(), issueKey+".ndjson")
}

// WaiversDir holds the repository-wide waivers (§7.4).
func (l Layout) WaiversDir() string { return filepath.Join(l.root, "waivers") }

// WaiversFile is one repository's waivers.
func (l Layout) WaiversFile(owner, repo string) string {
	return filepath.Join(l.WaiversDir(), owner, repo+".ndjson")
}

// LocksDir holds the advisory locks (§5.6).
func (l Layout) LocksDir() string { return filepath.Join(l.root, "locks") }

// globalDirs are the directories that exist independently of any repository,
// pull request, or issue.
func (l Layout) globalDirs() []string {
	return []string{
		l.root,
		l.ProfilesDir(),
		l.RolesDir(),
		l.RulesDir(),
		l.ReposDir(),
		l.StateDir(),
		l.ContextDir(),
		l.WaiversDir(),
		l.LocksDir(),
	}
}

// Init creates the global part of the §2.2 tree. It is idempotent: an existing
// directory or configuration file is left exactly as it was.
func (l Layout) Init() error {
	if err := makeDirs(l.globalDirs()); err != nil {
		return err
	}
	return touchFile(l.Config(), emptyConfig)
}

// EnsureRepo creates the repository-scoped paths of §2.2 for one repository.
// The owner and repository are only known once a command names them, so they
// are created here rather than by Init.
func (l Layout) EnsureRepo(owner, repo string) error {
	dirs := []string{
		l.RepoDir(owner, repo),
		l.RepoRolesDir(owner, repo),
		l.RepoRulesDir(owner, repo),
		l.RepoStateDir(owner, repo),
		filepath.Dir(l.WaiversFile(owner, repo)),
	}
	if err := makeDirs(dirs); err != nil {
		return err
	}
	if err := touchFile(l.RepoConfig(owner, repo), emptyConfig); err != nil {
		return err
	}
	for _, path := range []string{
		l.RepoTriage(owner, repo),
		l.RepoRuleStats(owner, repo),
		l.WaiversFile(owner, repo),
	} {
		if err := touchFile(path, ""); err != nil {
			return err
		}
	}
	return nil
}

// EnsurePR creates one pull request's state directory, and the repository tree
// that contains it.
func (l Layout) EnsurePR(owner, repo string, pr int) error {
	if err := l.EnsureRepo(owner, repo); err != nil {
		return err
	}
	return makeDirs([]string{l.PRDir(owner, repo, pr)})
}

// EnsureContext creates one issue's context store.
func (l Layout) EnsureContext(issueKey string) error {
	if err := makeDirs([]string{l.ContextDir()}); err != nil {
		return err
	}
	return touchFile(l.ContextFile(issueKey), "")
}

// makeDirs creates every directory, parents included.
func makeDirs(dirs []string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return fmt.Errorf("cannot create directory %s: %w", dir, err)
		}
	}
	return nil
}

// touchFile creates path with the given initial content, and leaves an existing
// file untouched so repeated creation never discards recorded state.
func touchFile(path, initial string) error {
	switch _, err := os.Stat(path); {
	case err == nil:
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot inspect %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(initial), filePerm); err != nil {
		return fmt.Errorf("cannot create %s: %w", path, err)
	}
	return nil
}
