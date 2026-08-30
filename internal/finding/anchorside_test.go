package finding

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// The file the change edits, as the merge base holds it: five lines, of which
// the fourth is the one the change rewrites.
const mergeBaseMoney = `package app

func Total(sum, discount int) int {
	return sum - discount
}
`

// The same file at the head: nine lines, the last four of which exist in no
// tree but this one.
const headMoney = `package app

func Total(sum, discount int) int {
	return sum - discount + tax(sum)
}

func Subtotal(sum int) int {
	return sum
}
`

// twoTrees builds a real repository in the shape §6.1.2 legislates about and
// returns the pair of readers an anchor resolves against.
//
// It is a repository and not a pair of maps because the two trees are what a
// run has: internal/git resolves the merge base and reads each blob out of the
// object database, and a stub of that pair would be a stub of exactly the thing
// the section is about — which commit each side reaches for.
//
// The branch does all three things a change can do to a file, because each one
// separates the two trees somewhere a swapped reader would still look right:
// it rewrites a line, adds a file the merge base never had, and deletes one the
// head no longer has.
func twoTrees(t *testing.T) Trees {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoErrorf(t, err, "git %s: %s", strings.Join(args, " "), out)
	}
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}

	// The identity is repository-local: a machine with no global user.email
	// cannot commit at all, and a test must not write a developer's own
	// configuration.
	run("init", "--quiet", "--initial-branch=main")
	run("config", "user.email", "fixture@cr.test")
	run("config", "user.name", "cr fixture")
	run("config", "commit.gpgsign", "false")

	write("app/money.go", mergeBaseMoney)
	write("app/legacy.go", "package app\n\nfunc Legacy() {}\n")
	run("add", "--all")
	run("commit", "--quiet", "-m", "what the branch left")

	run("checkout", "--quiet", "-b", "feature")
	write("app/money.go", headMoney)
	require.NoError(t, os.Remove(filepath.Join(dir, "app", "legacy.go")))
	write("app/tax.go", "package app\n\nfunc tax(sum int) int {\n\treturn sum / 10\n}\n")
	run("add", "--all")
	run("commit", "--quiet", "-m", "the change under review")

	base, err := git.MergeBase(dir, "main", "feature")
	require.NoError(t, err)
	return Trees{
		Head: func(path string) ([]string, bool, error) {
			return git.FileAtRevision(dir, "feature", path)
		},
		MergeBase: func(path string) ([]string, bool, error) {
			return git.FileAtRevision(dir, base, path)
		},
	}
}

// resolves asserts that one anchor resolves against the tree its side names.
func resolves(t *testing.T, trees Trees, path string, side git.Side, startLine, line int) {
	t.Helper()
	assert.NoErrorf(t, ResolveAnchor(trees, state.FileFindings, 7, &Anchor{
		Path: path, Side: side, StartLine: startLine, Line: line,
	}), "%s %s:%d..%d", side, path, startLine, line)
}

// refusesToResolve asserts the opposite, in the shape §6.1.3 gives every record
// rejection: RejectedRecordError, which internal/cli maps onto exit code 1,
// naming the line the record sits on and the field the user has to look at.
func refusesToResolve(t *testing.T, trees Trees, anchor *Anchor) *RejectedRecordError {
	t.Helper()
	var rejected *RejectedRecordError
	require.ErrorAs(t, ResolveAnchor(trees, state.FileFindings, 7, anchor), &rejected)
	assert.Equal(t, "anchor", rejected.Field)
	assert.Equal(t, 7, rejected.Line)
	assert.Equal(t, state.FileFindings, rejected.File)
	return rejected
}

// §6.1.2 sends each side to its own tree: a RIGHT anchor resolves against the
// head, a LEFT one against the merge base. §9.2.1 is why the two differ — RIGHT
// anchors a line in the head and LEFT a removed line, and a removed line is
// precisely the line the head no longer has.
//
// The pair of readers being right is not shown by either resolving on its own:
// most lines of most files sit at the same number in both trees, so a swapped
// pair passes anything taken from the middle of an unchanged file. What
// separates them is a file only one tree holds, and the branch here holds one
// of each — so each side resolves the file its own tree has and is refused the
// other's, naming the tree it looked in.
func TestARightAnchorResolvesAgainstTheHeadAndALeftOneAgainstTheMergeBase(t *testing.T) {
	trees := twoTrees(t)

	resolves(t, trees, "app/money.go", git.Right, 7, 9)
	resolves(t, trees, "app/money.go", git.Left, 4, 4)

	// The file the change added exists at the head and nowhere else.
	resolves(t, trees, "app/tax.go", git.Right, 3, 5)
	assert.Contains(t,
		refusesToResolve(t, trees, &Anchor{
			Path: "app/tax.go", Side: git.Left, StartLine: 3, Line: 5,
		}).Error(),
		`"app/tax.go", which the merge base does not hold as a file`)

	// The file the change deleted exists in the merge base and nowhere else,
	// which is the same claim from the other side: a record about a deletion
	// is exactly what §9.2.1 gives LEFT to.
	resolves(t, trees, "app/legacy.go", git.Left, 3, 3)
	assert.Contains(t,
		refusesToResolve(t, trees, &Anchor{
			Path: "app/legacy.go", Side: git.Right, StartLine: 3, Line: 3,
		}).Error(),
		`"app/legacy.go", which the head under review does not hold as a file`)
}

// A LEFT anchor pointing at a line only the head has is the failure §6.1.2's
// two trees exist to produce.
//
// It is the shape a record about added code takes when its side is wrong, and
// it is invisible to every check that reads the anchor alone: the path is a
// path of the change, the range runs forwards, and the numbers are lines of a
// real file. Only the merge base can say that they are not lines of it — the
// change added four lines to this file, so the head's line 7 is past the end of
// the merge base's copy, and §9.2.1's removed line is not there to be anchored.
//
// The boundary either side of the merge base's last line is asserted because
// that is where a reader off by one, or one measuring against the wrong file,
// stops agreeing with this: line 5 of the merge base is a line and line 6 is
// not, while at the head both are.
func TestALeftAnchorPointingAtAHeadOnlyLineIsRefused(t *testing.T) {
	trees := twoTrees(t)

	assert.Contains(t,
		refusesToResolve(t, trees, &Anchor{
			Path: "app/money.go", Side: git.Left, StartLine: 7, Line: 9,
		}).Error(),
		`runs to line 9 of "app/money.go", which holds 5 lines at the merge base`,
		"the user is told which tree was read and how far it goes")

	resolves(t, trees, "app/money.go", git.Left, 5, 5)
	assert.Contains(t,
		refusesToResolve(t, trees, &Anchor{
			Path: "app/money.go", Side: git.Left, StartLine: 6, Line: 6,
		}).Error(),
		"runs to line 6")

	// The same two ranges on the side §9.2.1 gives the head resolve, so what
	// the assertions above measure is the tree and not the range.
	resolves(t, trees, "app/money.go", git.Right, 7, 9)
	resolves(t, trees, "app/money.go", git.Right, 6, 6)
}

// A side §9.2 does not define names no tree, so ResolveAnchor opens nothing.
//
// ValidateAnchor already closes the vocabulary on every door a record comes in
// through, which is what makes this worth asserting rather than assuming:
// ResolveAnchor is exported and answers on its own account, and a function that
// trusted an upstream check would reach for a nil reader the day it is called
// anywhere else. Refusing is also the only honest answer available — §6.1.2
// names one tree per side, so a third side has none, and picking one would be
// cr guessing which tree a record it cannot read meant.
//
// The readers fail the test if they are called at all, because "refused" and
// "read the head anyway and happened not to find it" are the same error to a
// caller and different faults.
func TestASideNamingNeitherTreeResolvesAgainstNothing(t *testing.T) {
	unread := func(path string) ([]string, bool, error) {
		require.FailNow(t, "no tree is opened for a side §9.2 does not define", "read %q", path)
		return nil, false, nil
	}
	trees := Trees{Head: unread, MergeBase: unread}

	for _, side := range []git.Side{"", "right", "Right", "RIGHT ", "MIDDLE", "BOTH"} {
		anchor := anAnchor()
		anchor.Side = side
		rejected := refusesToResolve(t, trees, &anchor)
		assert.Contains(t, rejected.Error(), string(git.Right))
		assert.Contains(t, rejected.Error(), string(git.Left))
		assert.Contains(t, rejected.Error(), string(side),
			"the value is quoted back, because a side that is nearly right is invisible otherwise")
	}
}
