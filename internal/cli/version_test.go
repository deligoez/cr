package cli

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildInfo answers versionOf's read with a main module version, or with no
// build information at all when ok is false.
func buildInfo(mainVersion string, ok bool) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		if !ok {
			return nil, false
		}
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/deligoez/cr", Version: mainVersion}}, true
	}
}

// §14.6: an injected version wins; without one the module version the toolchain
// recorded is printed; and a build that knows neither prints the development
// marker, never an empty string.
func TestAnUnsetVersionRendersTheDevelopmentMarker(t *testing.T) {
	for name, tc := range map[string]struct {
		injected string
		read     func() (*debug.BuildInfo, bool)
		prints   string
	}{
		"a release build's injected tag":        {"v0.1.0", buildInfo("v0.0.9", true), "v0.1.0"},
		"a go install of a tag":                 {"", buildInfo("v0.1.0", true), "v0.1.0"},
		"a build the toolchain could not stamp": {"", buildInfo("(devel)", true), devVersion},
		"a module version left empty":           {"", buildInfo("", true), devVersion},
		"no build information at all":           {"", buildInfo("", false), devVersion},
	} {
		t.Run(name, func(t *testing.T) {
			printed := versionOf(tc.injected, tc.read)
			assert.Equal(t, tc.prints, printed)
			assert.NotEmpty(t, printed)
		})
	}
}

// §14.4 and §14.6 through a real `go install`: a copy of this module, committed
// and tagged v0.9.9 in a repository of its own, installed with no ldflags and no
// network, prints that tag from `cr --version`.
func TestAGoInstallOfATagPrintsTheTag(t *testing.T) {
	gotool, err := exec.LookPath("go")
	require.NoError(t, err)
	src := filepath.Join(t.TempDir(), "cr")
	copyModule(t, moduleRoot(t), src)
	commit := func(args ...string) {
		t.Helper()
		run := exec.Command("git", append([]string{"-C", src, "-c", "commit.gpgsign=false"}, args...)...)
		run.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=cr fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
			"GIT_COMMITTER_NAME=cr fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
		out, err := run.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	commit("init", "--quiet", "--initial-branch=main")
	commit("add", "-A")
	commit("commit", "--quiet", "-m", "the tagged release")
	commit("tag", "v0.9.9")

	bin := t.TempDir()
	install := exec.Command(gotool, "install", "./cmd/cr")
	install.Dir = src
	install.Env = append(os.Environ(), "GOBIN="+bin, "GOPROXY=off", "GOWORK=off", "GOFLAGS=")
	out, err := install.CombinedOutput()
	require.NoError(t, err, "go install failed: %s", out)

	printed, err := exec.Command(filepath.Join(bin, "cr"), "--version").Output()
	require.NoError(t, err)
	assert.Equal(t, "cr version v0.9.9\n", string(printed))
}

// copyModule copies the files a build of cr reads — go.mod, go.sum, and the
// cmd and internal trees — from root into dst.
func copyModule(t *testing.T, root, dst string) {
	t.Helper()
	for _, name := range []string{"go.mod", "go.sum", "cmd", "internal"} {
		from := filepath.Join(root, name)
		require.NoError(t, filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dst, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o750)
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			return os.WriteFile(target, body, 0o600)
		}))
	}
}
