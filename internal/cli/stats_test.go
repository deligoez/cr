package cli

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The repository whose ledger every test here is computed over, and the two
// pull requests §7.3.2's counts have to span.
const (
	statsOwner = "acme"
	statsRepo  = "web"
	statsSlug  = statsOwner + "/" + statsRepo
	statsFirst = 7
	statsLater = 9
)

// statsHome is a state root holding §2.2's tree and no triage ledger yet.
func statsHome(t *testing.T) state.Layout {
	t.Helper()
	root := crHome(t)
	layout := state.New(root)
	require.NoError(t, layout.Init())
	return layout
}

// aTriagedRecord is one record as §7.3.1's events are written from it: the
// class and the rule id the counts are cut by, and nothing the counts do not
// read.
func aTriagedRecord(id, class, rule string) *finding.Finding {
	return &finding.Finding{
		ID: id, Kind: finding.KindFinding,
		Axis: "correctness", Role: "correctness",
		Class: class, Rule: rule,
		Grade: finding.GradeCited, Severity: finding.SeverityHigh,
		Summary: "The error is dropped.",
	}
}

// raise writes the `raised` event §7.3.1 has `cr draft` write per queued
// record, on the given pull request and round.
func seedRaised(t *testing.T, l state.Layout, pr, round int, records ...*finding.Finding) {
	t.Helper()
	require.NoError(t, finding.RecordRaised(l, statsOwner, statsRepo, records, occasionOf(pr, round)))
}

// settle writes the one outcome event §7.3.1 has `cr post --confirm` write per
// queued record.
func seedOutcome(
	t *testing.T, l state.Layout, pr, round int,
	record *finding.Finding, outcome finding.Outcome,
) {
	t.Helper()
	require.NoError(t, finding.RecordOutcomes(l, statsOwner, statsRepo,
		[]finding.Settled{{Record: record, Outcome: outcome}}, occasionOf(pr, round)))
}

// occasionOf is the occasion one seeded write is performed for. The moment
// advances with the round so the ledger is written in a plausible order, which
// is what §7.3.3's first-seen report will later be read against.
func occasionOf(pr, round int) *finding.TriageOccasion {
	return &finding.TriageOccasion{
		PR: pr, Round: round, Head: "0a1b2c3",
		At: time.Date(2026, 9, 12, round, pr, 0, 0, time.UTC),
	}
}

// statsReport runs `cr stats --repo` and decodes what it printed.
func statsReport(t *testing.T) statsResult {
	t.Helper()
	printed, err := runCLIPrinting(t, "stats", "--repo", statsSlug)
	require.NoError(t, err)
	var report statsResult
	require.NoError(t, json.Unmarshal([]byte(printed), &report))
	return report
}

// §7.3.2 through the command: the five counts per class and per rule, computed
// over the repository's triage.ndjson across its pull requests.
//
// Two pull requests is the whole point of the fixture. §7.3.4 divides by a
// class's raises over the repository, so a report that counted one pull
// request's events would give every class a sample small enough to fall under
// §7.3.4's minimum whatever the project's history — and the defect would look
// exactly like a project that has not reviewed much.
//
// The per-rule cut is asserted on a class no single rule produced: `f4` carries
// the same class as `f1` and `f3` and no rule id at all, so the class row counts
// it and the rule row does not. A per-rule cut derived from the class rows would
// report three raises for a rule that produced two.
//
// The later pull request spans two of its own rounds, which is the second thing
// a repository-wide count has to get right: a round index is per pull request,
// so PR 7 round 1 and PR 9 round 1 are different occasions and a tally keyed on
// the round alone would fold them together.
func TestStatsCountsEveryActionPerClassAndPerRuleAcrossPullRequests(t *testing.T) {
	layout := statsHome(t)

	dropped := aTriagedRecord("f1", "unchecked-error", "no-dropped-error")
	untested := aTriagedRecord("f2", "missing-test", "")
	seedRaised(t, layout, statsFirst, 1, dropped, untested)
	seedOutcome(t, layout, statsFirst, 1, dropped, finding.OutcomeKept)
	seedOutcome(t, layout, statsFirst, 1, untested, finding.OutcomeDiscardedNotHere)

	alsoDropped := aTriagedRecord("f3", "unchecked-error", "no-dropped-error")
	seedRaised(t, layout, statsLater, 1, alsoDropped)
	seedOutcome(t, layout, statsLater, 1, alsoDropped, finding.OutcomeSoftened)

	unruled := aTriagedRecord("f4", "unchecked-error", "")
	seedRaised(t, layout, statsLater, 2, unruled)
	seedOutcome(t, layout, statsLater, 2, unruled, finding.OutcomeDiscardedWrong)

	report := statsReport(t)

	assert.Equal(t, statsSlug, report.Repo)
	assert.Equal(t, 8, report.Events, "four raises and four outcomes")
	assert.Equal(t, []finding.ClassTriage{
		{Class: "missing-test", TriageCounts: finding.TriageCounts{
			Raised: 1, DiscardedNotHere: 1,
		}},
		{Class: "unchecked-error", TriageCounts: finding.TriageCounts{
			Raised: 3, Kept: 1, Softened: 1, DiscardedWrong: 1,
		}},
	}, report.Classes, "§7.3.2: the five counts per class, over both pull requests")
	assert.Equal(t, []finding.RuleTriage{
		{Rule: "no-dropped-error", TriageCounts: finding.TriageCounts{
			Raised: 2, Kept: 1, Softened: 1,
		}},
	}, report.Rules, "§7.3.2: and per rule, counting only the records a rule produced")
}

// A repository nothing has been triaged on reports empty counts rather than
// refusing, and the arrays are §12.3's empty ones.
//
// It is the answer a project gets on the day it installs cr, so it has to be
// an answer: a refusal here would read as a broken install, and a `null` in
// place of either array would break the agent that parses the document.
func TestStatsOnAnUntriagedRepositoryReportsNoCountsRatherThanRefusing(t *testing.T) {
	statsHome(t)

	printed, err := runCLIPrinting(t, "stats", "--repo", statsSlug)
	require.NoError(t, err)
	assert.Contains(t, printed, `"classes": []`)
	assert.Contains(t, printed, `"rules": []`)

	report := statsReport(t)
	assert.Equal(t, 0, report.Events)
	assert.Empty(t, report.Classes)
	assert.Empty(t, report.Rules)
}

// An action outside §7.3.1's five stops the report rather than being counted by
// no arm of the tally.
//
// triage.ndjson is a file under `~/.cr` a user can open, and §7.3.1 calls the
// five names the complete vocabulary the statistics are computed from. A sixth
// silently skipped would shrink the class's raises — §7.3.4's denominator —
// while every outcome still counted, which is the one direction that makes a
// class look worse than its history says.
func TestStatsRefusesAnActionOutsideSection731sFive(t *testing.T) {
	_, err := finding.Tally([]finding.TriageEvent{
		{Record: "f1", Action: "retracted", Class: "unchecked-error"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "f1")
	assert.Contains(t, err.Error(), "retracted")
}
