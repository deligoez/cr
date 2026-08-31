package state

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// §5.1.1 puts the sandbox at ~/.cr/state/<owner>/<repo>/pr-<n>/sandbox/, which
// is one directory inside the pull request's own state directory of §2.3.
//
// The path is asserted whole rather than as a join onto PRDir, because the two
// halves fail differently: a sandbox under the wrong pull request would still
// sit under PRDir, and a sandbox under some sibling of the state root would
// still end in `sandbox`. §5.1.1 fixes both halves and this reads both.
func TestTheSandboxSitsInThePullRequestsStateDirectory(t *testing.T) {
	root := filepath.Join("home", ".cr")
	l := New(root)

	assert.Equal(t,
		filepath.Join(root, "state", "acme", "web", "pr-42", "sandbox"),
		l.Sandbox("acme", "web", 42))
	assert.Equal(t, filepath.Join(l.PRDir("acme", "web", 42), DirSandbox),
		l.Sandbox("acme", "web", 42))
}
