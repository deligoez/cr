package rule

import (
	"github.com/deligoez/cr/internal/text"
)

// Comment is one comment posted from a recorded round, as §2.6.3.1 reads it:
// where it was posted, what class the record behind it carried, and the body
// the author received.
//
// The class travels with the body because §2.6.3.1 groups by both. A rule
// candidate is a thing that keeps being said about one kind of defect, and two
// identical sentences written about different classes are two observations
// rather than one repetition — the class is what a harvested rule would end up
// declaring, so it is part of what makes the occurrences the same occurrence.
//
// The body is kept as it was posted rather than as it normalises. §2.6.3.2
// reports the candidate together with the comments that formed it, and a
// reviewer deciding whether to write the rule reads what was actually sent;
// the normal form is the key, not the evidence.
type Comment struct {
	// PR is the pull request the comment was posted on.
	PR int `json:"pr"`
	// Round is the round it was posted from.
	Round int `json:"round"`
	// Record is the id of the record it was drawn from.
	Record string `json:"record"`
	// Class is that record's class.
	Class string `json:"class"`
	// Body is the comment as the author received it.
	Body string `json:"body"`
}

// Candidate is §2.6.3.2's report for one group: the class and normalised body
// that made it a group, how many comments reached it, and every one of them.
//
// It is a report and nothing else. §2.6.3.3 forbids cr to write a rule file by
// itself, so there is no rule id here, no detect block, and no field a writer
// would need — what a candidate carries is what a human needs in order to
// decide whether to write one.
type Candidate struct {
	// Class is the class every comment of the group carried.
	Class string `json:"class"`
	// Body is §1.4's normal form of the body they share, which is the
	// other half of what made them one group.
	Body string `json:"body"`
	// Occurrences is how many comments formed the group. It is written
	// beside Comments rather than left to be counted, because
	// `rules.harvest_min` is a threshold on this number and a reader
	// comparing the two is reading the decision cr made.
	Occurrences int `json:"occurrences"`
	// Comments are those comments, in the order they were scanned.
	Comments []Comment `json:"comments"`
}

// Harvest is §2.6.3.1's grouping and §2.6.3.2's threshold over the comments
// posted from a repository's recorded rounds.
//
// Grouping is by class and by §1.4's normal form of the body, which is what
// makes a candidate a repetition rather than a coincidence: §1.4 collapses
// whitespace and line endings and nothing else, so two comments group only
// when a person would read them as the same sentence — it does not case-fold,
// strip punctuation, or reorder lines, and two wordings of one idea stay two
// comments.
//
// The order is the order the groups were first seen, and the comments inside a
// group are in the order they were scanned. §2.1.1 requires the same inputs to
// give the same result, and a report built out of a map's iteration order would
// be a different document on every run over one repository.
//
// A body §1.4 refuses — invalid UTF-8 — stops the harvest and names nothing
// else, for the reason §1.4 gives it exit code 1: the normal form is the group
// key, so a body that has none cannot be grouped, and a scan that skipped it
// would report a candidate count that quietly excluded a comment.
func Harvest(comments []Comment, threshold int) ([]Candidate, error) {
	type key struct{ class, body string }
	at := make(map[key]int, len(comments))
	grouped := make([]Candidate, 0, len(comments))
	for _, comment := range comments {
		normalised, err := text.Normalise(comment.Body)
		if err != nil {
			return nil, err
		}
		grouping := key{class: comment.Class, body: normalised}
		if i, seen := at[grouping]; seen {
			grouped[i].Comments = append(grouped[i].Comments, comment)
			grouped[i].Occurrences++
			continue
		}
		at[grouping] = len(grouped)
		grouped = append(grouped, Candidate{
			Class: comment.Class, Body: normalised,
			Occurrences: 1, Comments: []Comment{comment},
		})
	}
	return reaching(grouped, threshold), nil
}

// reaching is §2.6.3.2's threshold: the groups whose occurrence count reached
// `rules.harvest_min`, in the order they were formed.
//
// The comparison is `>=` because §2.6.3.2 says reaching the count and not
// exceeding it: with the default of 3, the third comment is what makes a
// candidate, and a group of exactly three is the smallest thing the section
// asks cr to report.
func reaching(grouped []Candidate, threshold int) []Candidate {
	candidates := make([]Candidate, 0, len(grouped))
	for _, group := range grouped {
		if group.Occurrences >= threshold {
			candidates = append(candidates, group)
		}
	}
	return candidates
}
