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
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// ruleID is the rule the two fan-out records below attribute themselves to.
const ruleID = "handle-every-error"

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

// merged runs §6.5.1's merge over the per-role files: each is decoded through
// the door that binds its records to its role, and §6.4.2 and §6.4.3 are
// applied across the result.
//
// This is the machinery `cr merge` will call rather than the command, which
// command-surface-stubs registered and merge-command owns. What it leaves out
// is the counts §6.5.1 reports and the summary §10.3 accumulates, neither of
// which touches a record's fields — so the file it writes is the merged file a
// record's rule id has to survive. It does record the file's digest where
// `cr merge` would, since that is what lets `cr record` read `duplicate_of`.
func merged(t *testing.T, layout state.Layout, files ...string) string {
	t.Helper()
	records := make([]*finding.Finding, 0, len(files))
	for _, file := range files {
		body, err := os.ReadFile(file)
		require.NoError(t, err)
		lens, bound := finding.RoleForFile(file)
		require.True(t, bound, "%s binds its records to no role", file)
		decoded, err := finding.DecodePerRole(file, body, []string{"u1", "u2"})
		require.NoErrorf(t, err, "%s was refused before anything was merged", lens)
		records = append(records, decoded...)
	}

	corpus, err := role.Resolve(
		layout.RepoRolesDir(recordOwner, recordRepo), layout.RolesDir())
	require.NoError(t, err)
	overlaps := finding.MarkDuplicates(records, role.Order(corpus))
	require.Equal(t, 1, overlaps.Suppressed(),
		"the fixture's two records share §6.4.1's key, so one is suppressed")

	var body bytes.Buffer
	for _, record := range records {
		line, err := json.Marshal(record)
		require.NoError(t, err)
		body.Write(line)
		body.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), "merged.ndjson")
	require.NoError(t, os.WriteFile(path, unstamped(t, body.Bytes()), 0o600))
	return asMergeOutput(t, layout, path)
}

// unstamped removes §2.3.3's head and round from every line of a merged file.
//
// state.Stamp carries neither key with `omitempty`, deliberately: §2.3.3
// requires the pair on every stored record, so a stored line that omitted it
// would be one cr wrote wrong. The consequence is that marshalling a decoded
// record produces a line `cr record` refuses — state.DecodeStamped rejects a
// wire line supplying either key, by presence and not by value. So a merge that
// writes its records out has to take the pair off first, and this is that step
// standing in for the one merge-command will have to make.
func unstamped(t *testing.T, body []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	for line := range bytes.SplitSeq(bytes.TrimSpace(body), []byte("\n")) {
		var record map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(line, &record))
		delete(record, "head")
		delete(record, "round")
		rewritten, err := json.Marshal(record)
		require.NoError(t, err)
		out.Write(rewritten)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// §2.6 item 3: a rule-produced record's rule id survives merge and record.
//
// The id is carried across three rewrites of the record and has to come out the
// other end on both records: the per-role decode of §6.1.3, §6.4's dedup, and
// `cr record`'s own stamping of state, axis and grade. Every one of them either
// builds a new value or writes into the record, and a field dropped by any of
// them would leave §7.3.2's per-rule statistics and §2.6.3.4's dead-rule report
// counting hits against no rule at all — with nothing in the run saying so,
// because §6.1 marks `rule` optional and a record without it is well formed.
//
// The suppressed duplicate is asserted alongside the representative, because it
// is the record a merge is likeliest to lose the field on and §6.4.3 keeps it
// deliberately.
func TestARuleProducedRecordKeepsItsRuleIDThroughMergeAndRecord(t *testing.T) {
	layout := recordedHome(t)
	dir := t.TempDir()
	file := merged(t, layout,
		fanOutFile(t, dir, "correctness", attributedRecord("f1", "correctness")),
		fanOutFile(t, dir, "convention", attributedRecord("f2", "convention")),
	)

	_, err := runRecord(t, recordPR, file, "--repo", recordSlug)
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
	}
	assert.Equal(t, finding.OriginRule, byID["f1"].SuggestionOrigin,
		"§2.6.2.4's mark survives beside the id it obliges")
	require.NotEmpty(t, byID["f2"].DuplicateOf+byID["f1"].DuplicateOf,
		"neither record was suppressed, so the id was not carried through §6.4.3")
}
