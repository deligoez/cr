package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/state"
)

// tally is one row of §10.1.4's counts: a value of a closed vocabulary, and how
// many of the round's records hold it.
//
// It is a slice of pairs rather than a JSON object because the order is part of
// the answer. §9.1's table, §6.1's severities and §6.2's grades are each
// written in an order that means something — a life cycle, a scale, a strength
// — and a map would hand the reader whichever order the encoder felt like.
type tally struct {
	// Name is the value as the record holds it, or `(none)` for a record
	// that holds none.
	Name string `json:"name"`
	// Count is how many of the round's records hold it.
	Count int `json:"count"`
}

// unsetTally is what an empty value is called in a tally. §6.1.4 has cr write
// `state` and `grade` itself, so an empty one is not a record the agent left
// incomplete but a line nothing in cr wrote — and printing it under an empty
// name would render as a count with no subject.
const unsetTally = "(none)"

// recordReport is §10.1.4: the round's findings and questions counted by state,
// by severity, and by grade.
//
// Every value of each vocabulary appears, at zero as well, for the reason
// finding.Drops prints its own zero case: a row that appeared only when it was
// non-zero would leave a reader unable to tell a round that produced nothing in
// that bucket from a report that never counted it. The three counts each sum to
// Total by construction, since a value outside the vocabulary is given a row of
// its own rather than dropped.
type recordReport struct {
	// Total is how many findings and questions the round holds.
	Total int `json:"total"`
	// ByState counts §9.1's seven, in that table's order.
	ByState []tally `json:"by_state"`
	// BySeverity counts §6.1's four, highest first.
	BySeverity []tally `json:"by_severity"`
	// ByGrade counts §6.2's three, strongest first.
	ByGrade []tally `json:"by_grade"`
}

// probeReport is §10.1.5: how many probes the round ran, and how many of them
// changed a finding's grade.
type probeReport struct {
	// Run is how many probe records the round wrote.
	Run int `json:"run"`
	// Graded is how many distinct probes stand behind a record graded
	// `probed`.
	//
	// §6.2's table has `probed` reachable only through a probe whose head
	// matches and whose result supports the record, so a record holding
	// that grade holds it because of its probe and would hold `cited` or
	// `argued` without it. That is what changing a finding's grade is, read
	// off the two files rather than out of a history cr does not keep.
	//
	// The probes are counted distinctly and across rounds. §5.5.2 lets one
	// probe stand behind several records, and counting the references would
	// report one experiment as several; §5.5.3 admits a probe recorded in an
	// earlier round at the same head, so a count narrowed to this round's
	// probes would leave such a grade unexplained.
	Graded int `json:"graded"`
}

// recordTalliesOf counts §10.1.4 over the round's findings and questions.
func recordTalliesOf(records []*finding.Finding) recordReport {
	states := make([]string, 0, len(records))
	severities := make([]string, 0, len(records))
	grades := make([]string, 0, len(records))
	for _, record := range records {
		states = append(states, record.State.String())
		severities = append(severities, string(record.Severity))
		grades = append(grades, string(record.Grade))
	}
	return recordReport{
		Total:      len(records),
		ByState:    tallyOf(stateNames(), states),
		BySeverity: tallyOf(namesOf(finding.Severities()), severities),
		ByGrade:    tallyOf(namesOf(finding.Grades()), grades),
	}
}

// stateNames is §9.1's seven states as a tally counts them, in that table's
// order. finding.State is a struct rather than a string type — §9.1's
// vocabulary is closed by construction — so it needs its own rendering rather
// than namesOf's.
func stateNames() []string {
	held := finding.States()
	names := make([]string, 0, len(held))
	for _, value := range held {
		names = append(names, value.String())
	}
	return names
}

// namesOf renders a closed vocabulary of string-based values as the names a
// tally counts them under, in the order the vocabulary gives them.
func namesOf[T ~string](values []T) []string {
	names := make([]string, 0, len(values))
	for _, value := range values {
		names = append(names, string(value))
	}
	return names
}

// tallyOf counts held against vocabulary: one row per vocabulary value in its
// own order, then one row for each value held that the vocabulary does not
// list, in the order those first appeared.
//
// The trailing rows are what keep the report honest about a file cr did not
// write. §6.1.4 refuses `state` and `grade` on the wire and cr stamps both, so
// a stored value outside the vocabulary means the line was hand-edited or
// written by a version this one does not share a vocabulary with — and a tally
// that silently dropped it would report fewer records than Total says there
// are.
func tallyOf(vocabulary, held []string) []tally {
	counts := make(map[string]int, len(vocabulary))
	order := make([]string, 0, len(vocabulary))
	for _, name := range vocabulary {
		counts[name] = 0
		order = append(order, name)
	}
	for _, value := range held {
		name := value
		if name == "" {
			name = unsetTally
		}
		if _, listed := counts[name]; !listed {
			order = append(order, name)
		}
		counts[name]++
	}
	rows := make([]tally, 0, len(order))
	for _, name := range order {
		rows = append(rows, tally{Name: name, Count: counts[name]})
	}
	return rows
}

// probesOf counts §10.1.5 over the round's probe records and its findings.
func probesOf(probes []probe.Record, records []*finding.Finding) probeReport {
	graded := make([]string, 0, len(records))
	for _, record := range records {
		if record.Grade != finding.GradeProbed || record.Probe == "" {
			continue
		}
		if !slices.Contains(graded, record.Probe) {
			graded = append(graded, record.Probe)
		}
	}
	return probeReport{Run: len(probes), Graded: len(graded)}
}

// duplicateDisclosure is §10.1.6's duplicate count, one of the seven reports
// §11.1 exempts from `--quiet`.
//
// It is read off findings.ndjson rather than out of the round summary. §6.4.3
// retains a suppressed duplicate in state `duplicate` with `duplicate_of`
// naming the representative, and `cr record` stamps that state, so the round's
// own records are what the suppression actually left behind — a count taken
// from anywhere else could disagree with the file this report is about.
//
// It is printed at zero, for the reason finding.Overlaps.Disclosure is: a
// reader told nothing cannot tell a round in which no two roles overlapped from
// a round in which the dedup pass was never reached.
func duplicateDisclosure(records []*finding.Finding) string {
	suppressed, lines := 0, make([]string, 0, len(records))
	for _, record := range records {
		if record.State != finding.StateDuplicate {
			continue
		}
		suppressed++
		if record.DuplicateOf != "" && !slices.Contains(lines, record.DuplicateOf) {
			lines = append(lines, record.DuplicateOf)
		}
	}
	return fmt.Sprintf("%d record(s) suppressed as duplicates across %d anchored line(s), per §6.4.3",
		suppressed, len(lines))
}

// summaryWaived is §10.3's waived count, by the key `rounds/<n>/summary.json`
// holds it under. §6.5.1 makes `cr merge` its author.
const summaryWaived = "waived"

// waiverDisclosure is §10.1.6's waiver count, the other of the pair §11.1
// exempts from `--quiet`.
//
// It is read out of the round summary because that is the only place it exists.
// §6.4.4 has a dropped finding never written to findings.ndjson at all, so that
// no record sits in a state §9.1 does not define, and says in as many words that
// only the count reaches the round summary — a status report deriving its own
// would have to re-run the merge against today's waivers and would answer about
// a pass that never happened.
//
// A round whose summary carries no such count is reported as one, not as zero.
// The two are different facts and the second is the one worth knowing: §6.5.1
// makes `cr merge` write the count, so its absence says the merge has not run
// for this round and no waiver has been applied to it yet, while a zero says
// the pass ran and matched nothing.
func waiverDisclosure(
	l state.Layout, owner, repo string, pr, round int,
) (string, error) {
	applied, recorded, err := state.ReadRoundSection[finding.Drops](
		l, owner, repo, pr, round, state.FileSummary, summaryWaived)
	if err != nil {
		return "", err
	}
	if !recorded {
		return "no §6.4.4 waiver drop recorded for round " + strconv.Itoa(round) +
			": §6.5.1 has `cr merge` write the count into summary.json, and it has not run" +
			" for this round", nil
	}
	return applied.Disclosure(), nil
}

// tallyLine renders one of §10.1.4's counts for the terminal: the dimension's
// name, then every row of it, in the order the tally holds them.
func tallyLine(dimension string, rows []tally) string {
	var out strings.Builder
	out.WriteString("  by " + dimension + ": ")
	for i, row := range rows {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(row.Name + " " + strconv.Itoa(row.Count))
	}
	return out.String()
}
