package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// theSecret is the line the fence exists for. It is asserted absent from every
// prompt `cr review` emits, so it must be a string nothing else could produce.
const theSecret = "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMIbK7MDENGbPxRfiCYcrFENCE"

// credentialHome is a pull request whose head changes one source file and adds
// two whose names say they carry a secret: `.env`, and `config/production.pem`.
// Neither may form a unit, because §4.6.1 puts a clustered file's changed lines
// verbatim into every role's prompt.
func credentialHome(t *testing.T, globs ...string) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	write("lib.go", "package lib\n\nfunc Load() {}\n")
	mustGit(t, dir, "add", "lib.go")
	mustGit(t, dir, "commit", "--quiet", "-m", "the base")
	mustGit(t, dir, "checkout", "--quiet", "-b", fixtureHeadBranch)
	write("lib.go", "package lib\n\nfunc Load() {\n\tparse()\n}\n")
	write(".env", theSecret+"\n")
	write("config/production.pem", "-----BEGIN PRIVATE KEY-----\n"+theSecret+"\n")
	mustGit(t, dir, "add", "lib.go", ".env", "config/production.pem")
	mustGit(t, dir, "commit", "--quiet", "-m", "the change under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
	base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))

	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	for id, body := range profile.Builtins() {
		require.NoError(t, layout.EnsureProfile(id, body))
	}
	require.NoError(t, layout.EnsureRepo(fixtureOwner, fixtureProject))
	// A variadic call with no argument gives a nil slice, which `ignoring`
	// would marshal as `null` and the config layer refuses; the fence's own
	// test needs the key genuinely unset, which is its default.
	if len(globs) > 0 {
		ignoring(t, layout, globs...)
	}
}

// briefCredential runs `cr brief` over credentialHome's pull request and
// returns the issue file beside the payload, because §3.1.4's flag is not
// inherited: `cr claims record` re-reads the issue and needs it again.
func briefCredential(t *testing.T) (units []unit.Unit, files unit.Files, issue string) {
	t.Helper()
	issue = filepath.Join(t.TempDir(), "issue.txt")
	require.NoError(t, os.WriteFile(issue, []byte(fixtureIssue+": load parses.\n"), 0o600))
	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue)
	require.NoError(t, err)
	var briefed struct {
		Units []unit.Unit `json:"units"`
		Files unit.Files  `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))
	return briefed.Units, briefed.Files, issue
}

// A credential-shaped file is listed and not clustered, so it forms no unit and
// its content reaches no prompt (§3.4.7).
func TestACredentialShapedFileIsListedAndItsContentReachesNoPrompt(t *testing.T) {
	credentialHome(t)

	units, briefed, issue := briefCredential(t)
	paths := make([]string, 0, len(units))
	for i := range units {
		paths = append(paths, units[i].Path)
	}
	assert.Equal(t, []string{"lib.go"}, paths, "§3.4.7: a credential-shaped file forms no unit")
	assert.Equal(t, unit.Files{Excluded: 0, Listed: []unit.Listed{
		{Path: ".env", Kind: unit.KindCredential},
		{Path: "config/production.pem", Kind: unit.KindCredential},
	}}, briefed, "both are listed by path, under the credential kind")

	claims := filepath.Join(t.TempDir(), "claims.ndjson")
	require.NoError(t, os.WriteFile(claims, []byte(`{"id":"`+fixtureIssue+`#c1",`+
		`"text":"load parses","source":"acceptance","span":"load parses."}`+"\n"), 0o600))
	_, err := runCLIPrinting(t, "claims", "record", fixturePR, claims,
		"--repo", fixtureSlug, "--intent-file", issue)
	require.NoError(t, err)

	printed, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug, "--axis", "intent")
	require.NoError(t, err)
	var emitted struct {
		Prompts []struct {
			Unit   string `json:"unit"`
			Prompt string `json:"prompt"`
		} `json:"prompts"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &emitted))
	require.NotEmpty(t, emitted.Prompts, "the intent axis emits at least one prompt for lib.go")
	for i := range emitted.Prompts {
		assert.NotContains(t, emitted.Prompts[i].Prompt, theSecret,
			"no prompt carries the content of a credential-shaped file")
	}

	shown := throughATerminal(t, "status", fixturePR, "--repo", fixtureSlug, "--no-color")
	assert.Contains(t, shown, "  .env (credential)")
	assert.Contains(t, shown, "  config/production.pem (credential)")
}

// The fence is tested before `ignore.globs`, so a repository that excludes the
// path anyway still sees it named rather than counted (§3.4.2, §3.4.7).
func TestTheCredentialFenceIsReadBeforeIgnoreGlobs(t *testing.T) {
	credentialHome(t, ".env", "config/**")

	units, briefed, _ := briefCredential(t)
	require.Len(t, units, 1)
	assert.Equal(t, 0, briefed.Excluded,
		"a credential-shaped file is never counted among the silent exclusions")
	assert.Equal(t, []unit.Listed{
		{Path: ".env", Kind: unit.KindCredential},
		{Path: "config/production.pem", Kind: unit.KindCredential},
	}, briefed.Listed, "the fence claims the file whatever the globs say")
}
