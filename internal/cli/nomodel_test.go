package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Invariant 1, which §2.1 states as normative text: cr is deterministic, and it
// never calls a language model. §1.3.4 puts the same sentence the other way
// round by declaring any call to a language model from within cr out of scope.
//
// The invariant is load-bearing rather than stylistic. §2.1.1 requires every
// command to be reproducible given the same state directory, the same head SHA,
// and the same inputs, and a model is the one input that answers differently on
// the second call. Everything downstream rests on that: a finding cr asserts is
// worth something to the author only because cr established it mechanically, so
// a model inside cr would let an unprovable claim reach a colleague wearing
// cr's own register.
//
// # Scope
//
// The guard fences what can execute, not what can be read. A model reaches cr
// in exactly three ways — an SDK sits in the module graph, something cr
// compiles imports one, or cr's own code addresses a model endpoint by hand —
// and those are the three tests below.
//
// Prose is outside the fence, and not as a concession. A grep for provider
// names across this repository fails on the documents that state the invariant:
// CLAUDE.md names Claude and Anthropic throughout in order to forbid their
// attribution, §1.3.4 and §2.1 say "language model" in the sentences that ban
// one, and spec/.tp-review/ holds thirteen rounds of findings arguing about
// models. A guard those files trip is a guard someone deletes, and it would
// have been measuring authorship of English rather than the behaviour of the
// binary. So the fence follows what runs: the Go source, the assets it embeds,
// and the module graph behind both.
//
// The user's own configuration is outside the fence from the other side. §3.1
// reads the tracker through a command the user supplies and §2.4 takes the test
// and setup commands from a profile, so a user can point either at anything on
// their machine; cr cannot police that, and a guard pretending to would claim
// more than it checks. What cr *ships* is inside: the built-in profiles of
// §2.4.5 are embedded in the binary and name commands cr runs itself, so they
// are scanned exactly like source.

// modelSDKPaths are the fragments that identify a language-model SDK by its
// module or import path. A provider's Go client is published under a path that
// says whose it is — github.com/anthropics/anthropic-sdk-go,
// github.com/sashabaranov/go-openai, google.golang.org/genai,
// github.com/tmc/langchaingo, github.com/aws/aws-sdk-go-v2/service/bedrockruntime
// — so a case-insensitive substring over the path catches the SDK without
// pinning its version or its exact spelling.
//
// A vendor list goes stale, but it goes stale in the safe direction: it names
// the clients that exist today and grows when one is added, and a module with
// three direct dependencies has no legitimate use for any of them. Nothing here
// is an English word that could occur in a path by accident, which is what
// keeps the substring match honest.
var modelSDKPaths = []string{
	"anthropic",
	"openai",
	"cohere",
	"mistral",
	"huggingface",
	"replicate-go",
	"langchain",
	"llamaindex",
	"ollama",
	"genai",
	"generativelanguage",
	"aiplatform",
	"vertexai",
	"bedrockruntime",
	"bedrock-runtime",
}

// selfPath is this file's own path on disk, so the source scan can exclude the
// guard from the surface it guards. Every provider name and model identifier
// this file needs as data would otherwise be read as the violation it exists to
// find. runtime.Caller answers instead of a hard-coded name, so renaming or
// moving the file cannot silently take the exclusion with it.
func selfPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "the compiler kept no path for this file, so the guard cannot exclude itself")
	return file
}

// moduleRoot is the directory holding go.mod, found by walking up from this
// file rather than from the working directory, which is the package directory
// under `go test` and something else entirely under a test binary run by hand.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir := filepath.Dir(selfPath(t))
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "no go.mod above %s", selfPath(t))
		dir = parent
	}
}

// A model SDK anywhere in the module graph fails, whether or not anything
// imports it. The two faults are not equally grave — a dependency nobody
// imports cannot call anything — but they are treated alike here, because the
// gap between them is one import statement, and an invariant that tolerates the
// dormant form is an invariant whose enforcement point sits after the decision
// has already been made. v0.1 carries neither.
//
// go.mod and go.sum are read as text rather than through `go list -m all`,
// which needs the module graph and therefore, on a cold cache, the network. Go
// 1.17 module pruning puts every module the build needs in go.mod's require
// blocks, and go.sum carries the graph beyond them, including a transitive
// module nobody chose.
func TestNoModelSDKSitsInTheModuleGraph(t *testing.T) {
	root := moduleRoot(t)

	var found []string
	for _, name := range []string{"go.mod", "go.sum"} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err, "%s is the module graph; if it moved, move this guard with it", name)

		for number, line := range strings.Split(string(raw), "\n") {
			for _, sdk := range modelSDKPaths {
				if strings.Contains(strings.ToLower(line), sdk) {
					found = append(found, fmt.Sprintf("%s:%d names %q: %s", name, number+1, sdk, strings.TrimSpace(line)))
				}
			}
		}
	}

	assert.Empty(t, found, "invariant 1: cr never calls a language model, so no model SDK may sit in its module graph")
}

// Nothing cr compiles imports a model SDK. The module graph above answers a
// different question from this one: the graph says what could be reached, and
// this says what the build actually reaches. Both are faults, because the
// distance between them is a single import line, but they fail with different
// messages because the fixes differ — one is a `go mod tidy` away, the other is
// a design that has already gone wrong.
//
// `go list -deps` answers rather than the import blocks of cr's own files,
// because those cannot see through a dependency: cobra or flock could grow a
// model client in a release, and the AST of this repository would still look
// clean while the binary reached one. `-test` widens it to the test binaries,
// so a model called only from a test — during development, on the author's
// machine, shaping what cr does with what it learned — is caught too.
func TestNothingCrBuildsImportsAModelSDK(t *testing.T) {
	root := moduleRoot(t)

	// A guard that quietly skips when its tool is missing guards nothing, so
	// an absent toolchain is a failure and not a skip.
	gotool, err := exec.LookPath("go")
	require.NoError(t, err, "the go toolchain is how this guard reads the compiled closure")

	var stdout, stderr bytes.Buffer
	list := exec.Command(gotool, "list", "-deps", "-test", "./...")
	list.Dir = root
	list.Stdout = &stdout
	list.Stderr = &stderr
	require.NoError(t, list.Run(), "go list failed: %s", strings.TrimSpace(stderr.String()))

	var found []string
	for pkg := range strings.FieldsSeq(stdout.String()) {
		for _, sdk := range modelSDKPaths {
			if strings.Contains(strings.ToLower(pkg), sdk) {
				found = append(found, fmt.Sprintf("%s names %q", pkg, sdk))
			}
		}
	}

	assert.Empty(t, found, "invariant 1: cr never calls a language model, so nothing it compiles may import one")
}

// modelSourceMarkers are the strings that would appear in cr's own source if a
// model were reached without an SDK — which is the hole an import check leaves,
// because net/http and a URL are all a provider's REST API needs. They are the
// three things such a call cannot do without: the host it is addressed to, the
// path on it, and the identifier of the model being asked. A credential name is
// listed with them because it is the same call one step earlier.
//
// Nothing here is an English word or a plausible identifier in a review tool.
// That is the whole selection rule: a substring match over source is only
// honest while every needle is a token that has no innocent reading, so
// "replicate" is absent although the SDK path list carries it, and no bare
// vendor name appears at all. A vendor name in a comment calls nothing; a host,
// a completion path, or a model family in a string literal is the call itself.
var modelSourceMarkers = []string{
	// The host a model answers on.
	"api.anthropic.com",
	"api.openai.com",
	"generativelanguage.googleapis.com",
	"api.cohere.com",
	"api.mistral.ai",
	"api.deepseek.com",
	"api.groq.com",
	"api.together.xyz",
	"openrouter.ai",
	"api.x.ai",
	"bedrock-runtime.",
	":11434",
	// The path on it.
	"/v1/chat/completions",
	"/v1/messages",
	"/v1/complete",
	"/v1/embeddings",
	"/v1beta/models/",
	// The credential it is made with.
	"anthropic_api_key",
	"openai_api_key",
	"gemini_api_key",
	// The model being asked.
	"claude-",
	"gpt-3.5",
	"gpt-4",
	"gpt-5",
	"o1-preview",
	"o3-mini",
	"gemini-",
	"llama-",
	"mistral-large",
	"deepseek-",
	"text-davinci",
	"text-embedding-",
}

// embeddedAssets returns the files gofile embeds into the binary, resolved from
// its //go:embed directives. They are read as part of the source surface
// because that is what they are: §2.4.5's built-in profiles ship inside cr and
// name the commands cr runs itself, so a model reached through one of those
// would be cr calling a model, with the argv merely one level away from the Go.
//
// A user's own profile and §3.1's tracker command are the deliberate other side
// of that line. Both are argv the user supplies at runtime, cr cannot see them
// from here, and policing where a user points their own tools is not what
// invariant 1 says.
func embeddedAssets(t *testing.T, gofile string) []string {
	t.Helper()
	raw, err := os.ReadFile(gofile)
	require.NoError(t, err)

	var assets []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		pattern, ok := strings.CutPrefix(strings.TrimSpace(line), "//go:embed ")
		if !ok {
			continue
		}
		for glob := range strings.FieldsSeq(pattern) {
			// `all:` and `-` only change which files inside a
			// directory are taken; the directory is the same one.
			glob = strings.TrimPrefix(strings.Trim(glob, `"`), "all:")
			matches, err := filepath.Glob(filepath.Join(filepath.Dir(gofile), glob))
			require.NoError(t, err, "%s embeds %q, which is not a valid pattern", gofile, glob)
			for _, match := range matches {
				require.NoError(t, filepath.WalkDir(match, func(path string, d fs.DirEntry, err error) error {
					if err == nil && !d.IsDir() {
						assets = append(assets, path)
					}
					return err
				}))
			}
		}
	}
	return assets
}

// No model endpoint, credential, or model identifier appears in what cr ships.
// The two tests above fence the SDKs; this one fences the way around them,
// because a provider's API is REST and cr already has net/http through its
// dependencies. An import check would call a hand-written POST to
// api.anthropic.com clean.
//
// The surface is the Go source and the assets it embeds — what executes, and
// what ships. This file excludes itself, since every marker above would
// otherwise be read as the violation it exists to find. The prose of the
// repository is outside, for the reason given at the top of this file: the
// documents that state the invariant are the ones a repository-wide grep would
// convict.
func TestNoModelEndpointOrIdentifierReachesTheBinary(t *testing.T) {
	root := moduleRoot(t)
	self := selfPath(t)

	scanned := 0
	var found []string
	scan := func(path string) {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		scanned++

		lower := strings.ToLower(string(raw))
		for _, marker := range modelSourceMarkers {
			if strings.Contains(lower, marker) {
				rel, err := filepath.Rel(root, path)
				require.NoError(t, err)
				found = append(found, fmt.Sprintf("%s names %q", rel, marker))
			}
		}
	}

	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || path == self {
			return nil
		}
		scan(path)
		for _, asset := range embeddedAssets(t, path) {
			scan(asset)
		}
		return nil
	}))

	// A walk that found nothing would pass, and would mean nothing. The floor
	// is the file count of the packages that exist, so it fails long before a
	// misresolved root can quietly empty the surface.
	require.Greater(t, scanned, 20, "only %d files were scanned, so this guard proved nothing", scanned)
	assert.Empty(t, found, "invariant 1: cr never calls a language model, by SDK or by hand")
}
