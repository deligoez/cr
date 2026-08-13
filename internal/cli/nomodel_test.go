package cli

import (
	"fmt"
	"os"
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
