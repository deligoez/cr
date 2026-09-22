package brief

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/text"
	"github.com/deligoez/cr/internal/unit"
)

// The issue text `sources` writes, and a span of it a claim can be drawn from.
// The span has to occur in the text for §3.3.3's occurrence half to report what
// it is here to report rather than a claim that no longer matches anything.
const (
	issueText  = "The order total sums the subtotal and the shipping.\n"
	claimSpan  = "sums the subtotal and the shipping"
	staleIssue = "The order total sums the subtotal.\n"
)

// advanceAdding moves the branch under review on by one commit that changes the
// file already under review and adds a second one.
//
// The second file is what makes §9.3.4's recomputation visible. A head that
// only edited `order.go` again would produce one unit called `u1`, exactly as
// the round before it did, and a brief that had carried the old units forward
// unchanged would be indistinguishable from one that recomputed them.
func advanceAdding(t *testing.T, dir string) string {
	t.Helper()
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	write("order.go", "package shop\n\nfunc Total() int { return subtotal() + shipping() + tax() }\n")
	write("tax.go", "package shop\n\nfunc tax() int { return 1 }\n")
	runGit(t, dir, "add", "order.go", "tax.go")
	runGit(t, dir, "commit", "--quiet", "-m", "tax the order")
	return runGit(t, dir, "rev-parse", "HEAD")
}

// seed writes the round the increment will close: one record in each of the two
// states §9.3.4 sweeps, one in a state it must not touch, a mapping, and a
// claim.
//
// The lines are written as NDJSON rather than encoded from this version's
// structs, and the drafted record carries a field no version of cr writes.
// §9.3.4 is a change to one field of a stored line, and a sweep that decoded
// each record into a struct and re-encoded it would silently drop whatever a
// later version had put beside that field — so the fixture is what a later
// version's file looks like, and the assertion below is that it survives.
func seed(t *testing.T, src *Sources, head string) {
	t.Helper()
	held, err := src.Layout.LockPR(testOwner, testRepo, testPR)
	require.NoError(t, err)

	stamp := `"head":"` + head + `","round":1`
	record := func(id, state, extra string) string {
		return `{"id":"` + id + `","kind":"finding","role":"correctness",` +
			`"class":"unchecked-error","severity":"high","unit":"u1",` +
			`"summary":"The shipping arm is unchecked.",` +
			`"evidence":"The branch has no test.",` +
			`"state":"` + state + `",` + extra + stamp + `}`
	}
	extracted, err := text.NormalisedHash(staleIssue)
	require.NoError(t, err)

	for name, body := range map[string]string{
		state.FileFindings: record("f1", "draft", `"future_note":"kept",`) + "\n" +
			record("f2", "queued", "") + "\n" +
			record("f3", "posted", "") + "\n",
		state.FileMapping: `{"claim":"` + testIssue + `#c1","unit":"u1",` + stamp + "}\n",
		state.FileClaims: `{"id":"` + testIssue + `#c1","text":"The total sums subtotal and shipping.",` +
			`"source":"acceptance","span":"` + claimSpan + `",` +
			`"issue_hash":"` + extracted + `",` + stamp + "}\n",
	} {
		require.NoError(t, held.Write(name, []byte(body)), name)
	}
	require.NoError(t, held.Unlock())
}

// lines reads one §2.3 row back as the fields each of its lines supplied, which
// is how a stored record is read when the question is what survived rather than
// what this version can decode.
func lines(t *testing.T, src *Sources, name string) []map[string]json.RawMessage {
	t.Helper()
	body, err := os.ReadFile(src.Layout.PRFile(testOwner, testRepo, testPR, name))
	require.NoError(t, err)
	out := make([]map[string]json.RawMessage, 0)
	for line := range strings.SplitSeq(string(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var fields map[string]json.RawMessage
		require.NoErrorf(t, json.Unmarshal([]byte(line), &fields), "%s: %s", name, line)
		out = append(out, fields)
	}
	return out
}

// field is one raw value of a stored line, decoded as a string.
func field(t *testing.T, line map[string]json.RawMessage, name string) string {
	t.Helper()
	var value string
	require.NoErrorf(t, json.Unmarshal(line[name], &value), "%s", name)
	return value
}

// §9.3.4 in full, over the increment §9.3.3 makes: the open records go stale,
// the mapping does not follow the round forward, the units are recomputed and
// renumbered for the new head, and the claims are carried forward unchanged
// with §3.3.3's comparison re-run against the issue text.
//
// The four are one test because §9.3.4 is one sentence about one increment and
// they are not independent of each other: a sweep that restamped the records it
// moved would pass a staleness assertion while taking the closing round's
// history with it, and a carry-forward that re-extracted would pass a survival
// assertion while inventing claims. What separates those from the honest
// implementation is what the *other* rows hold afterwards, so every row is read
// back rather than only the one under test.
func TestAMovedHeadStalesTheOpenRecordsAndCarriesTheClaimsForward(t *testing.T) {
	dir, first, base := repository(t)
	src := sources(t, dir, answering(first, base, oneThread))
	_, err := Run(src)
	require.NoError(t, err)
	seed(t, src, first)

	second := advanceAdding(t, dir)
	require.NotEqual(t, first, second)
	src.GH = gh.WithRunner(answering(second, base, oneThread))
	assembled, err := Run(src)
	require.NoError(t, err)
	require.Equal(t, 2, assembled.Round, "§9.3.3: the head moved, so the index did")

	t.Run("open records go stale and terminal ones do not", func(t *testing.T) {
		stored, err := state.ReadRecords[*finding.Finding](
			src.Layout, testOwner, testRepo, testPR, state.FileFindings)
		require.NoError(t, err)
		require.Len(t, stored, 3, "§9.3.4 moves records; it deletes none")

		states := map[string]finding.State{}
		for _, record := range stored {
			states[record.ID] = record.State
			assert.Equalf(t, 1, record.Round,
				"%s was produced in round 1, and §2.3.3's pair says which run wrote it", record.ID)
			assert.Equal(t, first, record.Head, record.ID)
		}
		assert.Equal(t, finding.StateStale, states["f1"], "§9.3.4: a drafted record goes stale")
		assert.Equal(t, finding.StateStale, states["f2"], "§9.3.4: a queued record goes stale")
		assert.Equal(t, finding.StatePosted, states["f3"],
			"§9.3.4 names draft and queued, and §9.1 gives cr brief no row out of posted")

		for _, line := range lines(t, src, state.FileFindings) {
			if field(t, line, "id") != "f1" {
				continue
			}
			assert.Equal(t, "kept", field(t, line, "future_note"),
				"the sweep changes one field and carries every other through")
		}
	})

	t.Run("the mapping is not carried forward and round 1's stays as history", func(t *testing.T) {
		stored := lines(t, src, state.FileMapping)
		require.Len(t, stored, 1, "§9.3.4 opens round 2 with no mapping, and §9.3.5 keeps round 1's")
		assert.Equal(t, "1", string(stored[0]["round"]))
		assert.Equal(t, "u1", field(t, stored[0], "unit"))
		assert.Equal(t, first, field(t, stored[0], "head"))
	})

	t.Run("units are recomputed and renumbered for the new head", func(t *testing.T) {
		kept, err := state.ReadStamped[unit.Record](
			src.Layout, testOwner, testRepo, testPR, state.FileUnits, 1)
		require.NoError(t, err)
		require.Len(t, kept, 1, "§9.3.5 keeps round 1's unit, which its stale records and mapping name")
		assert.Equal(t, "u1", kept[0].ID)
		assert.Equal(t, "order.go", kept[0].Path)
		assert.Equal(t, first, kept[0].Head)

		stored, err := state.ReadStamped[unit.Record](
			src.Layout, testOwner, testRepo, testPR, state.FileUnits, 2)
		require.NoError(t, err)
		require.Len(t, stored, 2, "the new head touches two files, where the old one touched one")

		paths := make([]string, 0, len(stored))
		for _, record := range stored {
			paths = append(paths, record.Path)
			assert.Equal(t, 2, record.Round, "§2.3.3 stamps the round the units were computed in")
			assert.Equal(t, second, record.Head, "§9.3.4 recomputes the units for the new head")
		}
		slices.Sort(paths)
		assert.Equal(t, []string{"order.go", "tax.go"}, paths)

		ids := make([]string, 0, len(stored))
		for _, record := range stored {
			ids = append(ids, record.ID)
		}
		slices.Sort(ids)
		assert.Equal(t, []string{"u1", "u2"}, ids,
			"§3.4.6 numbers the round's units from the start, so the set is renumbered")
	})

	t.Run("claims are carried forward with a drift report", func(t *testing.T) {
		stored := lines(t, src, state.FileClaims)
		require.Len(t, stored, 2,
			"§9.3.4 carries the claim into round 2, and §9.3.5 keeps round 1's line as history")

		byRound := map[string]map[string]json.RawMessage{}
		for _, line := range stored {
			byRound[string(line["round"])] = line
		}
		opened, carried := byRound["1"], byRound["2"]
		require.NotNil(t, opened)
		require.NotNil(t, carried)
		assert.Equal(t, first, field(t, opened, "head"))
		assert.Equal(t, second, field(t, carried, "head"),
			"the carried claim is round 2's record, so §2.3.3 stamps it with round 2's head")
		for _, name := range []string{"id", "text", "source", "span", "issue_hash"} {
			assert.Equalf(t, field(t, opened, name), field(t, carried, name),
				"§9.3.4 carries the claim forward unchanged, and %s changed", name)
		}

		// The payload reports the same records the file now holds, which
		// is what §3.7.3 prints, and the drift report is §3.3.3 re-run
		// rather than an extraction: the hash is of the issue text as it
		// now reads, and the claim still carries the hash it was
		// extracted against.
		require.Len(t, assembled.Claims, 1)
		assert.Equal(t, testIssue+"#c1", assembled.Claims[0].ID)
		assert.Equal(t, 2, assembled.Claims[0].Round)

		current, err := text.NormalisedHash(issueText)
		require.NoError(t, err)
		extracted, err := text.NormalisedHash(staleIssue)
		require.NoError(t, err)
		assert.Equal(t, current, assembled.Drift.Hash,
			"§9.3.4 re-reads the issue text, so the comparison is against it as it now reads")
		assert.True(t, assembled.Drift.Drifted,
			"the claim was extracted against an earlier issue text, and §3.3.3 says so")
		require.Len(t, assembled.Drift.Claims, 1)
		assert.Equal(t, extracted, assembled.Drift.Claims[0].ExtractedHash,
			"§3.3.3 reports the hash the claim was extracted against; it does not re-extract")
		assert.True(t, assembled.Drift.Claims[0].SpanOccurs)
		assert.Equal(t, field(t, carried, "issue_hash"), extracted,
			"the carried claim keeps the hash it was extracted against, so drift stays reportable")
	})
}

// A brief that opens no round performs none of §9.3.4.
//
// §9.3.3's second half is that a same-head brief is idempotent, and each clause
// of §9.3.4 breaks it in its own way: the clearing would take out a mapping
// `cr map record` wrote into the open round, and the carry-forward would
// rewrite that round's claims under a round nobody closed. Only the sweep is
// harmless at the same head, and a test that watched the sweep alone would pass
// on an implementation that ran all three.
func TestASameHeadBriefPerformsNoneOfSection934(t *testing.T) {
	dir, head, base := repository(t)
	src := sources(t, dir, answering(head, base, oneThread))
	_, err := Run(src)
	require.NoError(t, err)
	seed(t, src, head)

	before := rows(t, src)
	assembled, err := Run(src)
	require.NoError(t, err)
	require.Equal(t, 1, assembled.Round, "§9.3.3: the head has not moved")

	after := rows(t, src)
	for _, name := range []string{state.FileFindings, state.FileMapping, state.FileClaims} {
		assert.Equalf(t, before[name], after[name],
			"§9.3.4 runs on the increment, and a same-head brief made none, so %s is untouched", name)
	}
}

// On the increment `cr brief` writes the rows §9.3.4 names and no others.
//
// This is the carve-out the same-head fence in nojudgement_test.go cannot make.
// That guard compares the whole §2.3 table and permits three rows, which is the
// right answer for every brief that opens no round; on an increment §9.3.4
// requires more, findings.ndjson among them, and a guard that forbade
// them everywhere would forbid the section. So the exception is written out
// here as its own closed list, against the same whole table: a brief that
// started writing a fourth row on a moved head fails this, and one that started
// writing any row at all on an unmoved head fails the other.
//
// The permitted rows are asserted to have actually changed, not merely
// permitted to. A list that grew a row nothing writes would quietly widen what
// §9.3.4 admits, and nothing else would notice.
func TestAnIncrementWritesTheRowsSection934NamesAndNoOthers(t *testing.T) {
	dir, first, base := repository(t)
	src := sources(t, dir, answering(first, base, oneThread))
	_, err := Run(src)
	require.NoError(t, err)
	seed(t, src, first)

	// Every other row is given a line of its own, so a write into one is a
	// difference rather than a file that was empty before and after.
	held, err := src.Layout.LockPR(testOwner, testRepo, testPR)
	require.NoError(t, err)
	sentinel := map[string]string{}
	for _, name := range state.PRFiles() {
		if name == state.FileMeta {
			continue
		}
		if _, seeded := invalidated[name]; seeded {
			continue
		}
		line := `{"id":"sentinel","head":"` + first + `","round":1}` + "\n"
		require.NoError(t, held.Write(name, []byte(line)), name)
		sentinel[name] = line
	}
	require.NoError(t, held.Unlock())

	before := rows(t, src)
	second := advanceAdding(t, dir)
	src.GH = gh.WithRunner(answering(second, base, oneThread))
	_, err = Run(src)
	require.NoError(t, err)
	after := rows(t, src)

	for _, name := range state.PRFiles() {
		_, permitted := invalidated[name]
		if slices.Contains(derivedFiles, name) || permitted {
			assert.NotEqualf(t, before[name], after[name],
				"%s is permitted on an increment because §9.3.4 writes it, and this run did not", name)
			continue
		}
		assert.Equalf(t, before[name], after[name],
			"§9.3.4 and §9.1.1 name three rows beside §3.7's derived inputs, and %s is not one of them", name)
	}
	for name, line := range sentinel {
		if slices.Contains(derivedFiles, name) {
			continue
		}
		assert.Equalf(t, line, after[name], "%s was neither added to nor cleared", name)
	}
}

// invalidated are the rows §9.3.4 writes on top of §3.7's derived inputs:
// findings.ndjson holds the records it moves to `stale`, and claims.ndjson holds
// the claims it carries forward — plus transitions.ndjson, where §9.1.1 has each
// of those moves to `stale` leave its journal line, and migrations.ndjson, where
// §9.4.7 has the increment report every record it migrated. mapping.ndjson is
// not among them: §9.3.4 clears the opening round's mapping, which holds no
// line, and §9.3.5 keeps every earlier round's.
var invalidated = map[string]bool{
	state.FileFindings:    true,
	state.FileClaims:      true,
	state.FileTransitions: true,
	state.FileMigrations:  true,
}

// The claims a brief carries forward are claims and not extractions.
//
// §9.3.4's last clause is a prohibition, and a prohibition is not tested by the
// happy path: a brief that re-extracted would still produce claims, and the
// test above would still find one. What distinguishes them is what happens when
// the issue text says something the recorded claims do not — a re-extraction
// would follow the text, and a carry-forward follows the file.
func TestACarriedForwardClaimFollowsTheFileAndNotTheIssueText(t *testing.T) {
	dir, first, base := repository(t)
	src := sources(t, dir, answering(first, base, oneThread))
	_, err := Run(src)
	require.NoError(t, err)
	seed(t, src, first)

	// An issue text with nothing in common with the recorded claim. Any
	// extraction against it would produce something else or nothing.
	require.NoError(t, os.WriteFile(src.Intent.File,
		[]byte("Ship the invoice as a PDF.\n"), 0o600))

	second := advanceAdding(t, dir)
	src.GH = gh.WithRunner(answering(second, base, oneThread))
	assembled, err := Run(src)
	require.NoError(t, err)

	require.Len(t, assembled.Claims, 1, "§9.3.4 carries the recorded claims and extracts none")
	assert.Equal(t, testIssue+"#c1", assembled.Claims[0].ID)
	assert.Equal(t, claimSpan, assembled.Claims[0].Span,
		"the span is the one the claim was recorded with, not one drawn from the new text")
	assert.False(t, assembled.Drift.Claims[0].SpanOccurs,
		"§3.3.3 reports that the span no longer occurs; it does not repair the claim")

	stored, err := state.ReadRecords[intent.Claim](
		src.Layout, testOwner, testRepo, testPR, state.FileClaims)
	require.NoError(t, err)
	require.Len(t, stored, 2)
	for _, claim := range stored {
		assert.Equal(t, claimSpan, claim.Span, "neither round's claim was re-extracted")
	}
}
