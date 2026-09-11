package draft

import (
	"fmt"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// HeaderFacts is what §7.1.4's summary header needs from outside the queued
// records: the cap they are counted against, and the round's coverage state.
type HeaderFacts struct {
	// MaxComments is the resolved post.max_comments of §2.7. It is handed
	// in rather than read here, for the reason finding.CommentCap gives: §2.7
	// resolves it across five layers, and a second path to it is a second
	// answer.
	MaxComments int
	// Coverage is the round's coverage state, counted per §10.1.1.
	Coverage coverage.Rows
}

// The delimiters of the header. It is one HTML comment, so nothing inside it
// reaches a rendered view, and it opens with §8.1.3's reserved sequence because
// it is cr's: §8.1.3 keeps that sequence out of every body, so no block can
// hold a line mistaken for the header.
const (
	headerOpen  = render.Reserved + "summary"
	headerClose = "-->"
)

// File is §7.1's draft.md whole: §7.1.4's summary header, then every queued
// record as one block per Render, carrying the bodies preserved holds.
//
// The header always opens the file, including a round with nothing queued. A
// file that opened with a block on some rounds and with nothing on others would
// leave the reviewer to infer the counts from what is absent, and the header is
// where the counts are said.
func File(
	queued []*finding.Finding, lang render.Lang, sources *Provenances,
	preserved map[string]string, facts HeaderFacts,
) (string, error) {
	blocks, err := Render(queued, lang, sources, preserved)
	if err != nil {
		return "", err
	}
	if blocks == "" {
		return header(queued, facts), nil
	}
	return header(queued, facts) + "\n" + blocks, nil
}

// header is §7.1.4's summary header: counts, the coverage state, and the
// comment count against post.max_comments, as a comment that is not posted.
//
// The comment count is finding.CommentCapFor's disclosure, verbatim. §1.6.2's
// cap check reads its number from that one call, so the count the reviewer
// triages against here and the count that blocks `cr post` are one number by
// construction rather than two that happen to agree — and the header names the
// excess in the words the block will use.
//
// It is English for the reason §6.1.1's summary is: the header is cr's report
// to the reviewer and the agent, never text the author reads.
func header(queued []*finding.Finding, facts HeaderFacts) string {
	lines := []string{
		headerOpen,
		"Not posted. cr regenerates this header on every `cr draft`.",
		fmt.Sprintf("records: %d queued", len(queued)),
		"kind: " + tally(queued, kinds, func(r *finding.Finding) string { return string(r.Kind) }),
		"severity: " + tally(queued, severities, func(r *finding.Finding) string { return string(r.Severity) }),
		"grade: " + tally(queued, grades, func(r *finding.Finding) string { return string(r.Grade) }),
		"coverage: " + coverageLine(facts.Coverage),
		"comments: " + finding.CommentCapFor(queued, facts.MaxComments).Disclosure(),
		headerClose,
	}
	return strings.Join(lines, "\n") + "\n"
}

// The §6.1 vocabularies the header counts by, each in the order §6.1 gives
// it, so the header reads the same way on every round.
var (
	kinds      = []string{string(finding.KindFinding), string(finding.KindQuestion)}
	severities = []string{
		string(finding.SeverityCritical), string(finding.SeverityHigh),
		string(finding.SeverityMedium), string(finding.SeverityLow),
	}
	grades = []string{
		string(finding.GradeProbed), string(finding.GradeCited), string(finding.GradeArgued),
	}
)

// tally counts the queued records by one field: every value of the vocabulary
// in its order, zeros included, and then any value outside it in the order it
// first appears.
//
// A value outside the vocabulary is listed rather than dropped. `cr record`
// refuses one on the way in, so none should reach here; a header that lost a
// record to a count it had no row for would under-report the round and say
// nothing about it.
func tally(queued []*finding.Finding, vocabulary []string, field func(*finding.Finding) string) string {
	order := append(make([]string, 0, len(vocabulary)), vocabulary...)
	counts := make(map[string]int, len(vocabulary))
	for _, record := range queued {
		value := field(record)
		if !slices.Contains(order, value) {
			order = append(order, value)
		}
		counts[value]++
	}
	parts := make([]string, 0, len(order))
	for _, value := range order {
		parts = append(parts, fmt.Sprintf("%s %d", value, counts[value]))
	}
	return strings.Join(parts, ", ")
}

// coverageLine is §10.1.1's four unit counts and the active roles a complete
// row is counted against.
func coverageLine(rows coverage.Rows) string {
	return fmt.Sprintf(
		"%d unit(s) against %d active role(s): %d with a complete row of cells, %d with gaps, %d oversized",
		rows.Units, rows.Roles, rows.Complete, rows.Gaps, rows.Oversized)
}
