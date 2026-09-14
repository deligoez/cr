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
	"strings"
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

// ProfileStem refuses a profile id that is not one file stem, so the path
// Profile joins it into stays a file of ProfilesDir. §2.4 makes a profile's id
// its file stem, so an id holding a separator, naming . or .., or rooted
// elsewhere names no profile §2.2's tree holds.
func ProfileStem(id string) error {
	if id == "." || id == ".." || strings.ContainsAny(id, `/\`) || filepath.IsAbs(id) {
		return &ProfileIDError{ID: id}
	}
	return nil
}

// ProfileIDError is a stored profile id ProfileStem refuses. It is carried
// inside the *FileError naming the file that recorded it, which is what sets
// the exit code and the hint; the type lets a door that reads a missing file as
// a missing round tell this file apart as one that is present and unusable.
type ProfileIDError struct {
	// ID is the profile id as the file recorded it.
	ID string
}

func (e *ProfileIDError) Error() string {
	return fmt.Sprintf("profile_id %q is not one profile's file stem: §2.4 makes the id the "+
		"name of a file directly under ~/.cr/profiles, so it holds no path separator and is not . or ..", e.ID)
}

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

// PRFile is one file inside a pull request's state directory (§2.3).
func (l Layout) PRFile(owner, repo string, pr int, name string) string {
	return filepath.Join(l.PRDir(owner, repo, pr), name)
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

// PRLockFile is the advisory lock guarding one pull request's state (§2.3.1).
// It is nested by owner and repository, and the probe lock of §5.6 — which is
// named after a repository path and a profile id — is nested under
// DirProbeLocks, so the two trees share the locks directory and no name.
func (l Layout) PRLockFile(owner, repo string, pr int) string {
	return filepath.Join(l.LocksDir(), owner, repo, "pr-"+strconv.Itoa(pr)+".lock")
}

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

// EnsureProfile writes one shipped profile file into the profiles directory,
// leaving an existing file exactly as it was: §2.4 makes a profile the user's
// to edit, so re-running `cr init` must never discard an edit. Init creates the
// directory, and this method creates it too so the caller need not order them.
func (l Layout) EnsureProfile(id, content string) error {
	if err := makeDirs([]string{l.ProfilesDir()}); err != nil {
		return err
	}
	return touchFile(l.Profile(id), content)
}

// EnsureRole writes one shipped role file into the roles directory, leaving an
// existing file exactly as it was. §2.5.2 ejects the defaults as editable
// files, so a second eject must complete a tree and never revert an edit: the
// file is the user's once it is written, and cr overwriting it would spend a
// user's work to restore a default they still have in the binary. Init creates
// the directory, and this method creates it too so the caller need not order
// them.
func (l Layout) EnsureRole(id, content string) error {
	if err := makeDirs([]string{l.RolesDir()}); err != nil {
		return err
	}
	return touchFile(l.Role(id), content)
}

// EnsureRepo creates the repository-scoped paths of §2.2 for one repository.
// The owner and repository are only known once a command names them, so they
// are created here rather than by Init.
func (l Layout) EnsureRepo(owner, repo string) error {
	if err := l.containRepo(owner, repo); err != nil {
		return err
	}
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

// EnsurePR creates one pull request's state directory, the repository tree that
// contains it, and every file of the §2.3 table. LockPR creates the directory
// itself; creating the files inside it is a write to per-PR state, so it happens
// under the exclusive lock §2.3.1 requires of every write.
func (l Layout) EnsurePR(owner, repo string, pr int) error {
	if err := l.EnsureRepo(owner, repo); err != nil {
		return err
	}
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	// Joined rather than branched: the lock is released whether or not the
	// files were created, and neither failure is traded away for the other.
	return errors.Join(held.createFiles(newMeta(owner, repo, pr)), held.Unlock())
}

// EnsureContext creates one issue's context store.
func (l Layout) EnsureContext(issueKey string) error {
	if err := makeDirs([]string{l.ContextDir()}); err != nil {
		return err
	}
	return touchFile(l.ContextFile(issueKey), "")
}

// dirHint is §12.4's next actionable step for a directory of §2.2's tree that
// could not be created.
//
// It is a file failure and §11.2 codes it 3. Measured by audit round 1: under a
// root whose locks path was a file, LockPR's bare `cannot create directory`
// exited 2 and told the user to retype a command line that was right.
const dirHint = "§2.2 keeps all of cr's state under one root; check that every parent of " +
	"the directory the message names is a directory cr can write to"

// makeDirs creates every directory, parents included.
func makeDirs(dirs []string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return FileFailure("create directory", dir, dirHint, err)
		}
	}
	return nil
}

// OutsideRootError reports a path of §2.2's tree that resolves outside the
// directory it belongs under.
//
// Every path here is joined from segments a caller supplies — an owner and a
// repository out of `--repo` or a remote, a file name — and filepath.Join
// resolves a `..` among them rather than refusing it. Measured by audit round
// 1: LockPR("..", "..", 1) and a Write under a root at outer/crhome created
// outer/pr-1.lock and outer/pr-1/meta.json. §2.2 keeps all of cr's state under
// one root, so a path that leaves it is refused before anything is created or
// read there.
type OutsideRootError struct {
	// Path is where the segments led, and Under is the directory it had
	// to stay inside.
	Path, Under string
}

func (e *OutsideRootError) Error() string {
	return fmt.Sprintf("refusing %s: it resolves outside %s, and §2.2 keeps all of cr's state under one root",
		e.Path, e.Under)
}

// contain refuses every path that does not resolve strictly inside under.
//
// under is the tree the path belongs to rather than the root alone, so an
// owner of `..` cannot aim a pull request's state into the locks or profiles
// directory either.
func contain(under string, paths ...string) error {
	for _, path := range paths {
		rel, err := filepath.Rel(under, path)
		if err != nil || rel == "." || rel == ".." ||
			strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return &OutsideRootError{Path: path, Under: under}
		}
	}
	return nil
}

// containRepo refuses an owner and repository whose §2.2 paths leave the trees
// they are nested in.
//
// Every repository-keyed path of the tree is <tree>/<owner>/<repo> followed by
// a name of cr's own, so the repository's directory in one of those trees
// answers for all of them: a pair that stays strictly inside state/ stays
// inside repos/, locks/ and waivers/ by the same two segments.
func (l Layout) containRepo(owner, repo string) error {
	return contain(l.StateDir(), l.RepoStateDir(owner, repo))
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
