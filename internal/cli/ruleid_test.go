package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// ruleID is the rule the two fan-out records below attribute themselves to.
const ruleID = "handle-every-error"

// ruleClass is the class ruleID's file gives the records it produces, and the
// class aRecord already carries.
const ruleClass = "unchecked-error"

// attributedRecord is a §6.1 record a rule produced: it carries the rule id of
// §2.6 item 3 and the `suggestion_origin: rule` §2.6.2.4 gives a suggestion a
// rule's `fix` wrote.
//
// The two records this builds share an anchor and a class and differ in role,
// which is §6.4.1's duplicate key exactly. That is the case worth carrying the
// id through: §6.4.3 keeps the suppressed record rather than dropping it, and a
// merge that rebuilt records instead of editing them in place would lose the
// field on the one nobody looks at first.
func attributedRecord(id, lens string) map[string]any {
	record := aRecord(id, "u1")
	record["role"] = lens
	record["rule"] = ruleID
	record["suggestion"] = "if err != nil {"
	record["suggestion_origin"] = "rule"
	return record
}

// writeAttributingRule writes ruleID as a global rule of class ruleClass, so
// the records naming it are held to §2.6's class row against a real corpus.
func writeAttributingRule(t *testing.T, layout state.Layout) {
	t.Helper()
	body := `{"id":"` + ruleID + `","title":"Every error is handled.",` +
		`"rationale":"A dropped error hides the failure it reports.","class":"` + ruleClass + `"}`
	require.NoError(t, os.WriteFile(layout.Rule(ruleID), []byte(body), 0o600))
}

// fanOutFile writes one role's §4.6.2 output file and returns its path. The
// name is finding.FanOutFile's, because §6.1.3 binds a record's role to the
// file it arrived in by that name.
func fanOutFile(t *testing.T, dir, lens string, records ...map[string]any) string {
	t.Helper()
	var body bytes.Buffer
	for _, record := range records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		body.Write(line)
		body.WriteByte('\n')
	}
	path := filepath.Join(dir, finding.FanOutFile(lens))
	require.NoError(t, os.WriteFile(path, body.Bytes(), 0o600))
	return path
}

// §2.6 item 3: a rule-produced record's rule id survives merge and record.
//
// The id is carried across three rewrites of the record and has to come out the
// other end on both records: `cr merge`'s per-role decode of §6.1.3 and its
// §6.4 dedup, and `cr record`'s own stamping of state, axis and grade. Every one
// of them either builds a new value or writes into the record, and a field
// dropped by any of them would leave §7.3.2's per-rule statistics and
// §2.6.3.4's dead-rule report counting hits against no rule at all — with
// nothing in the run saying so, because §6.1 marks `rule` optional and a record
// without it is well formed.
//
// Both commands are the real ones: the file `cr record` reads is the one
// `cr merge` wrote, which is also the only file §6.5.1 lets carry the
// `duplicate_of` the suppressed record needs. The suppressed duplicate is
// asserted alongside the representative, because it is the record a merge is
// likeliest to lose the field on and §6.4.3 keeps it deliberately.
func TestARuleProducedRecordKeepsItsRuleIDThroughMergeAndRecord(t *testing.T) {
	layout := recordedHome(t)
	writeAttributingRule(t, layout)
	dir := t.TempDir()
	file := mergedOut(t)
	_, err := runMergeCLI(t, file,
		fanOutFile(t, dir, "correctness", attributedRecord("f1", "correctness")),
		fanOutFile(t, dir, "convention", attributedRecord("f2", "convention")),
	)
	require.NoError(t, err)

	_, err = runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, recordOwner, recordRepo, recordPRNum, state.FileFindings,
	)
	require.NoError(t, err)
	require.Len(t, stored, 2)

	byID := map[string]finding.Finding{}
	for _, record := range stored {
		byID[record.ID] = record
	}
	for _, id := range []string{"f1", "f2"} {
		require.Containsf(t, byID, id, "%s did not reach findings.ndjson", id)
		assert.Equalf(t, ruleID, byID[id].Rule,
			"§2.6 item 3: %s lost the rule that produced it", id)
		assert.Equalf(t, ruleClass, byID[id].Class, "§2.6's class row: %s carries its rule's class", id)
	}
	assert.Equal(t, finding.OriginRule, byID["f1"].SuggestionOrigin,
		"§2.6.2.4's mark survives beside the id it obliges")
	assert.Equal(t, [2]string{"f2", ""}, [2]string{byID["f1"].DuplicateOf, byID["f2"].DuplicateOf},
		"§6.4.2 elects f2 and §6.4.3 keeps f1 as its suppressed duplicate")
	assert.Equal(t, finding.StateDuplicate, byID["f1"].State)
}

// §2.6's class row through `cr record`: a record naming a rule whose class
// differs from that rule's is refused with exit code 1, naming the file, the
// line and the field, and nothing is stored. cr refuses rather than assigns,
// because both fields are the agent's and cr cannot say which one is wrong.
//
// The faulty record is the second line, after a record carrying the rule's
// class, so the line named is the one at fault and a matching class is shown
// passing in the same run. A record naming a rule the corpus does not hold has
// no class to be compared with, and is accepted.
func TestARecordWhoseClassIsNotItsRulesIsRefused(t *testing.T) {
	layout := recordedHome(t)
	writeAttributingRule(t, layout)

	matching := aRecord("f1", "u1")
	matching["rule"] = ruleID
	differing := aRecord("f2", "u2")
	differing["rule"] = ruleID
	differing["class"] = "dropped-error"
	file := writeRecordFile(t, "merged.ndjson", matching, differing)

	_, err := runRecord(t, recordPR, file, "--repo", recordSlug)

	var rejected *finding.RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "a record fault exits 1")
	assert.Equal(t, &finding.RejectedRecordError{
		File: file, Line: 2, Field: "class",
		Problem: `reads "dropped-error", and record f2 names rule "handle-every-error", whose class is ` +
			`"unchecked-error"; §2.6's class row gives a record its rule's class, so write "unchecked-error" ` +
			`or name the rule that produced the record`,
	}, rejected)
	assert.Empty(t, recordStore(t, layout), "a refused line takes the whole file with it")

	unknown := aRecord("f3", "u1")
	unknown["rule"] = "no-such-rule"
	unknown["class"] = "dropped-error"
	_, err = runRecord(t, recordPR, writeRecordFile(t, "unknown.ndjson", matching, unknown), "--repo", recordSlug)
	require.NoError(t, err)
	assert.Len(t, recordStore(t, layout), 2)
}
