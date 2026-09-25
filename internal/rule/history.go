package rule

import (
	"cmp"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/deligoez/cr/internal/text"
)

// HistoryComment is one human review comment of a repository's history as
// §2.6.3.7 reports it: where it was written, by whom and when, what it says,
// the replies it drew, and its length.
//
// It is a report for the agent and carries no judgement. §2.6.3.7 forbids cr
// to decide which comments an agent drafted, so the length is stated beside the
// body and nothing is concluded from it.
type HistoryComment struct {
	PR        int            `json:"pr"`
	URL       string         `json:"url"`
	Author    string         `json:"author"`
	CreatedAt string         `json:"created_at"`
	Path      string         `json:"path"`
	Line      int            `json:"line"`
	Side      string         `json:"side"`
	Body      string         `json:"body"`
	Replies   []HistoryReply `json:"replies"`
	BodyChars int            `json:"body_chars"`
}

// HistoryReply is one reply §2.6.3.6 reports under the comment it answers.
type HistoryReply struct {
	URL       string `json:"url"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	Body      string `json:"body"`
}

// BodyChars is a body's length in characters, the unit §2.6.3.7's length is
// read in: a Turkish sentence counted in bytes would read longer than the same
// sentence in English.
func BodyChars(body string) int { return utf8.RuneCountInString(body) }

// MonthMedian is §2.6.3.7's median body length for one month of the window.
type MonthMedian struct {
	// Month is `YYYY-MM`, read off the comments' `created_at`.
	Month string `json:"month"`
	// Comments is how many included comments the month holds.
	Comments int `json:"comments"`
	// MedianBodyChars is the median of their BodyChars; with an even
	// count it is the mean of the middle two.
	MedianBodyChars float64 `json:"median_body_chars"`
}

// MonthMedians is §2.6.3.7's per-month median over the included comments, in
// ascending month order. A month no included comment was written in has no
// median and is not listed.
func MonthMedians(comments []HistoryComment) []MonthMedian {
	byMonth := make(map[string][]int)
	for i := range comments {
		month := comments[i].CreatedAt[:min(len(comments[i].CreatedAt), len("2006-01"))]
		byMonth[month] = append(byMonth[month], comments[i].BodyChars)
	}
	medians := make([]MonthMedian, 0, len(byMonth))
	for _, month := range slices.Sorted(maps.Keys(byMonth)) {
		lengths := byMonth[month]
		slices.Sort(lengths)
		middle := len(lengths) / 2
		median := float64(lengths[middle])
		if len(lengths)%2 == 0 {
			median = float64(lengths[middle-1]+lengths[middle]) / 2
		}
		medians = append(medians, MonthMedian{Month: month, Comments: len(lengths), MedianBodyChars: median})
	}
	return medians
}

// HistoryGroup is one group of §2.6.3.8: the key that formed it, the distinct
// pull requests it spans, how many comments reached it, and their URLs.
type HistoryGroup struct {
	Key         string   `json:"key"`
	PRs         []int    `json:"prs"`
	DistinctPRs int      `json:"distinct_prs"`
	Occurrences int      `json:"occurrences"`
	Comments    []string `json:"comments"`
}

// HistoryGroups are §2.6.3.8's three groupings.
type HistoryGroups struct {
	ByPath     []HistoryGroup `json:"by_path"`
	ByCodeSpan []HistoryGroup `json:"by_code_span"`
	ByBody     []HistoryGroup `json:"by_body"`
}

// GroupHistory is §2.6.3.8's grouping of the included comments by path shape,
// by backticked code span and by §1.4's normal form of the body, keeping the
// groups that span at least threshold distinct pull requests.
//
// The threshold counts pull requests rather than comments: one pull request
// can carry dozens of comments about one migration, and a repetition inside
// one change is not yet a standard the team holds. A comment counts once
// toward a code span however often it writes it.
//
// Groups are ordered by distinct pull requests, then occurrences, then key, so
// the same history gives the same report (§2.1.1).
func GroupHistory(comments []HistoryComment, threshold int) (HistoryGroups, error) {
	byPath, byCode, byBody := newGrouping(), newGrouping(), newGrouping()
	for i := range comments {
		comment := &comments[i]
		if shape := PathShape(comment.Path); shape != "" {
			byPath.add(shape, comment)
		}
		for _, span := range CodeSpans(comment.Body) {
			byCode.add(span, comment)
		}
		normalised, err := text.Normalise(comment.Body)
		if err != nil {
			return HistoryGroups{}, err
		}
		byBody.add(normalised, comment)
	}
	return HistoryGroups{
		ByPath: byPath.reaching(threshold), ByCodeSpan: byCode.reaching(threshold),
		ByBody: byBody.reaching(threshold),
	}, nil
}

// PathShape is §2.6.3.8's path shape, the directory and the extension in the
// shape a rule's `globs` takes: `app/Models/User.php` is `app/Models/*.php`.
func PathShape(file string) string {
	if file == "" {
		return ""
	}
	dir, ext := path.Dir(file), path.Ext(path.Base(file))
	if dir == "." {
		return "*" + ext
	}
	return dir + "/*" + ext
}

// codeSpan is one backticked inline span.
var codeSpan = regexp.MustCompile("`([^`\n]+)`")

// CodeSpans is the distinct backticked inline code spans of a body, in the
// order they first appear. A fenced block — a suggestion among them — is code
// the comment proposes rather than a name it mentions, and is skipped.
func CodeSpans(body string) []string {
	var prose strings.Builder
	fenced := false
	for line := range strings.Lines(body) {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if !fenced {
			prose.WriteString(line)
		}
	}
	spans := make([]string, 0)
	for _, match := range codeSpan.FindAllStringSubmatch(prose.String(), -1) {
		span := strings.TrimSpace(match[1])
		if span != "" && !slices.Contains(spans, span) {
			spans = append(spans, span)
		}
	}
	return spans
}

// grouping is one of the three groupings as it is formed.
type grouping struct {
	at     map[string]int
	groups []HistoryGroup
}

func newGrouping() *grouping { return &grouping{at: make(map[string]int)} }

func (g *grouping) add(key string, comment *HistoryComment) {
	i, seen := g.at[key]
	if !seen {
		i = len(g.groups)
		g.at[key] = i
		g.groups = append(g.groups, HistoryGroup{Key: key, PRs: make([]int, 0), Comments: make([]string, 0)})
	}
	group := &g.groups[i]
	if !slices.Contains(group.PRs, comment.PR) {
		group.PRs = append(group.PRs, comment.PR)
	}
	group.Occurrences++
	group.Comments = append(group.Comments, comment.URL)
}

func (g *grouping) reaching(threshold int) []HistoryGroup {
	kept := make([]HistoryGroup, 0)
	for _, group := range g.groups {
		group.DistinctPRs = len(group.PRs)
		if group.DistinctPRs >= threshold {
			slices.Sort(group.PRs)
			kept = append(kept, group)
		}
	}
	slices.SortStableFunc(kept, func(a, b HistoryGroup) int {
		return cmp.Or(
			cmp.Compare(b.DistinctPRs, a.DistinctPRs),
			cmp.Compare(b.Occurrences, a.Occurrences),
			cmp.Compare(a.Key, b.Key),
		)
	})
	return kept
}
