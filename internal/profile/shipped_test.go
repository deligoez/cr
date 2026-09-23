package profile

import (
	"bytes"
	"cmp"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"slices"
	"strconv"
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
	// The directory lists in name order, where v0.10.0 sorts before v0.2.0,
	// so the two are compared as sets and the list's own order is held to
	// the version order StandingOf walks it backwards in.
	assert.ElementsMatch(t, shippedReleases, embedded)
	assert.True(t, slices.IsSortedFunc(shippedReleases, compareReleases),
		"shippedReleases is oldest first, by version rather than by name: %v", shippedReleases)

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
//
// Every tag is read and not only the ones `--merged HEAD` reaches, and that is
// measured rather than tidy: on 2026-09-16 the v0.2.2 release commit had been
// replayed onto main under a different SHA, so `git tag --merged HEAD` listed
// v0.1.0, v0.2.0 and v0.2.1 and silently omitted the tag a user had installed
// from — this guard was then covering three of the four releases in the
// repository and saying nothing about the fourth. A release is a release
// whether or not its commit is an ancestor of the branch being tested.
func TestEveryReleaseTagCarryingABuiltinProfileIsCovered(t *testing.T) {
	tags := strings.Fields(repositoryGit(t, "tag", "--list", "v*"))
	if len(tags) == 0 {
		t.Skip("this clone carries no release tag, so only the pinned list is checked")
	}
	covered := 0
	for _, tag := range tags {
		listed := strings.FieldsSeq(repositoryGit(t, "ls-tree", "--full-tree", "--name-only", tag, "internal/profile/builtin/"))
		for file := range listed {
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

// compareReleases orders two `v<major>.<minor>.<patch>` tags by version.
func compareReleases(a, b string) int {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := range min(len(pa), len(pb)) {
		na, _ := strconv.Atoi(pa[i])
		nb, _ := strconv.Atoi(pb[i])
		if c := cmp.Compare(na, nb); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(pa), len(pb))
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

// A parsed profile carries the sentence for a file an earlier release shipped,
// and nothing for the current profile or an edited one. The sentence names the
// file it was parsed as, because that is the file `cr init` updates.
func TestAParsedProfileNamesTheReleaseThatShippedItsBytes(t *testing.T) {
	previous, err := shipped.ReadFile("builtin/shipped/v0.2.1/laravel-pest.json")
	require.NoError(t, err)
	// The newest release whose bytes differ from this build's, so the
	// sentence names one changed field rather than two: `rules`, which is
	// the starter corpus the profile now carries.
	latest, err := shipped.ReadFile("builtin/shipped/v0.3.1/laravel-pest.json")
	require.NoError(t, err)
	edited := bytes.Replace(previous, []byte(`"lang": "php"`), []byte(`"lang": "hack"`), 1)
	const file = "/home/dev/.cr/profiles/laravel-pest.json"

	cases := map[string]struct {
		content []byte
		want    []string
	}{
		"v0.2.1's bytes": {previous, []string{file + " is the laravel-pest profile cr v0.2.1 shipped, unedited, " +
			"and the shipped profile has since changed rules, sandbox.copy; cr init updates the file to it, " +
			"and the next cr test or cr probe run then recreates a sandbox lacking a file it copies"}},
		"v0.3.1's bytes": {latest, []string{file + " is the laravel-pest profile cr v0.3.1 shipped, unedited, " +
			"and the shipped profile has since changed rules; cr init updates the file to it"}},
		"the current bytes": {[]byte(laravelPest), []string{}},
		"an edited file":    {edited, []string{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := Parse(file, tc.content)
			require.NoError(t, err)
			assert.Equal(t, tc.want, p.StaleDisclosures())
		})
	}
}

// changedFields descends into objects and compares everything else whole, so
// a key on one side only is named, a nested change is named by its dotted path,
// and a list is named once however many of its entries moved.
func TestChangedFieldsNamesEveryDifferingFieldByItsDottedPath(t *testing.T) {
	cases := map[string]struct {
		before, after string
		want          []string
	}{
		"nothing differs":         {`{"a": {"b": [1, 2]}}`, `{"a":{"b":[1,2]}}`, []string{}},
		"a list gained an entry":  {`{"s": {"copy": ["x"]}}`, `{"s": {"copy": ["x", "y"]}}`, []string{"s.copy"}},
		"a key on one side only":  {`{"a": 1}`, `{"a": 1, "b": {"c": 2}}`, []string{"b"}},
		"a key removed":           {`{"a": 1, "z": 2}`, `{"a": 1}`, []string{"z"}},
		"several, sorted":         {`{"t": {"cmd": ["a"]}, "a": 1}`, `{"t": {"cmd": ["b"]}, "a": 2}`, []string{"a", "t.cmd"}},
		"an object became a list": {`{"a": {"b": 1}}`, `{"a": [1]}`, []string{"a"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, changedFields([]byte(tc.before), []byte(tc.after)))
		})
	}
	assert.Nil(t, changedFields([]byte(`{`), []byte(`{}`)), "a document that does not decode names nothing")
}
