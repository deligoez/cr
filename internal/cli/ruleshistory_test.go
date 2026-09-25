package cli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// aHistoryComment is one comment of the REST listing, in GitHub's own field
// names as measured on tarfin-labs/backend.
type aHistoryComment struct {
	id      int64
	pr      int
	login   string
	kind    string
	created string
	body    string
	path    string
	replyTo int64
}

// rest renders the comment the way `repos/<o>/<r>/pulls/comments` answers it.
func (c *aHistoryComment) rest() map[string]any {
	kind := c.kind
	if kind == "" {
		kind = "User"
	}
	path := c.path
	if path == "" {
		path = "app/Models/User.php"
	}
	node := map[string]any{
		"id": c.id, "html_url": fmt.Sprintf("https://github.com/%s/pull/%d#discussion_r%d", harvestSlug, c.pr, c.id),
		"pull_request_url": fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d", harvestSlug, c.pr),
		"user":             map[string]any{"login": c.login, "type": kind},
		"body":             c.body, "path": path, "line": 12, "side": "RIGHT",
		"created_at": c.created, "updated_at": "2026-09-25T00:00:00Z",
	}
	if c.replyTo != 0 {
		node["in_reply_to_id"] = c.replyTo
	}
	return node
}

// historyGH installs a gh that answers the two reads §2.6.3.5 and §2.6.3.6
// make, in the argv the real gh accepts, and refuses every other invocation.
// pages are the listing's pages newest first; authors are the pull requests'
// openers. It returns the invocations it was given.
func historyGH(t *testing.T, authors map[int]string, pages ...[]aHistoryComment) *[]string {
	t.Helper()
	calls := &[]string{}
	listing := "repos/" + harvestSlug + "/pulls/comments?sort=created&direction=desc&per_page=100&page="
	restore := ghClient
	ghClient = func() gh.Client {
		return gh.WithRunner(func(args ...string) (string, error) {
			*calls = append(*calls, strings.Join(args, " "))
			if len(args) != 2 || args[0] != "api" {
				return "", fmt.Errorf("no answer for %q", args)
			}
			if number, ok := strings.CutPrefix(args[1], listing); ok {
				page, err := strconv.Atoi(number)
				if err != nil || page < 1 {
					return "", fmt.Errorf("no page %q", number)
				}
				nodes := make([]map[string]any, 0)
				if page <= len(pages) {
					for i := range pages[page-1] {
						nodes = append(nodes, pages[page-1][i].rest())
					}
				}
				body, err := json.Marshal(nodes)
				return string(body), err
			}
			if number, ok := strings.CutPrefix(args[1], "repos/"+harvestSlug+"/pulls/"); ok {
				pr, err := strconv.Atoi(number)
				if login, known := authors[pr]; err == nil && known {
					return fmt.Sprintf(`{"number":%d,"user":{"login":%q}}`, pr, login), nil
				}
			}
			return "", fmt.Errorf("no answer for %q", args)
		})
	}
	t.Cleanup(func() { ghClient = restore })
	return calls
}

// historyHome is a state root and a checkout with nothing in either.
func historyHome(t *testing.T) state.Layout {
	t.Helper()
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	checkout := t.TempDir()
	restore := repoDir
	repoDir = func() (string, error) { return checkout, nil }
	t.Cleanup(func() { repoDir = restore })
	return layout
}

// fromHistory runs `cr rules suggest --from-history` and returns its report.
func fromHistory(t *testing.T, args ...string) rulesHistoryResult {
	t.Helper()
	printed, err := runCLIPrinting(t, append([]string{"rules", "suggest", "--repo", harvestSlug, "--from-history"},
		args...)...)
	require.NoError(t, err)
	var report rulesHistoryResult
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report
}

// urlsOf is the included comments by URL.
func urlsOf(report *rulesHistoryResult) []string {
	urls := make([]string, 0, len(report.Comments))
	for i := range report.Comments {
		urls = append(urls, report.Comments[i].URL)
	}
	return urls
}

// §2.6.3.5: the window is read by `created_at`, `since` inclusive and `until`
// exclusive, and the read stops at the first comment older than the window. A
// comment updated inside the window and created after it is not in it.
func TestTheWindowIsReadByCreationTime(t *testing.T) {
	historyHome(t)
	historyGH(t, map[int]string{7: "author"}, []aHistoryComment{
		{id: 5, pr: 7, login: "ayse", created: "2025-03-01T00:00:00Z", body: "after"},
		{id: 4, pr: 7, login: "ayse", created: "2025-02-10T09:00:00Z", body: "inside"},
		{id: 3, pr: 7, login: "ayse", created: "2025-01-01T00:00:00Z", body: "first day"},
		{id: 2, pr: 7, login: "ayse", created: "2024-12-31T23:59:59Z", body: "before"},
	})

	report := fromHistory(t, "--since", "2025-01-01", "--until", "2025-03-01")

	assert.Equal(t, []string{
		"https://github.com/acme/api/pull/7#discussion_r4", "https://github.com/acme/api/pull/7#discussion_r3",
	}, urlsOf(&report))
	assert.Equal(t, historyWindow{
		Since: "2025-01-01", Until: "2025-03-01", Limit: defaultHistoryLimit,
		Newest: "2025-02-10T09:00:00Z", Oldest: "2025-01-01T00:00:00Z",
	}, report.Window)
	assert.Equal(t, 2, report.Read)
}

// §2.6.3.5: the read stops at `--limit` opening comments and reports that the
// limit cut it; a window that ends exactly at the limit was not cut.
func TestALimitThatCutsTheWindowIsReported(t *testing.T) {
	historyHome(t)
	listing := []aHistoryComment{
		{id: 3, pr: 7, login: "ayse", created: "2025-02-03T00:00:00Z", body: "three"},
		{id: 2, pr: 7, login: "ayse", created: "2025-02-02T00:00:00Z", body: "two"},
		{id: 1, pr: 7, login: "ayse", created: "2025-02-01T00:00:00Z", body: "one"},
	}
	historyGH(t, map[int]string{7: "author"}, listing)

	cut := fromHistory(t, "--limit", "2")
	whole := fromHistory(t, "--limit", "3")

	assert.Len(t, cut.Comments, 2)
	assert.True(t, cut.Window.LimitCut)
	assert.Equal(t, "2025-02-02T00:00:00Z", cut.Window.Oldest)
	assert.Len(t, whole.Comments, 3)
	assert.False(t, whole.Window.LimitCut)
}

// §2.6.3.6: a bot, a reply and the pull request's own author are each left out
// and counted under their reason, and the author is read once per pull request.
func TestEachExclusionIsCountedByReason(t *testing.T) {
	historyHome(t)
	calls := historyGH(t, map[int]string{7: "author", 8: "other"}, []aHistoryComment{
		{id: 6, pr: 8, login: "ayse", created: "2025-02-06T00:00:00Z", body: "kept on 8"},
		{id: 5, pr: 7, login: "ayse", created: "2025-02-05T00:00:00Z", body: "kept on 7"},
		{id: 4, pr: 7, login: "Author", created: "2025-02-04T00:00:00Z", body: "the author's own"},
		{id: 3, pr: 7, login: "ayse", created: "2025-02-03T00:00:00Z", body: "a reply", replyTo: 1},
		{id: 2, pr: 7, login: "Copilot", kind: "Bot", created: "2025-02-02T00:00:00Z", body: "a bot"},
		{id: 1, pr: 7, login: "ayse", created: "2025-02-01T00:00:00Z", body: "kept, answered"},
	})

	report := fromHistory(t)

	assert.Equal(t, historyExcluded{Bot: 1, Reply: 1, PRAuthor: 1}, report.Excluded)
	assert.Equal(t, 6, report.Read)
	assert.Equal(t, 3, report.Included)
	reads := 0
	for _, call := range *calls {
		if strings.HasPrefix(call, "api repos/"+harvestSlug+"/pulls/7") {
			reads++
		}
	}
	assert.Equal(t, 1, reads, "§2.6.3.6's author read is one per pull request")
}

// §2.6.3.6: a reply is reported under the comment it answers, oldest first,
// and not as a comment of its own.
func TestAReplyIsReportedUnderTheCommentItAnswers(t *testing.T) {
	historyHome(t)
	historyGH(t, map[int]string{7: "author"}, []aHistoryComment{
		{id: 3, pr: 7, login: "author", created: "2025-02-03T00:00:00Z", body: "Updated.", replyTo: 1},
		{id: 2, pr: 7, login: "ayse", created: "2025-02-02T00:00:00Z", body: "Why?", replyTo: 1},
		{id: 1, pr: 7, login: "ayse", created: "2025-02-01T00:00:00Z", body: "Rename `total`."},
	})

	report := fromHistory(t)

	require.Len(t, report.Comments, 1)
	assert.Equal(t, []rule.HistoryReply{
		{URL: "https://github.com/acme/api/pull/7#discussion_r2", Author: "ayse",
			CreatedAt: "2025-02-02T00:00:00Z", Body: "Why?"},
		{URL: "https://github.com/acme/api/pull/7#discussion_r3", Author: "author",
			CreatedAt: "2025-02-03T00:00:00Z", Body: "Updated."},
	}, report.Comments[0].Replies)
}

// §2.6.3.7: each included comment carries its length in characters, and each
// month of the window its median; an even month is the mean of the middle two.
func TestEachMonthReportsItsMedianBodyLength(t *testing.T) {
	historyHome(t)
	historyGH(t, map[int]string{7: "author"}, []aHistoryComment{
		{id: 5, pr: 7, login: "ayse", created: "2025-02-02T00:00:00Z", body: strings.Repeat("ğ", 31)},
		{id: 4, pr: 7, login: "ayse", created: "2025-02-01T00:00:00Z", body: strings.Repeat("a", 10)},
		{id: 3, pr: 7, login: "ayse", created: "2025-01-03T00:00:00Z", body: strings.Repeat("a", 40)},
		{id: 2, pr: 7, login: "ayse", created: "2025-01-02T00:00:00Z", body: strings.Repeat("a", 10)},
		{id: 1, pr: 7, login: "ayse", created: "2025-01-01T00:00:00Z", body: strings.Repeat("a", 20)},
	})

	report := fromHistory(t)

	assert.Equal(t, 31, report.Comments[0].BodyChars, "a length is counted in characters, not bytes")
	assert.Equal(t, []rule.MonthMedian{
		{Month: "2025-01", Comments: 3, MedianBodyChars: 20},
		{Month: "2025-02", Comments: 2, MedianBodyChars: 20.5},
	}, report.Months)
}

// §2.6.3.8: a group is reported when it spans `rules.harvest_min` distinct pull
// requests. A code span repeated five times on one pull request is one
// migration and is not reported; one written once on each of three is.
func TestGroupsCountDistinctPullRequests(t *testing.T) {
	historyHome(t)
	listing := make([]aHistoryComment, 0)
	for i := range 5 {
		listing = append(listing, aHistoryComment{id: int64(20 + i), pr: 9, login: "ayse",
			created: fmt.Sprintf("2025-02-%02dT00:00:00Z", 20-i), body: "drop the `->value`", path: "app/Enums/Kind.php"})
	}
	for i, pr := range []int{7, 8, 10} {
		listing = append(listing, aHistoryComment{id: int64(10 + i), pr: pr, login: "ayse",
			created: fmt.Sprintf("2025-02-%02dT00:00:00Z", 10-i), body: "Testi yazılabilir\n`Rule::enum`",
			path: "app/Models/User.php"})
	}
	historyGH(t, map[int]string{7: "a", 8: "a", 9: "a", 10: "a"}, listing)

	report := fromHistory(t)

	assert.Equal(t, []rule.HistoryGroup{{
		Key: "Rule::enum", PRs: []int{7, 8, 10}, DistinctPRs: 3, Occurrences: 3,
		Comments: []string{
			"https://github.com/acme/api/pull/7#discussion_r10", "https://github.com/acme/api/pull/8#discussion_r11",
			"https://github.com/acme/api/pull/10#discussion_r12",
		},
	}}, report.Groups.ByCodeSpan)
	require.Len(t, report.Groups.ByPath, 1)
	assert.Equal(t, "app/Models/*.php", report.Groups.ByPath[0].Key)
	require.Len(t, report.Groups.ByBody, 1)
	assert.Equal(t, "Testi yazılabilir\n`Rule::enum`", report.Groups.ByBody[0].Key)
	assert.Equal(t, 3, report.Min)
}

