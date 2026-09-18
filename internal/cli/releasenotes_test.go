package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// notesHeadline is the one shape §14 gives a release notes file's first line,
// and therefore the one shape a release title takes: `# cr v<version> —
// <headline>`, with the em dash the earlier releases used.
var notesHeadline = regexp.MustCompile(`^# cr v\d+\.\d+\.\d+ — \S`)

// Every release notes file opens with the headline a release title is taken
// from, and its version matches its file name.
//
// The title is not written by hand at release time; it is this line with `cr `
// removed. So a file whose first line says something else does not merely read
// oddly — it names the release on GitHub, and it is read once, after the tag
// that cannot be taken back.
//
// The naming was three different things before this guard: `cr v0.2.1` for the
// first three releases, `v0.2.2 — <headline>` for the next three, and the bare
// tag for v0.3.1 and v0.4.0, which took GoReleaser's default because nobody
// pasted a body.
func TestEveryReleaseNotesFileOpensWithItsTitle(t *testing.T) {
	notes, err := filepath.Glob(filepath.Join("..", "..", "spec", "*-release-notes.md"))
	require.NoError(t, err)
	require.NotEmpty(t, notes, "a guard over no notes file proves nothing")

	for _, path := range notes {
		t.Run(filepath.Base(path), func(t *testing.T) {
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			first, _, found := strings.Cut(string(body), "\n")
			require.True(t, found, "the file holds no line")

			assert.Regexp(t, notesHeadline, first,
				"§14: a release title is this line without `cr `")
			version := strings.TrimSuffix(filepath.Base(path), "-release-notes.md")
			assert.True(t, strings.HasPrefix(first, "# cr v"+version+" — "),
				"%s names a version its file name does not: %q", filepath.Base(path), first)
		})
	}
}

// Every release tag this repository carries has a notes file, so the workflow's
// refusal never fires on a release somebody already published.
//
// v0.3.2 is exempt and says so in its own first blockquote: it was written,
// never tagged, and its fixes shipped inside v0.4.0. A notes file with no tag
// is the harmless direction; a tag with no notes file is the one that publishes
// a commit list.
func TestEveryReleaseTagHasItsNotesFile(t *testing.T) {
	tags := strings.Fields(mustGit(t, filepath.Join("..", ".."), "tag", "--list", "v*"))
	if len(tags) == 0 {
		t.Skip("this clone carries no release tag")
	}
	for _, tag := range tags {
		path := filepath.Join("..", "..", "spec", strings.TrimPrefix(tag, "v")+"-release-notes.md")
		assert.FileExistsf(t, path, "%s was tagged and §14 publishes %s as its body", tag, path)
	}
}

// The release workflow publishes that file and takes the title from it, and
// GoReleaser is told to use both rather than its own changelog.
//
// Read out of the two files rather than trusted, for the reason
// release_test.go gives about the whole release path: it is configuration no
// compiler sees, and a step dropped from it is invisible until a tag is pushed.
func TestTheReleaseWorkflowPublishesTheNotesFile(t *testing.T) {
	var flow workflow
	require.NoError(t, yaml.Unmarshal(repoFile(t, ".github/workflows/release.yml"), &flow))
	job, named := flow.Jobs["release"]
	require.True(t, named, "the release workflow has no `release` job")

	var prepared, passed bool
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "RELEASE_NOTES=") && strings.Contains(step.Run, "RELEASE_NAME=") {
			prepared = true
			assert.Contains(t, step.Run, "-release-notes.md",
				"the notes file is the one in spec/, named after the tag")
			assert.Contains(t, step.Run, "::error::",
				"a missing notes file fails the release rather than falling back")
		}
		if args, given := step.With["args"].(string); given {
			passed = strings.Contains(args, "--release-notes=")
		}
	}
	assert.True(t, prepared, "no step reads the notes file out of the tag")
	assert.True(t, passed, "goreleaser is not given --release-notes, so it writes the commit list")

	var config struct {
		Release struct {
			NameTemplate string `yaml:"name_template"`
		} `yaml:"release"`
	}
	require.NoError(t, yaml.Unmarshal(repoFile(t, ".goreleaser.yml"), &config))
	assert.Equal(t, "{{ .Env.RELEASE_NAME }}", config.Release.NameTemplate,
		"the title comes from the notes file's headline, through the workflow")
}
