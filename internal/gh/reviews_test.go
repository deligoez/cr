package gh

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reviewsPage is one page of the reviews connection in GitHub's own shape.
func reviewsPage(hasNext bool, cursor string, nodes ...string) string {
	end := "null"
	if cursor != "" {
		end = `"` + cursor + `"`
	}
	next := "false"
	if hasNext {
		next = "true"
	}
	return `{"data":{"repository":{"pullRequest":{"reviews":{"pageInfo":{"hasNextPage":` + next +
		`,"endCursor":` + end + `},"nodes":[` + strings.Join(nodes, ",") + `]}}}}}`
}

// reviewNode is one review node as the query selects it.
func reviewNode(id, body string) string {
	return `{"id":"` + id + `","url":"https://github.com/acme/web/pull/42#pullrequestreview-` + id +
		`","body":"` + body + `","commit":{"oid":"0a1b2c3"}}`
}

// reviewsServer answers only the argv `gh api graphql` accepts for the
// reviews query: every variable a separate -f or -F field beside `query`, the
// page size GitHub's maximum, and the cursor given as its own field. Any other
// argv fails the test, so a request the real service would refuse cannot pass
// here. pages maps a cursor, empty for the first page, to its answer, and
// each is answered once.
func reviewsServer(
	t *testing.T, pages map[string]string,
) (run func(...string) (string, error), asked *[][]string) {
	t.Helper()
	calls := make([][]string, 0)
	return func(args ...string) (string, error) {
		t.Helper()
		calls = append(calls, args)
		require.GreaterOrEqual(t, len(args), 12, "argv: %v", args)
		require.Equal(t, []string{
			"api", "graphql",
			"-f", "query=" + reviewsQuery,
			"-f", "owner=acme", "-f", "repo=web",
			"-F", "number=42", "-F", "reviews=100",
		}, args[:12])
		cursor := ""
		switch rest := args[12:]; len(rest) {
		case 0:
		case 2:
			require.Equal(t, "-f", rest[0], "the cursor is a string field: argv %v", args)
			value, named := strings.CutPrefix(rest[1], "cursor=")
			require.True(t, named, "argv: %v", args)
			require.NotEmpty(t, value, "an empty cursor is no cursor: argv %v", args)
			cursor = value
		default:
			require.Failf(t, "unexpected argv", "%v", args)
		}
		page, known := pages[cursor]
		require.Truef(t, known, "no page for cursor %q", cursor)
		// A page asked for twice is a walk that would never end.
		delete(pages, cursor)
		return page, nil
	}, &calls
}

// §8.4.4 reads every review on the pull request, so a pull request whose
// reviews need a second page is read to its end: the second page is asked for
// under the first page's cursor, and the reviews come back oldest first.
func TestReviewsReadsEveryPage(t *testing.T) {
	run, calls := reviewsServer(t, map[string]string{
		"":             reviewsPage(true, "Y3Vyc29yOjE=", reviewNode("PRR_1", "first")),
		"Y3Vyc29yOjE=": reviewsPage(false, "", reviewNode("PRR_2", "second")),
	})

	reviews, err := WithRunner(run).Reviews("acme", "web", 42)

	require.NoError(t, err)
	require.Len(t, reviews, 2)
	assert.Equal(t, "PRR_1", reviews[0].ID)
	assert.Equal(t, "first", reviews[0].Body)
	assert.Equal(t, "0a1b2c3", reviews[0].Commit.OID)
	assert.Equal(t, "PRR_2", reviews[1].ID)
	assert.Len(t, *calls, 2, "one request per page, and none past the last")
}

// A page that claims a successor and names no cursor would be asked for again
// under no cursor for as long as the process lives, so the walk stops and says
// which pull request's reviews it was reading, returning no partial list.
func TestReviewsRefusesAPageThatNamesNoCursor(t *testing.T) {
	run, calls := reviewsServer(t, map[string]string{
		"": reviewsPage(true, "", reviewNode("PRR_1", "first")),
	})

	reviews, err := WithRunner(run).Reviews("acme", "web", 42)

	require.Error(t, err)
	assert.Equal(t, "acme/web#42: a page of reviews claims a next page and names no cursor", err.Error())
	assert.Nil(t, reviews)
	assert.Len(t, *calls, 1, "the page is not asked for again")
}
