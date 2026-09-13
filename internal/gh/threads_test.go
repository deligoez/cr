package gh

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// fixture reads one recorded GraphQL answer.
func fixture(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(body)
}

// recorder answers each successive gh invocation with the next recorded
// payload and keeps the arguments it was called with.
//
// No test in this package reaches the network. A test that called the real API
// would need a token, would rate-limit in CI, and would assert against
// whatever the pull request looks like today rather than against a payload it
// can reason about. Every fixture under testdata was captured once, by hand,
// from a public repository, and is replayed from then on.
type recorder struct {
	t       *testing.T
	answers []string
	calls   [][]string
}

// replay returns a recorder answering with the named fixtures in order. An
// invocation it has no answer for fails the test, so a test states the exact
// sequence of requests it expects rather than letting a stray one pass.
func replay(t *testing.T, answers ...string) *recorder {
	t.Helper()
	return &recorder{t: t, answers: answers}
}

func (r *recorder) run(args ...string) (string, error) {
	r.t.Helper()
	require.Lessf(r.t, len(r.calls), len(r.answers),
		"gh was run %d times against %d recorded answers: %v",
		len(r.calls)+1, len(r.answers), args)
	r.calls = append(r.calls, args)
	return fixture(r.t, r.answers[len(r.calls)-1]), nil
}

// field returns the value gh was given for one GraphQL variable.
func field(args []string, name string) (string, bool) {
	for _, arg := range args {
		if value, found := strings.CutPrefix(arg, name+"="); found {
			return value, true
		}
	}
	return "", false
}

// §3.5.1 ingests every existing review thread, resolved ones included, with
// author, body, anchor, resolution state, and replies. A resolved thread is
// the one a reviewer is most tempted to drop, and it is the one §3.5.4 needs:
// a concern somebody already raised and closed is exactly what must not be
// raised a second time.
func TestIngestionKeepsResolvedAndUnresolvedThreadsAlike(t *testing.T) {
	answers := replay(t, "threads.json")

	threads, err := WithRunner(answers.run).Threads("cli", "cli", 11451)
	require.NoError(t, err)
	require.Len(t, threads, 2)

	resolved := threads[0]
	assert.Equal(t, "PRRT_kwDODKw3uc5XATDu", resolved.ID)
	assert.True(t, resolved.Resolved)
	assert.False(t, resolved.Outdated)
	assert.Equal(t, Anchor{
		Path:              "pkg/cmd/release/shared/fetch.go",
		Side:              git.Right,
		StartLine:         168,
		Line:              168,
		OriginalStartLine: 168,
		OriginalLine:      168,
	}, resolved.Anchor)
	assert.Equal(t, "copilot-pull-request-reviewer", resolved.Comment.Author)
	assert.Equal(t, "Bot", resolved.Comment.AuthorTypename)
	assert.Equal(t, "PRRC_kwDODKw3uc6G7GwX", resolved.Comment.ID)
	assert.Equal(t, "2025-08-08T17:29:32Z", resolved.Comment.CreatedAt)
	assert.Contains(t, resolved.Comment.Body, "masks all JSON decoding errors")
	assert.Contains(t, resolved.Comment.URL, "#discussion_r2263641111")
	assert.Empty(t, resolved.Replies)

	open := threads[1]
	assert.Equal(t, "PRRT_kwDODKw3uc5XAsRq", open.ID)
	assert.False(t, open.Resolved)
	assert.Equal(t, "babakks", open.Comment.Author)
	assert.Equal(t, "User", open.Comment.AuthorTypename)
	require.Len(t, open.Replies, 1)
	assert.Equal(t, "PRRC_kwDODKw3uc6G8AGk", open.Replies[0].ID)
	assert.Equal(t, "ejahnGithub", open.Replies[0].Author)
	assert.Contains(t, open.Replies[0].Body, "partial semver match")

	require.Len(t, answers.calls, 1)
	number, sent := field(answers.calls[0], "number")
	assert.True(t, sent)
	assert.Equal(t, "11451", number)
}

// A pull request carrying more threads than one page holds must be ingested
// whole. A first page taken for the whole set drops threads silently, and a
// thread cr never saw is a thread §3.5.3 cannot attach and §3.5.4 cannot
// suppress a finding against.
func TestIngestionAsksForEveryPageOfThreads(t *testing.T) {
	answers := replay(t, "threads-first-page.json", "threads-second-page.json")

	threads, err := WithRunner(answers.run).Threads("cli", "cli", 11451)
	require.NoError(t, err)
	require.Len(t, threads, 2)
	assert.Equal(t, "PRRT_kwDODKw3uc5XATDu", threads[0].ID)
	assert.Equal(t, "PRRT_kwDODKw3uc5XAsRq", threads[1].ID)

	require.Len(t, answers.calls, 2)
	_, opened := field(answers.calls[0], "cursor")
	assert.False(t, opened, "the first page is asked for from the start, not from a cursor")
	cursor, resumed := field(answers.calls[1], "cursor")
	require.True(t, resumed)
	assert.Equal(t, "Y3Vyc29yOnYyOpK0MjAyNS0wOC0wOFQxODo1MTozOFrOVwLEag==", cursor)
}

// §3.5.1 requires a thread's replies, and §3.5.5 offers the author's among
// them as candidate context notes. A thread long enough to need a second page
// of comments is the one whose replies settled something, so the connection is
// followed rather than truncated at the page that arrived with the thread.
func TestIngestionFollowsRepliesPastTheFirstPage(t *testing.T) {
	answers := replay(t, "threads-held-replies.json", "replies-second-page.json")

	threads, err := WithRunner(answers.run).Threads("cli", "cli", 8950)
	require.NoError(t, err)
	require.Len(t, threads, 1)

	assert.Equal(t, "williammartin", threads[0].Comment.Author)
	require.Len(t, threads[0].Replies, 4)
	assert.Equal(t, "richterdavid", threads[0].Replies[2].Author)
	assert.Contains(t, threads[0].Replies[3].Body, "let's add `update` back")

	require.Len(t, answers.calls, 2)
	thread, asked := field(answers.calls[1], "thread")
	require.True(t, asked, "a further page of comments is asked for by thread id")
	assert.Equal(t, "PRRT_kwDODKw3uc47mh8c", thread)
	cursor, resumed := field(answers.calls[1], "cursor")
	require.True(t, resumed)
	assert.Equal(t, "Y3Vyc29yOnYyOpK0MjAyNC0wNC0xNVQxNTo0NDo1MFrOXVdaPQ==", cursor)
}

// An outdated thread hangs on code the head no longer carries, and GitHub
// answers null for every current line it has. It is still ingested — §3.5.1
// admits no exception — and it keeps the position it was written against, so
// §3.5.3 can tell a thread that still resolves from one that does not by
// looking at the anchor alone. No file has a line zero.
func TestAnOutdatedThreadKeepsWhereItWasWritten(t *testing.T) {
	answers := replay(t, "threads-outdated.json")

	threads, err := WithRunner(answers.run).Threads("cli", "cli", 8950)
	require.NoError(t, err)
	require.Len(t, threads, 1)

	assert.True(t, threads[0].Outdated)
	assert.False(t, threads[0].Resolved)
	assert.Equal(t, Anchor{
		Path:              "docs/install_linux.md",
		Side:              git.Right,
		StartLine:         0,
		Line:              0,
		OriginalStartLine: 17,
		OriginalLine:      17,
	}, threads[0].Anchor)
}

// A page that claims a successor and names no cursor would be asked for again
// under the same cursor for as long as the process lives. Ingestion stops and
// says so instead: a command that never returns is harder to diagnose than one
// that fails.
func TestIngestionRefusesAPageThatNamesNoCursor(t *testing.T) {
	const threadsPage = `{"data":{"repository":{"pullRequest":{"reviewThreads":{
		"pageInfo":{"hasNextPage":true,"endCursor":null},"nodes":[]}}}}}`
	const commentsPage = `{"data":{"repository":{"pullRequest":{"reviewThreads":{
		"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"id":"PRRT_1","comments":{"pageInfo":{"hasNextPage":true,"endCursor":null},
		"nodes":[]}}]}}}}}`

	_, err := WithRunner(func(...string) (string, error) { return threadsPage, nil }).
		Threads("acme", "web", 42)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "acme/web#42")
	assert.Contains(t, err.Error(), "names no cursor")

	_, err = WithRunner(func(...string) (string, error) { return commentsPage, nil }).
		Threads("acme", "web", 42)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "thread PRRT_1")
	assert.Contains(t, err.Error(), "names no cursor")
}

// A review thread is created by its opening comment, so one carrying none is a
// shape GitHub does not answer with. Reading it as though it did would index
// past the end of the slice, and an ingest that panics loses every other thread
// on the pull request along with the unreadable one.
func TestAThreadCarryingNoCommentIsStillIngested(t *testing.T) {
	const empty = `{"data":{"repository":{"pullRequest":{"reviewThreads":{
		"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"id":"PRRT_1","path":"web/app.go","line":9,"diffSide":"LEFT",
		"isResolved":true,"comments":{"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}}}`

	threads, err := WithRunner(func(...string) (string, error) { return empty, nil }).
		Threads("acme", "web", 42)

	require.NoError(t, err)
	require.Len(t, threads, 1)
	assert.Equal(t, "PRRT_1", threads[0].ID)
	assert.True(t, threads[0].Resolved)
	assert.Equal(t, git.Left, threads[0].Anchor.Side)
	assert.Equal(t, Comment{}, threads[0].Comment)
	assert.Empty(t, threads[0].Replies)
}

// §3.5.2 requires every ingested thread to carry an author type of `human` or
// `bot`, and §3.5.3 filters on it: only the human threads are attached to a
// unit. The tag is recorded next to the typename GitHub answered rather than
// in place of it, so the agent can see both the judgement and the fact it was
// made from.
func TestEveryIngestedThreadIsTaggedHumanOrBot(t *testing.T) {
	threads, err := WithRunner(replay(t, "threads.json").run).Threads("cli", "cli", 11451)
	require.NoError(t, err)
	require.Len(t, threads, 2)

	assert.Equal(t, AuthorBot, threads[0].AuthorType)
	assert.Equal(t, "Bot", threads[0].Comment.AuthorTypename)
	assert.Equal(t, AuthorHuman, threads[1].AuthorType)
	assert.Equal(t, "User", threads[1].Comment.AuthorTypename)
}

// Only GitHub's own `Bot` author type makes the tag `bot`. A login ending in
// `[bot]`, an imported mannequin, and an author the API answers null for are
// all `human`, because none of them is GitHub saying a bot wrote the comment.
//
// `dependabot[bot]` posting as a `User` is therefore tagged `human`, and that
// is deliberate. Reading the suffix would catch it and would be cr judging
// whose comment counts, which §2.1 leaves to the agent; it would also err in
// the expensive direction, because a human wrongly tagged `bot` loses the
// attachment §3.5.3 makes and the suppression §3.5.4 builds on it, and cr
// repeats a concern a colleague already raised.
func TestOnlyGitHubsOwnBotTypeIsTaggedBot(t *testing.T) {
	const authors = `{"data":{"repository":{"pullRequest":{"reviewThreads":{
		"pageInfo":{"hasNextPage":false},"nodes":[
		{"id":"PRRT_1","diffSide":"RIGHT","comments":{"pageInfo":{"hasNextPage":false},
			"nodes":[{"id":"c1","author":{"__typename":"User","login":"dependabot[bot]"}}]}},
		{"id":"PRRT_2","diffSide":"RIGHT","comments":{"pageInfo":{"hasNextPage":false},
			"nodes":[{"id":"c2","author":{"__typename":"Mannequin","login":"imported"}}]}},
		{"id":"PRRT_3","diffSide":"RIGHT","comments":{"pageInfo":{"hasNextPage":false},
			"nodes":[{"id":"c3","author":null}]}},
		{"id":"PRRT_4","diffSide":"RIGHT","comments":{"pageInfo":{"hasNextPage":false},
			"nodes":[{"id":"c4","author":{"__typename":"Bot","login":"copilot"}}]}}
		]}}}}}`

	threads, err := WithRunner(func(...string) (string, error) { return authors, nil }).
		Threads("acme", "web", 42)
	require.NoError(t, err)
	require.Len(t, threads, 4)

	assert.Equal(t, AuthorHuman, threads[0].AuthorType, "a `[bot]` login GitHub calls a User")
	assert.Equal(t, AuthorHuman, threads[1].AuthorType, "a mannequin stands in for a person")
	assert.Equal(t, AuthorHuman, threads[2].AuthorType, "an author the API does not name")
	assert.Equal(t, AuthorBot, threads[3].AuthorType, "GitHub itself reports a bot")
}

// §8.3.3's read-back pairs a comment only with a thread the review a round
// created opened, so each comment carries the review GitHub names for it, a
// reply included, and a comment GitHub names no review for carries none.
func TestEachCommentCarriesTheReviewItWasPostedIn(t *testing.T) {
	const reviewed = `{"data":{"repository":{"pullRequest":{"reviewThreads":{
		"pageInfo":{"hasNextPage":false},"nodes":[
		{"id":"PRRT_1","diffSide":"RIGHT","comments":{"pageInfo":{"hasNextPage":false},
			"nodes":[{"id":"c1","pullRequestReview":{"id":"PRR_round1"}},
				{"id":"c2","pullRequestReview":{"id":"PRR_round2"}}]}},
		{"id":"PRRT_2","diffSide":"RIGHT","comments":{"pageInfo":{"hasNextPage":false},
			"nodes":[{"id":"c3","pullRequestReview":null}]}}
		]}}}}}`

	threads, err := WithRunner(func(...string) (string, error) { return reviewed, nil }).
		Threads("acme", "web", 42)
	require.NoError(t, err)
	require.Len(t, threads, 2)

	assert.Equal(t, "PRR_round1", threads[0].Comment.Review)
	require.Len(t, threads[0].Replies, 1)
	assert.Equal(t, "PRR_round2", threads[0].Replies[0].Review)
	assert.Empty(t, threads[1].Comment.Review, "a review GitHub answers null for")
}

// §3.5.3 forbids cr to assign a class to an ingested thread. A class is §6.1's
// defect class, which drives dedup, triage statistics, and waivers; an
// ingested thread is somebody else's comment, and cr has no basis to say what
// defect it names.
//
// The prohibition is held by pinning the record's whole field surface rather
// than by asserting the absence of one name, because the way it would be
// broken is a field added years from now — and the type is walked as well as
// the wire, so a hand-written marshaller could not slip a key past either.
func TestNoIngestedThreadCarriesAClass(t *testing.T) {
	assert.ElementsMatch(t, threadFields, fieldPaths(reflect.TypeFor[Thread](), ""))

	wire, err := json.Marshal(Thread{})
	require.NoError(t, err)
	keys := map[string]any{}
	require.NoError(t, json.Unmarshal(wire, &keys))
	assert.NotContains(t, keys, "class")
}

// threadFields is everything an ingested thread records: §3.5.1's author,
// body, anchor, resolution state, and replies, plus §3.5.2's tag.
var threadFields = []string{
	"id",
	"anchor.path",
	"anchor.side",
	"anchor.start_line",
	"anchor.line",
	"anchor.original_start_line",
	"anchor.original_line",
	"resolved",
	"outdated",
	"comment.id",
	"comment.author",
	"comment.author_typename",
	"comment.body",
	"comment.created_at",
	"comment.url",
	"comment.review",
	"author_type",
	"replies",
}

// fieldPaths returns the dotted json names of every leaf field, descending
// into nested structs only. A slice is a leaf: it holds the same type again.
func fieldPaths(t reflect.Type, prefix string) []string {
	paths := make([]string, 0, t.NumField())
	for field := range t.Fields() {
		name := field.Tag.Get("json")
		if prefix != "" {
			name = prefix + "." + name
		}
		if field.Type.Kind() == reflect.Struct {
			paths = append(paths, fieldPaths(field.Type, name)...)
			continue
		}
		paths = append(paths, name)
	}
	return paths
}

// §2.3 puts ingested threads in threads.ndjson, and §2.3.3 does not list that
// file, so its records carry no head and round. They survive the round trip
// through the state package's own writer, and an empty reply list is written
// as [] rather than null per §12.3.
func TestThreadsAreStoredInThreadsNdjson(t *testing.T) {
	threads, err := WithRunner(replay(t, "threads.json").run).Threads("cli", "cli", 11451)
	require.NoError(t, err)

	l := state.New(t.TempDir())
	held, err := l.LockPR("cli", "cli", 11451)
	require.NoError(t, err)
	require.NoError(t, WriteThreads(held, threads))
	require.NoError(t, held.Unlock())

	stored, err := ReadThreads(l, "cli", "cli", 11451)
	require.NoError(t, err)
	assert.Equal(t, threads, stored)

	body, err := os.ReadFile(l.PRFile("cli", "cli", 11451, state.FileThreads))
	require.NoError(t, err)
	assert.Len(t, strings.Split(strings.TrimSuffix(string(body), "\n"), "\n"), 2)
	assert.Contains(t, string(body), `"replies":[]`)
}

// oneThread is a page holding a single thread with the diffSide given, spelled
// into the payload exactly as it stands so an unquotable value arrives here as
// an unreadable answer rather than as something Go tidied up on the way.
func oneThread(side string) string {
	return `{"data":{"repository":{"pullRequest":{"reviewThreads":{
		"pageInfo":{"hasNextPage":false},"nodes":[
		{"id":"PRRT_1","path":"web/app.go","line":9,"diffSide":"` + side + `",
		"comments":{"pageInfo":{"hasNextPage":false},
		"nodes":[{"id":"c1","author":{"__typename":"User","login":"babakks"}}]}}]}}}}}`
}

// GitHub's diffSide is the one field of an ingested thread that has to mean the
// same thing as §9.2's side, because §3.5.3 compares the two: a thread is
// attached to a unit whose hunks its anchor falls inside, and §3.4.4 partitions
// those hunks by side.
//
// A value that is neither RIGHT nor LEFT therefore matches no hunk of any unit,
// and the cost lands where cr can least afford it. The thread would be
// ingested, attached to nothing, and §3.5.4 would suppress nothing against it —
// so cr would raise a concern a colleague has already raised, on a pull request
// where the answer is sitting in a thread nobody joined it to. Nothing in the
// run would report it.
//
// So ingestion refuses, and refuses loudly. Dropping the thread leaves exactly
// the same silence and hides the fault as well, and §3.5.1 admits no exception
// in any case: every existing thread is ingested. A refusal costs one failed
// command and names what to look at.
func TestAThreadWhoseDiffSideNamesNeitherTreeIsRefused(t *testing.T) {
	// The empty value is the one a payload missing the field decodes to, and
	// the rest are the spellings a neighbouring API or a hand-written
	// fixture produces.
	for _, side := range []string{"", "right", "Right", "RIGHT ", "MIDDLE", "BOTH"} {
		threads, err := WithRunner(func(...string) (string, error) {
			return oneThread(side), nil
		}).Threads("acme", "web", 42)

		require.Errorf(t, err, "diffSide %q names no tree", side)
		assert.Nilf(t, threads, "a refused ingest hands back no threads, so no caller reads half a set: %q", side)
		assert.Contains(t, err.Error(), "PRRT_1", "the thread is named")
		assert.Contains(t, err.Error(), string(git.Right))
		assert.Contains(t, err.Error(), string(git.Left))
	}

	// The two values §9.2 does define are ingested exactly as before: the
	// check refuses an answer cr cannot place and decides nothing else.
	for _, side := range []git.Side{git.Right, git.Left} {
		threads, err := WithRunner(func(...string) (string, error) {
			return oneThread(string(side)), nil
		}).Threads("acme", "web", 42)

		require.NoError(t, err)
		require.Len(t, threads, 1)
		assert.Equal(t, Anchor{
			Path: "web/app.go", Side: side, StartLine: 9, Line: 9,
		}, threads[0].Anchor)
		assert.Equal(t, "babakks", threads[0].Comment.Author)
	}
}
