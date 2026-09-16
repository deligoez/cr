package role

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The embedded releases are exactly the pinned list, and every file in them is
// a role cr would load under its own name. A directory added without the list,
// or the list extended without its directory, would make StandingOf read one
// set while the coverage test below checks another.
func TestTheEmbeddedRoleReleasesAreExactlyThePinnedOnes(t *testing.T) {
	entries, err := fs.ReadDir(shipped, shippedDir)
	require.NoError(t, err)
	embedded := make([]string, 0, len(entries))
	for _, entry := range entries {
		require.True(t, entry.IsDir(), "%s holds one directory per release, not %s", shippedDir, entry.Name())
		embedded = append(embedded, entry.Name())
	}
	assert.Equal(t, shippedReleases, embedded)

	for _, release := range shippedReleases {
		files, err := fs.ReadDir(shipped, path.Join(shippedDir, release))
		require.NoError(t, err)
		require.NotEmpty(t, files, "%s ships no role", release)
		for _, file := range files {
			body, err := shipped.ReadFile(path.Join(shippedDir, release, file.Name()))
			require.NoError(t, err)
			r, err := Parse(file.Name(), body)
			require.NoError(t, err, "%s/%s", release, file.Name())
			assert.Equal(t, strings.TrimSuffix(file.Name(), fileExt), r.ID)
			assert.NotEqual(t, Builtins()[r.ID], string(body),
				"%s's %s equals the role this build ships, so embedding it names the same answer twice",
				release, r.ID)
		}
	}
}

// Every release tag that carries a builtin role is covered: its files are
// either embedded under its own name, byte for byte, or equal to the roles this
// build ships. A later release that changes a shipped role without embedding
// the release before it fails here, because that release's files then equal
// neither — and a user holding them would be left out of §2.5.2's update
// without a word.
//
// The tags are read from the repository, which a shallow clone does not carry;
// there the pinned list above is what remains checked.
//
// Every tag is read and not only the ones `--merged HEAD` reaches, and that is
// measured rather than tidy: on 2026-09-16 the v0.2.2 release commit had been
// replayed onto main under a different SHA, so `git tag --merged HEAD` listed
// v0.1.0, v0.2.0 and v0.2.1 and silently omitted the tag a user had installed
// from. A release is a release whether or not its commit is an ancestor of the
// branch being tested.
func TestEveryReleaseTagCarryingABuiltinRoleIsCovered(t *testing.T) {
	tags := strings.Fields(repositoryGit(t, "tag", "--list", "v*"))
	if len(tags) == 0 {
		t.Skip("this clone carries no release tag, so only the pinned list is checked")
	}
	covered := 0
	for _, tag := range tags {
		listed := strings.Fields(repositoryGit(t, "ls-tree", "--full-tree", "--name-only", tag, "internal/role/builtin/"))
		for _, file := range listed {
			if !strings.HasSuffix(file, fileExt) {
				continue
			}
			id := strings.TrimSuffix(path.Base(file), fileExt)
			atTag := repositoryGit(t, "show", tag+":"+file)
			embedded, err := shipped.ReadFile(path.Join(shippedDir, tag, id+fileExt))
			if err == nil {
				assert.Equal(t, atTag, string(embedded), "%s's %s is embedded with other bytes", tag, id)
			} else {
				assert.Equal(t, Builtins()[id], atTag,
					"%s shipped a %s role that is neither embedded under %s/%s nor this build's",
					tag, id, shippedDir, tag)
			}
			covered++
		}
	}
	assert.Positive(t, covered, "no tag carried a builtin role")
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

// StandingOf answers three ways, and the current role wins over a release
// holding the same bytes: convention did not change between releases, and a
// file holding it is up to date rather than stale.
func TestStandingOfTellsACurrentStaleAndEditedRoleApart(t *testing.T) {
	previous, err := shipped.ReadFile(path.Join(shippedDir, "v0.2.2", "correctness.json"))
	require.NoError(t, err)
	edited := bytes.Replace(previous, []byte(`"title": "Correctness`), []byte(`"title": "Sharpness`), 1)
	require.NotEqual(t, previous, edited)

	cases := map[string]struct {
		id      string
		content []byte
		want    Standing
		stale   bool
	}{
		"the current correctness": {correctnessID, []byte(correctness), Standing{Current: true}, false},
		"v0.2.2's correctness":    {correctnessID, previous, Standing{Release: "v0.2.2"}, true},
		"an edited correctness":   {correctnessID, edited, Standing{}, false},
		"the current convention":  {conventionID, []byte(convention), Standing{Current: true}, false},
		"a role cr never shipped": {"security", []byte(correctness), Standing{}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := StandingOf(tc.id, tc.content)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.stale, got.Stale())
		})
	}
}

// §2.5.2's report: a corpus that resolved an ejected role file holding an
// earlier release's bytes carries one sentence naming that file, the release
// and `cr init`, and a corpus of current or edited files carries none.
//
// The built-in layer contributes nothing however stale the ejected file is,
// which is the half that keeps the report about a file somebody has to fix.
func TestAResolvedCorpusNamesTheReleaseThatShippedAnEjectedRole(t *testing.T) {
	previous, err := shipped.ReadFile(path.Join(shippedDir, "v0.2.2", "correctness.json"))
	require.NoError(t, err)

	cases := map[string]struct {
		content []byte
		want    func(file string) []string
	}{
		"an earlier release's bytes": {previous, func(file string) []string {
			return []string{file + " is the correctness role cr v0.2.2 shipped, unedited, and the " +
				"shipped role has since changed instructions; cr init updates the file to it, and " +
				"until then every prompt cr emits for this role carries the earlier release's framing"}
		}},
		"the current bytes": {[]byte(correctness), func(string) []string { return []string{} }},
		"an edited file": {
			bytes.Replace(previous, []byte(`"title": "Correctness`), []byte(`"title": "Sharpness`), 1),
			func(string) []string { return []string{} },
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "correctness.json")
			require.NoError(t, os.WriteFile(file, tc.content, 0o600))

			corpus, err := Resolve(filepath.Join(dir, "absent"), dir)
			require.NoError(t, err)
			require.Len(t, corpus, len(Builtins()), "the ejected file shadows one built-in and no more")

			assert.Equal(t, tc.want(file), StaleDisclosures(corpus))
		})
	}
}

// changedFields compares §2.5's flat table field by field, so a key on one side
// only is named, a changed scalar is named, and a list is named once however
// many of its entries moved.
func TestChangedRoleFieldsNamesEveryDifferingField(t *testing.T) {
	cases := map[string]struct {
		before, after string
		want          []string
	}{
		"nothing differs":        {`{"a": "x", "f": [1, 2]}`, `{"f":[1,2],"a":"x"}`, []string{}},
		"a list gained an entry": {`{"focus": ["x"]}`, `{"focus": ["x", "y"]}`, []string{"focus"}},
		"a key on one side only": {`{"a": 1}`, `{"a": 1, "classes": ["c"]}`, []string{"classes"}},
		"a key removed":          {`{"a": 1, "profiles": []}`, `{"a": 1}`, []string{"profiles"}},
		"several, sorted":        {`{"title": "a", "axis": "test"}`, `{"title": "b", "axis": "intent"}`, []string{"axis", "title"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, changedFields([]byte(tc.before), []byte(tc.after)))
		})
	}
	assert.Nil(t, changedFields([]byte(`{`), []byte(`{}`)), "a document that does not decode names nothing")
}
