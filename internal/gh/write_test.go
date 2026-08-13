package gh

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveMutation is a GraphQL write in the shape cr would plausibly grow one:
// §9's re-review half resolves a thread, and that is a mutation sent by gh as
// the same POST every ingestion query already travels as.
const resolveMutation = `mutation($thread:ID!){resolveReviewThread(input:{threadId:$thread}){thread{id}}}`

// No write reaches gh through the read door, whatever it is wearing. §2.1.2
// allows cr no network write except §8's calls, and those only behind §8.5's
// gate, so the door a read goes through must refuse every one of these before
// gh is started at all — which is what the marker file proves. A refusal that
// arrived after the process ran would be a report, not a boundary.
//
// The cases are the four disguises a write comes in. A verb named outright, in
// each of the spellings gh accepts. A body with no verb, which gh turns into a
// POST for the caller. A subcommand that is a write by name rather than by
// flag. And a GraphQL mutation, whose verb is POST exactly like the ingestion
// query beside it, so the verb decides nothing and the operation keyword must.
func TestNoWriteReachesGhThroughTheReadDoor(t *testing.T) {
	for name, args := range map[string][]string{
		"a method named outright":              {"api", "repos/cli/cli/pulls/11451/reviews", "--method", "POST"},
		"a method attached to its flag":        {"api", "repos/cli/cli/issues/11451/comments", "--method=POST"},
		"a method against its shorthand":       {"api", "repos/cli/cli/pulls/11451", "-XPATCH"},
		"a method after its shorthand":         {"api", "repos/cli/cli/pulls/11451", "-X", "DELETE"},
		"a body read from a file":              {"api", "repos/cli/cli/pulls/11451/reviews", "--input", "payload.json"},
		"a body read from standard input":      {"api", "repos/cli/cli/pulls/11451/reviews", "--input", "-"},
		"a field with no method, which posts":  {"api", "repos/cli/cli/issues/11451/comments", "-f", "body=hello"},
		"a raw field with no method":           {"api", "repos/cli/cli/issues/11451/comments", "-F", "body=hello"},
		"a subcommand that writes by name":     {"pr", "comment", "11451", "--body", "hello"},
		"a subcommand that reviews by name":    {"pr", "review", "11451", "--comment", "--body", "hello"},
		"a GraphQL mutation":                   {"api", "graphql", "-f", "query=" + resolveMutation},
		"a mutation behind a leading query":    {"api", "graphql", "-f", "query=query Q{viewer{login}} " + resolveMutation},
		"a mutation behind a leading comment":  {"api", "graphql", "-f", "query=# query Q{viewer{login}}\n" + resolveMutation},
		"a GraphQL call carrying no document":  {"api", "graphql", "-f", "owner=cli"},
		"a GraphQL document from a file":       {"api", "graphql", "--input", "payload.json"},
		"a GraphQL document that is empty":     {"api", "graphql", "-f", "query="},
		"a GraphQL document opening elsewhere": {"api", "graphql", "-f", "query=fragment F on X{id} query Q{viewer{login}}"},
		"nothing at all":                       {},
	} {
		t.Run(name, func(t *testing.T) {
			ran := filepath.Join(t.TempDir(), "gh-was-started")
			stubGh(t, "touch "+ran+"\necho '{}'")

			_, err := Run(args...)

			var refused *WriteRefusedError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, args, refused.Args, "the refusal must name the invocation it refused")
			assert.Contains(t, refused.Error(), "§2.1.2")
			assert.NoFileExists(t, ran, "gh was started before the boundary refused")
		})
	}
}

// The reads cr makes still reach gh. A boundary closed by default is only
// worth having if it is narrower than the reads it stands in front of, and
// this is where that is measured: §3.5.1's ingestion is `gh api graphql`, sent
// as the same POST every mutation above travels as, and a rule that read verbs
// would have to refuse it or let the mutations through with it.
//
// The rest are the read shapes the boundary must not strangle as cr grows into
// §8.4.4's reconcile and the REST reads beside it: a plain GET, a GET that
// names its verb so its fields become query parameters rather than a body, an
// anonymous query, a document behind a comment, and an endpoint sitting after
// flags that take a value of their own.
func TestTheReadsCrMakesStillReachGh(t *testing.T) {
	for name, args := range map[string][]string{
		"the ingestion query of §3.5.1": {
			"api", "graphql",
			"-f", "query=" + threadsQuery,
			"-f", "owner=cli", "-f", "repo=cli",
			"-F", "number=11451", "-F", "threads=100", "-F", "comments=100",
		},
		"the replies query beside it":  {"api", "graphql", "-f", "query=" + repliesQuery, "-f", "thread=T_1"},
		"an anonymous query":           {"api", "graphql", "-f", "query={viewer{login}}"},
		"a query behind a comment":     {"api", "graphql", "-f", "query=# the threads of §3.5.1\nquery{viewer{login}}"},
		"a plain REST read":            {"api", "repos/cli/cli/pulls/11451"},
		"a REST read naming its verb":  {"api", "search/issues", "--method", "GET", "-f", "q=repo:cli/cli"},
		"a listing for §8.4.4":         {"api", "repos/cli/cli/pulls/11451/reviews", "--paginate"},
		"an endpoint after its flags":  {"api", "-q", ".head.sha", "-H", "Accept: application/json", "repos/cli/cli/pulls/11451"},
		"a read that names its accept": {"api", "--header=Accept: application/vnd.github+json", "repos/cli/cli/pulls/11451"},
	} {
		t.Run(name, func(t *testing.T) {
			ran := filepath.Join(t.TempDir(), "gh-was-started")
			stubGh(t, "touch "+ran+"\necho '{}'")

			out, err := Run(args...)

			require.NoError(t, err)
			assert.Equal(t, "{}\n", out)
			assert.FileExists(t, ran, "the boundary refused a read cr makes")
		})
	}
}

// The write door opens for a confirmed token and for nothing else.
//
// The refusals are the three ways a caller can arrive at it. A Confirmation
// composed rather than minted is the zero value — the only thing a package
// outside this one can build, since granted is unexported — and it writes
// nothing. A token minted from a --confirm that was not given is the same. And
// a confirmed token is still not a licence for any call: §8.3.1 requires every
// comment of a round to arrive as one review and §8.4.1 requires that review to
// be created atomically, so `gh pr comment` and `gh pr review` are refused
// after the token, not before it.
//
// The last case is the one §8 sanctions, and it must actually run: a door that
// never opens would pass every test above while making cr post nothing at all.
func TestOnlyAConfirmedTokenOpensTheWriteDoor(t *testing.T) {
	review := []string{"api", "repos/cli/cli/pulls/11451/reviews", "--method", "POST", "--input", "-"}

	for name, tc := range map[string]struct {
		token  Confirmation
		args   []string
		reason string
	}{
		"a token composed instead of minted": {Confirmation{}, review, "no confirmation was presented"},
		"a token minted without --confirm":   {Confirm(false), review, "no confirmation was presented"},
		"a comment posted by subcommand":     {Confirm(true), []string{"pr", "comment", "11451", "--body", "hi"}, "§8's only write is an API call"},
		"a review posted by subcommand":      {Confirm(true), []string{"pr", "review", "11451", "--comment"}, "§8's only write is an API call"},
		"no invocation at all":               {Confirm(true), nil, "§8's only write is an API call"},
	} {
		t.Run(name, func(t *testing.T) {
			ran := filepath.Join(t.TempDir(), "gh-was-started")
			stubGh(t, "touch "+ran+"\necho '{}'")

			_, err := tc.token.Write(tc.args...)

			var refused *WriteRefusedError
			require.ErrorAs(t, err, &refused)
			assert.Contains(t, refused.Error(), tc.reason)
			assert.NoFileExists(t, ran, "gh was started before the boundary refused")
		})
	}

	t.Run("the review creation of §8.3", func(t *testing.T) {
		ran := filepath.Join(t.TempDir(), "gh-was-started")
		stubGh(t, "touch "+ran+"\necho '{}'")

		out, err := Confirm(true).Write(review...)

		require.NoError(t, err)
		assert.Equal(t, "{}\n", out)
		assert.FileExists(t, ran, "the write §8 sanctions never reached gh")
	})
}
