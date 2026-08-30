package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The release path is configuration, not code, so the compiler and the linter
// see none of it. §14.2 names an exact matrix — darwin and linux on amd64 and
// arm64 — and a GoReleaser build matrix is a cross product, so a fifth `goos`
// silently publishes surface the spec never asked for and a dropped one
// silently stops publishing surface it did. Both are invisible until a tag is
// pushed, which is the one moment a mistake cannot be taken back. These tests
// read the two files that produce a release and pin what §14 fixes.
//
// What they cannot do is run a release. The dry run
// (`goreleaser release --snapshot --clean --skip=publish`) is the other half,
// run by hand; it produced exactly the four archives this matrix predicts.

// goreleaserConfig is the slice of .goreleaser.yml that §14 constrains. Every
// other key — the archive name template, the changelog filters, the brews block
// that §14.3 owns — is deliberately absent, so this test fails only on the
// fields it is about.
type goreleaserConfig struct {
	Before struct {
		Hooks []string `yaml:"hooks"`
	} `yaml:"before"`
	Builds []struct {
		Main    string   `yaml:"main"`
		Binary  string   `yaml:"binary"`
		Goos    []string `yaml:"goos"`
		Goarch  []string `yaml:"goarch"`
		Ldflags []string `yaml:"ldflags"`
	} `yaml:"builds"`
	Archives []struct {
		Formats         []string `yaml:"formats"`
		FormatOverrides []struct {
			Goos string `yaml:"goos"`
		} `yaml:"format_overrides"`
	} `yaml:"archives"`
}

// workflow is the slice of a GitHub Actions file these tests read: what fires
// it, and what its steps run.
type workflow struct {
	On struct {
		Push struct {
			Tags []string `yaml:"tags"`
		} `yaml:"push"`
	} `yaml:"on"`
	Jobs map[string]struct {
		Steps []struct {
			Name string         `yaml:"name"`
			Uses string         `yaml:"uses"`
			Run  string         `yaml:"run"`
			With map[string]any `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// repoFile reads a file relative to the repository root, two levels above this
// package.
func repoFile(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(name)))
	require.NoError(t, err, "%s holds part of the release contract; if it moved, move this test with it", name)
	return raw
}

func readGoreleaser(t *testing.T) goreleaserConfig {
	t.Helper()
	var cfg goreleaserConfig
	require.NoError(t, yaml.Unmarshal(repoFile(t, ".goreleaser.yml"), &cfg))
	require.Len(t, cfg.Builds, 1, "§14.2 is one build matrix; a second one changes what a tag publishes")
	require.Len(t, cfg.Archives, 1, "a second archive config multiplies the artifact count past §14.2's four")
	return cfg
}

func readWorkflow(t *testing.T, name string) workflow {
	t.Helper()
	var wf workflow
	require.NoError(t, yaml.Unmarshal(repoFile(t, name), &wf))
	require.NotEmpty(t, wf.Jobs, "%s declares no job", name)
	return wf
}

// A `v*` tag is the whole trigger. Nothing else may publish, and this tag must.
func TestAVersionTagIsWhatTriggersARelease(t *testing.T) {
	wf := readWorkflow(t, ".github/workflows/release.yml")

	assert.Contains(t, wf.On.Push.Tags, "v*", "§14.2 releases on a v* tag")

	var args string
	for _, job := range wf.Jobs {
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "goreleaser/goreleaser-action@") {
				args = fmt.Sprint(step.With["args"])
			}
		}
	}
	require.NotEmpty(t, args, "§14.2 requires GoReleaser to produce the release; no goreleaser-action step runs")
	assert.True(t, strings.HasPrefix(args, "release"),
		"the action must run `release`; `build` produces no archives and publishes nothing, got %q", args)
}

// §14.2 fixes the matrix exactly. A cross product of two operating systems and
// two architectures is the four archives the third acceptance criterion counts,
// so this asserts the sets rather than a subset: adding windows back, or losing
// arm64, fails here rather than at the tag.
func TestTheBuildMatrixIsExactlyTheFourTheSpecNames(t *testing.T) {
	cfg := readGoreleaser(t)
	build := cfg.Builds[0]

	assert.ElementsMatch(t, []string{"darwin", "linux"}, build.Goos,
		"§14.2 publishes darwin and linux, and only those")
	assert.ElementsMatch(t, []string{"amd64", "arm64"}, build.Goarch,
		"§14.2 publishes amd64 and arm64, and only those")
	assert.Equal(t, 4, len(build.Goos)*len(build.Goarch),
		"the matrix is a cross product, so its size is the archive count")

	archive := cfg.Archives[0]
	assert.Equal(t, []string{"tar.gz"}, archive.Formats,
		"one format across the matrix keeps the archive count equal to the matrix size")
	for _, override := range archive.FormatOverrides {
		assert.Contains(t, build.Goos, override.Goos,
			"a format override for an unbuilt %q is dead configuration that reads as a promise to ship it", override.Goos)
	}
}

// §14.4's install path and the release binary must be the same package, and the
// version must come from the tag rather than from the source tree. The `-X`
// target is the variable `cr --version` prints; §14.6's own task owns the
// printing end, this asserts only that the release path fills it.
func TestTheReleaseBinaryIsTheOneGoInstallNames(t *testing.T) {
	cfg := readGoreleaser(t)
	build := cfg.Builds[0]

	assert.Equal(t, "./cmd/cr", build.Main)
	assert.Equal(t, "cr", build.Binary, "the archive must carry the binary under the name §14.3 installs")

	// `go install github.com/deligoez/cr/cmd/cr@<tag>` resolves to the module
	// path plus the package directory, which is exactly what GoReleaser builds.
	assert.Contains(t, string(repoFile(t, "go.mod")), "module github.com/deligoez/cr\n",
		"§14.4's install path is the module path plus %s; a renamed module breaks it", build.Main)

	assert.Contains(t, build.Ldflags, "-X github.com/deligoez/cr/internal/cli.version={{.Version}}",
		"§14.6 injects the tagged version into internal/cli.version at build time")
}
