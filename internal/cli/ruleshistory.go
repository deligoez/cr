package cli

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
)

// historyDate is §2.6.3.5's date form.
const historyDate = "2006-01-02"

// defaultHistoryLimit is §2.6.3.5's default `--limit`.
const defaultHistoryLimit = 1000

// historyWindow is §2.6.3.5's report of the window read: the bounds asked for,
// the creation times of the newest and oldest comment read inside them, and
// whether `--limit` stopped the read before the window was exhausted.
//
// `since` is inclusive and `until` exclusive, both at midnight UTC, so
// `--since 2025-01-01 --until 2025-03-01` is January and February.
type historyWindow struct {
	Since    string `json:"since"`
	Until    string `json:"until"`
	Limit    int    `json:"limit"`
	LimitCut bool   `json:"limit_cut"`
	Newest   string `json:"newest_read"`
	Oldest   string `json:"oldest_read"`
}

// historyExcluded is §2.6.3.6's count of the comments read and left out, by
// reason. A comment is counted once, under the first reason in this order.
type historyExcluded struct {
	Bot      int `json:"bot"`
	Reply    int `json:"reply"`
	PRAuthor int `json:"pr_author"`
}

// rulesHistoryResult is `cr rules suggest --from-history`'s report
// (§2.6.3.5 to §2.6.3.8). Like the posted-comment harvest it has no field
// through which a rule could be written.
type rulesHistoryResult struct {
	Repo     string                `json:"repo"`
	Min      int                   `json:"harvest_min"`
	Window   historyWindow         `json:"window"`
	Read     int                   `json:"read"`
	Included int                   `json:"included"`
	Excluded historyExcluded       `json:"excluded"`
	Months   []rule.MonthMedian    `json:"months"`
	Groups   rule.HistoryGroups    `json:"groups"`
	Rules    []string              `json:"resolved_rules"`
	Comments []rule.HistoryComment `json:"comments"`
}

// Text summarises the window, the exclusions, the month medians and the
// groups, and says that cr wrote nothing. The comments themselves are in the
// JSON document, which is what an agent reads.
func (r *rulesHistoryResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s: %s review comment(s) included of %d read in the window [%s, %s)",
		w.accent(r.Repo), w.accent(strconv.Itoa(r.Included)), r.Read, orOpen(r.Window.Since), orOpen(r.Window.Until))
	if r.Window.LimitCut {
		fmt.Fprintf(&out, "\n  --limit %d cut the read at %s; older comments of the window were not read",
			r.Window.Limit, r.Window.Oldest)
	}
	fmt.Fprintf(&out, "\n  excluded: %d bot, %d reply, %d by the pull request's author",
		r.Excluded.Bot, r.Excluded.Reply, r.Excluded.PRAuthor)
	for _, month := range r.Months {
		fmt.Fprintf(&out, "\n  %s: %d comment(s), median %.1f characters", month.Month, month.Comments,
			month.MedianBodyChars)
	}
	for _, named := range []struct {
		label  string
		groups []rule.HistoryGroup
	}{
		{"path", r.Groups.ByPath}, {"code span", r.Groups.ByCodeSpan}, {"body", r.Groups.ByBody},
	} {
		for _, group := range named.groups {
			fmt.Fprintf(&out, "\n  %s %q: %d pull request(s), %d occurrence(s)",
				named.label, group.Key, group.DistinctPRs, group.Occurrences)
		}
	}
	fmt.Fprintf(&out, "\n%d resolved rule(s); the report is for the agent to draw rules from (§2.6.3.8), "+
		"and cr wrote nothing", len(r.Rules))
	return out.String()
}

// orOpen prints an absent bound as an open one.
func orOpen(bound string) string {
	if bound == "" {
		return "…"
	}
	return bound
}

// historyFlags registers §2.6.3.5's flags on `cr rules suggest`.
func historyFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("from-history", false,
		"report the repository's human review comments for the agent to draw rules from (§2.6.3.5)")
	cmd.Flags().String("since", "", "with --from-history, the first day of the window, YYYY-MM-DD, inclusive")
	cmd.Flags().String("until", "", "with --from-history, the day the window ends, YYYY-MM-DD, exclusive")
	cmd.Flags().Int("limit", defaultHistoryLimit, "with --from-history, the most opening comments to report")
}

// historyRequest is the window and limit a run was asked for.
type historyRequest struct {
	since, until time.Time
	window       historyWindow
}

// historyRequestOf reads §2.6.3.5's flags. A window flag without
// --from-history, a date not in `YYYY-MM-DD`, an empty window and a limit
// below one are each a malformed invocation.
func historyRequestOf(cmd *cobra.Command) (*historyRequest, bool, error) {
	from, err := cmd.Flags().GetBool("from-history")
	if err != nil {
		return nil, false, err
	}
	if !from {
		for _, name := range []string{"since", "until", "limit"} {
			if cmd.Flags().Changed(name) {
				return nil, false, fmt.Errorf("--%s is a flag of `cr rules suggest --from-history` (§2.6.3.5)", name)
			}
		}
		return nil, false, nil
	}
	request := &historyRequest{}
	if request.window.Limit, err = cmd.Flags().GetInt("limit"); err != nil {
		return nil, false, err
	}
	if request.window.Limit < 1 {
		return nil, false, fmt.Errorf("--limit is %d, and §2.6.3.5 reads at least one comment", request.window.Limit)
	}
	for _, bound := range []struct {
		name string
		at   *time.Time
		text *string
	}{
		{"since", &request.since, &request.window.Since}, {"until", &request.until, &request.window.Until},
	} {
		if *bound.text, err = cmd.Flags().GetString(bound.name); err != nil || *bound.text == "" {
			continue
		}
		if *bound.at, err = time.Parse(historyDate, *bound.text); err != nil {
			return nil, false, fmt.Errorf("--%s is %q, and §2.6.3.5 writes a date YYYY-MM-DD", bound.name, *bound.text)
		}
	}
	if !request.since.IsZero() && !request.until.IsZero() && !request.since.Before(request.until) {
		return nil, false, fmt.Errorf("--since %s is not before --until %s, so the window is empty",
			request.window.Since, request.window.Until)
	}
	return request, true, nil
}

// harvest is one run's reading of the history: the comments read so far, and
// the one read per pull request §2.6.3.6 needs for its author.
type harvest struct {
	client  gh.Client
	owner   string
	repo    string
	request *historyRequest
	authors map[int]string
	roots   []int64
	replies map[int64][]rule.HistoryReply
	result  *rulesHistoryResult
}

// emitHistory runs §2.6.3.5 to §2.6.3.8 and prints the report. Every read is a
// gh GET or a lock-free read of §2.2's tree, and nothing is written.
func emitHistory(out *writer, owner, repo string, request *historyRequest) error {
	layout, err := state.Default()
	if err != nil {
		return err
	}
	threshold, err := harvestMin(layout, owner, repo)
	if err != nil {
		return err
	}
	listed, _, err := effectiveRules(layout, owner, repo)
	if err != nil {
		return err
	}
	run := &harvest{
		client: ghClient(), owner: owner, repo: repo, request: request,
		authors: make(map[int]string), replies: make(map[int64][]rule.HistoryReply),
		result: &rulesHistoryResult{
			Repo: owner + "/" + repo, Min: threshold, Window: request.window,
			Rules:    make([]string, 0, len(listed.result.Rules)+len(listed.result.OutOfProfile)),
			Comments: make([]rule.HistoryComment, 0),
		},
	}
	for _, resolved := range listed.result.Rules {
		run.result.Rules = append(run.result.Rules, resolved.ID)
	}
	for _, scoped := range listed.result.OutOfProfile {
		run.result.Rules = append(run.result.Rules, scoped.ID)
	}
	if err := run.read(); err != nil {
		return err
	}
	run.attachReplies()
	run.result.Included = len(run.result.Comments)
	run.result.Months = rule.MonthMedians(run.result.Comments)
	if run.result.Groups, err = rule.GroupHistory(run.result.Comments, threshold); err != nil {
		return err
	}
	return out.emit(run.result)
}

// errStop ends the page walk: the window is behind the read, or the limit is
// reached with more of the window unread.
var errStop = errors.New("stop")

// read walks the listing newest first until the window or the listing ends,
// or until the limit is reached.
func (h *harvest) read() error {
	for page := 1; ; page++ {
		comments, last, err := h.client.ReviewCommentsPage(h.owner, h.repo, page)
		if err != nil {
			return err
		}
		for i := range comments {
			if err := h.take(&comments[i]); errors.Is(err, errStop) {
				return nil
			} else if err != nil {
				return err
			}
		}
		if last {
			return nil
		}
	}
}

// take reads one comment of the listing into the report.
func (h *harvest) take(comment *gh.HistoryComment) error {
	created, err := time.Parse(time.RFC3339, comment.CreatedAt)
	if err != nil {
		return fmt.Errorf("review comment %s has created_at %q, which is not RFC 3339", comment.URL, comment.CreatedAt)
	}
	if !h.request.until.IsZero() && !created.Before(h.request.until) {
		return nil
	}
	if !h.request.since.IsZero() && created.Before(h.request.since) {
		return errStop
	}
	if len(h.result.Comments) == h.request.window.Limit {
		h.result.Window.LimitCut = true
		return errStop
	}
	h.result.Read++
	if h.result.Window.Newest == "" {
		h.result.Window.Newest = comment.CreatedAt
	}
	h.result.Window.Oldest = comment.CreatedAt
	switch {
	case comment.AuthorType == gh.AuthorBot:
		h.result.Excluded.Bot++
	case comment.InReplyTo != 0:
		h.result.Excluded.Reply++
		h.replies[comment.InReplyTo] = append(h.replies[comment.InReplyTo], rule.HistoryReply{
			URL: comment.URL, Author: comment.Author, CreatedAt: comment.CreatedAt, Body: comment.Body,
		})
	default:
		author, err := h.pullAuthor(comment.PR)
		if err != nil {
			return err
		}
		if author != "" && strings.EqualFold(author, comment.Author) {
			h.result.Excluded.PRAuthor++
			return nil
		}
		h.roots = append(h.roots, comment.ID)
		h.result.Comments = append(h.result.Comments, rule.HistoryComment{
			PR: comment.PR, URL: comment.URL, Author: comment.Author, CreatedAt: comment.CreatedAt,
			Path: comment.Path, Line: comment.Line, Side: comment.Side, Body: comment.Body,
			Replies: make([]rule.HistoryReply, 0), BodyChars: rule.BodyChars(comment.Body),
		})
	}
	return nil
}

// pullAuthor is the login that opened one pull request, read once per pull
// request per run.
func (h *harvest) pullAuthor(pr int) (string, error) {
	if author, read := h.authors[pr]; read {
		return author, nil
	}
	author, err := h.client.PullAuthor(h.owner, h.repo, pr)
	if err != nil {
		return "", err
	}
	h.authors[pr] = author
	return author, nil
}

// attachReplies puts every reply read under the included comment it answers,
// oldest first. The listing is newest first and a reply is newer than the
// comment it answers, so each list is reversed.
func (h *harvest) attachReplies() {
	for i, id := range h.roots {
		answers := slices.Clone(h.replies[id])
		slices.Reverse(answers)
		h.result.Comments[i].Replies = append(h.result.Comments[i].Replies, answers...)
	}
}
