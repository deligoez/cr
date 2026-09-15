package profile

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The embedded releases are exactly the pinned list, and every file in them is
// a profile cr would load under its own name. A directory added without the
// list, or the list extended without its directory, would make StandingOf read
// one set while the coverage test below checks another.
func TestTheEmbeddedReleasesAreExactlyThePinnedOnes(t *testing.T) {
	entries, err := fs.ReadDir(shipped, "builtin/shipped")
	require.NoError(t, err)
	embedded := make([]string, 0, len(entries))
	for _, entry := range entries {
		require.True(t, entry.IsDir(), "builtin/shipped holds one directory per release, not %s", entry.Name())
		embedded = append(embedded, entry.Name())
	}
	assert.Equal(t, shippedReleases, embedded)

	for _, release := range shippedReleases {
		files, err := fs.ReadDir(shipped, path.Join("builtin/shipped", release))
		require.NoError(t, err)
		require.NotEmpty(t, files, "%s ships no profile", release)
		for _, file := range files {
			body, err := shipped.ReadFile(path.Join("builtin/shipped", release, file.Name()))
			require.NoError(t, err)
			p, err := Parse(file.Name(), body)
			require.NoError(t, err, "%s/%s", release, file.Name())
			assert.Equal(t, strings.TrimSuffix(file.Name(), fileExt), p.ID)
		}
	}
}

// Every release tag reachable from HEAD that carries a builtin profile is
// covered: its files are either embedded under its own name, byte for byte, or
// equal to the profiles this build ships. A later release that changes a
// shipped profile without embedding the release before it fails here, because
// that release's files then equal neither — and a user holding them would be
// left out of `cr init`'s update without a word.
//
// The tags are read from the repository, which a shallow clone does not carry;
// there the pinned list above is what remains checked.
func TestEveryReleaseTagCarryingABuiltinProfileIsCovered(t *testing.T) {
	tags := strings.Fields(repositoryGit(t, "tag", "--merged", "HEAD", "--list", "v*"))
	if len(tags) == 0 {
		t.Skip("this clone carries no release tag, so only the pinned list is checked")
	}
	covered := 0
	for _, tag := range tags {
		listed := strings.Fields(repositoryGit(t, "ls-tree", "--full-tree", "--name-only", tag, "internal/profile/builtin/"))
		for _, file := range listed {
			if !strings.HasSuffix(file, fileExt) {
				continue
			}
			id := strings.TrimSuffix(path.Base(file), fileExt)
			atTag := repositoryGit(t, "show", tag+":"+file)
			embedded, err := shipped.ReadFile(path.Join("builtin/shipped", tag, id+fileExt))
			if err == nil {
				assert.Equal(t, atTag, string(embedded), "%s's %s is embedded with other bytes", tag, id)
			} else {
				assert.Equal(t, Builtins()[id], atTag,
					"%s shipped a %s profile that is neither embedded under builtin/shipped/%s nor this build's", tag, id, tag)
			}
			covered++
		}
	}
	assert.Positive(t, covered, "no tag reachable from HEAD carried a builtin profile")
}

// repositoryGit runs one git read in the repository this package lives in, with
// no configuration of the machine's own, and returns its standard output.
func repositoryGit(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_OPTIONAL_LOCKS=0",
		"LC_ALL=C",
	}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoErrorf(t, cmd.Run(), "git %s: %s", strings.Join(args, " "), stderr.String())
	return stdout.String()
}

// StandingOf answers three ways, and the current profile wins over a release
// holding the same bytes: generic did not change between releases, and a file
// holding it is up to date rather than stale.
func TestStandingOfTellsCurrentStaleAndEditedApart(t *testing.T) {
	previous, err := shipped.ReadFile("builtin/shipped/v0.2.1/laravel-pest.json")
	require.NoError(t, err)
	edited := bytes.Replace(previous, []byte(`"lang": "php"`), []byte(`"lang": "hack"`), 1)
	require.NotEqual(t, previous, edited)

	cases := map[string]struct {
		id      string
		content []byte
		want    Standing
		stale   bool
	}{
		"the current laravel-pest":   {laravelPestID, []byte(laravelPest), Standing{Current: true}, false},
		"v0.2.1's laravel-pest":      {laravelPestID, previous, Standing{Release: "v0.2.1"}, true},
		"an edited laravel-pest":     {laravelPestID, edited, Standing{}, false},
		"the current generic":        {genericID, []byte(generic), Standing{Current: true, Release: "v0.2.1"}, false},
		"a profile cr never shipped": {"shop", []byte(laravelPest), Standing{}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := StandingOf(tc.id, tc.content)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.stale, got.Stale())
		})
	}
}
