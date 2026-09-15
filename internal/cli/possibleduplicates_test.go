package cli

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The two files of pairedHome's head, and the first head line of each unit's
// one seven-line hunk range. Two files are what field-feedback 2.4's pairs
// look like: every one of the four real pairs had its records in different
// files, which is why §6.4.1's key never grouped them.
const (
	pairedHandler = "internal/api/handler.go"
	pairedStore   = "internal/api/store.go"
)

var pairedUnits = []struct {
	id, path string
	start    int
}{
	{"u1", pairedHandler, 40},
	{"u2", pairedStore, 40},
	{"u3", pairedHandler, 90},
	{"u4", pairedStore, 90},
}

// pairedHome is recordedHome with the round's units spread over two files of a
// hundred lines each, so records can sit in different files and cite each
// other's lines.
func pairedHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	body := make([]string, 0, 100)
	for n := 1; n <= 100; n++ {
		body = append(body, fmt.Sprintf("line %d", n))
	}
	for _, name := range []string{pairedHandler, pairedStore} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(body, "\n")+"\n"), 0o600))
	}
	mustGit(t, dir, "-c", "init.defaultBranch=main", "init", "--quiet")
	mustGit(t, dir, "add", pairedHandler, pairedStore)
	mustGit(t, dir, "commit", "--quiet", "-m", "two files under review")
	head := strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))

	restore := repoDir
	repoDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { repoDir = restore })
	layout := state.New(crHome(t))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsurePR(recordOwner, recordRepo, recordPRNum))

	lines := make([]string, 0, len(pairedUnits))
	for _, formed := range pairedUnits {
		lines = append(lines, fmt.Sprintf(
			`{"id":%q,"path":%q,"hunk_ranges":[{"start":%d,"end":%d}],"head":%q,"round":%d}`,
			formed.id, formed.path, formed.start, formed.start+6, head, recordRound))
	}
	held, err := layout.LockPR(recordOwner, recordRepo, recordPRNum)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&state.Meta{
		Owner: recordOwner, Repo: recordRepo, PR: recordPRNum, Round: recordRound, Head: head,
	}))
	require.NoError(t, held.Write(state.FileUnits, []byte(strings.Join(lines, "\n")+"\n")))
	require.NoError(t, held.Unlock())
}

// aPairedRecord is one role's record on unit, anchored two to four lines into
// the unit's range, in its own class so no two records share §6.4.1's key by
// accident, citing each `path:line` in cites.
func aPairedRecord(id, role, class, unit string, cites ...string) map[string]any {
	record := aRoleRecord(id, role, class, unit)
	for _, formed := range pairedUnits {
		if formed.id == unit {
			record["anchor"] = map[string]any{
				"path": formed.path, "side": "RIGHT",
				"start_line": formed.start + 2, "line": formed.start + 4,
			}
		}
	}
	citations := make([]map[string]any, 0, len(cites))
	for _, cite := range cites {
		path, line, found := strings.Cut(cite, ":")
		if !found {
			panic("a citation is path:line, not " + cite)
		}
		var n int
		if _, err := fmt.Sscanf(line, "%d", &n); err != nil {
			panic(err)
		}
		citations = append(citations, map[string]any{"path": path, "line": n})
	}
	record["citations"] = citations
	return record
}

// field-feedback 2.4 through `cr merge`: records a shared citation or a
// citation inside the other's anchor joins are listed as pairs, with the link
// kinds, in the report and nowhere else; the merged file keeps every record,
// unaltered, and `cr record` still accepts it.
//
// The anchors are u1 handler.go 42-44, u2 store.go 42-44, u3 handler.go 92-94
// and u4 store.go 92-94. The cited lines sit on both ends of a range (42 and
// 94), and f1's near misses sit one line outside two (45 and 91), so a range
// read one line wider or narrower lists a different set. The correctness file
// is read first, so the arrival order is f1, f4, f2, f3, and f4 is the citing
// record of its pair with f2 while f3 is the cited-by one of its pair with f2:
// both directions of the anchor link are exercised.
func TestMergeListsPossibleDuplicatePairsWithoutTouchingTheRecords(t *testing.T) {
	pairedHome(t)
	dir, out := t.TempDir(), mergedOut(t)
	correctness := writeFanOut(t, dir, "correctness",
		aPairedRecord("f1", "correctness", "class-one", "u1", pairedStore+":10", pairedStore+":45", pairedHandler+":91"),
		aPairedRecord("f4", "correctness", "class-four", "u4", pairedHandler+":94", pairedStore+":42"))
	convention := writeFanOut(t, dir, "convention",
		aPairedRecord("f2", "convention", "class-two", "u2", pairedStore+":10"),
		aPairedRecord("f3", "convention", "class-three", "u3", pairedStore+":42"))

	printed, err := runMergeCLI(t, out, correctness, convention)
	require.NoError(t, err)
	reported := decodeMergeResult(t, printed)

	assert.Equal(t, 4, reported.Merged, "a listed pair drops nothing")
	assert.Empty(t, reported.Overlaps, "no two records share §6.4.1's key")
	assert.Equal(t, []finding.PossibleDuplicate{
		{Records: []string{"f1", "f2"}, Links: []finding.DuplicateLink{finding.LinkSharedCitation}},
		{Records: []string{"f4", "f2"}, Links: []finding.DuplicateLink{finding.LinkCitesAnchor}},
		{Records: []string{"f4", "f3"}, Links: []finding.DuplicateLink{finding.LinkSharedCitation, finding.LinkCitesAnchor}},
		{Records: []string{"f2", "f3"}, Links: []finding.DuplicateLink{finding.LinkCitesAnchor}},
	}, reported.PossibleDuplicates)

	text := strings.Split(reported.Text(&writer{}), "\n")
	require.Len(t, text, 1+4+5+len(reported.Honesty))
	assert.Equal(t, []string{
		"possible duplicate pairs: 4",
		"  f1 and f2: shared-citation",
		"  f4 and f2: cites-anchor",
		"  f4 and f3: shared-citation, cites-anchor",
		"  f2 and f3: cites-anchor",
	}, text[5:10], "the terminal lists the same pairs, between the breakdowns and the disclosures")

	body, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, [][]string{pairedLineKeys, pairedLineKeys, pairedLineKeys, pairedLineKeys},
		keysOfMergedLines(t, body),
		"§6.5.1: a record line carries the fields the role supplied and nothing the listing computed")

	_, err = runRecord(t, recordPR, out, "--repo", recordSlug)
	require.NoError(t, err, "§6.1.4: `cr record` accepts the merged file the listing sat beside")
}

// field-feedback 2.4's negative half through `cr merge`: two records in
// different files with no shared citation and no citation inside the other's
// anchor are not a pair — though each cites a line with the other's number, and
// f2 cites a line numbered inside f1's range in another file — and two records
// §6.4.1 already grouped are not listed again, though they share a citation.
func TestMergeListsNoPairWhereNoLinkJoinsTheRecordsOrDedupAlreadyDid(t *testing.T) {
	pairedHome(t)
	dir, out := t.TempDir(), mergedOut(t)
	correctness := writeFanOut(t, dir, "correctness",
		aPairedRecord("f1", "correctness", "class-one", "u1", pairedStore+":1"),
		aPairedRecord("f3", "correctness", "same-class", "u3", pairedStore+":60"))
	convention := writeFanOut(t, dir, "convention",
		aPairedRecord("f2", "convention", "class-two", "u2", pairedHandler+":1", pairedStore+":43"),
		aPairedRecord("f4", "convention", "same-class", "u3", pairedStore+":60"))

	printed, err := runMergeCLI(t, out, correctness, convention)
	require.NoError(t, err)
	require.Len(t, decodeMergeResult(t, printed).Overlaps, 1, "f3 and f4 share §6.4.1's key")
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(printed), &document))
	assert.Equal(t, "[]", string(document["possible_duplicates"]),
		"§12.3: a merge with no pair prints an empty list, never null or no key")
}

// A LEFT anchor numbers the merge base, and a citation numbers the head
// (§6.2.3), so a citation carrying a number inside a LEFT anchor's range names
// a different line and is no link. removalHome's u2 removes merge-base lines 7
// and 8 of lib.go; f1 cites head line 7.
func TestMergeListsNoPairForACitationInsideALeftAnchorsRange(t *testing.T) {
	removalHome(t)
	left := func(id, class, cited string) map[string]any {
		record := aRoleRecord(id, "correctness", class, "u2")
		record["anchor"] = map[string]any{"path": "lib.go", "side": "LEFT", "start_line": 7, "line": 8}
		var n int
		_, err := fmt.Sscanf(cited, "%d", &n)
		require.NoError(t, err)
		record["citations"] = []map[string]any{{"path": "lib.go", "line": n}}
		return record
	}
	dir, out := t.TempDir(), mergedOut(t)
	file := writeFanOut(t, dir, "correctness", left("f1", "class-one", "7"), left("f2", "class-two", "4"))

	printed, err := runCLIPrinting(t, "merge", file, "-o", out, "--repo", fixtureSlug, "--pr", fixturePR)
	require.NoError(t, err)
	reported := decodeMergeResult(t, printed)
	assert.Equal(t, 2, reported.Merged)
	assert.Empty(t, reported.PossibleDuplicates)
}

// pairedLineKeys are the sorted top-level keys a merged line of an unsuppressed
// aPairedRecord holds: the rows the role supplied, and no computed field.
var pairedLineKeys = []string{
	"anchor", "citations", "class", "evidence", "id", "kind", "role", "severity", "summary", "unit",
}

// keysOfMergedLines reads each line of a merged file into its sorted top-level
// keys, and fails when a citation entry holds any key but path and line.
func keysOfMergedLines(t *testing.T, body []byte) [][]string {
	t.Helper()
	keys := make([][]string, 0)
	for line := range strings.SplitSeq(strings.TrimSpace(string(body)), "\n") {
		var held map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(line), &held))
		var citations []map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(held["citations"], &citations))
		for _, cited := range citations {
			assert.Equal(t, []string{"line", "path"}, slices.Sorted(maps.Keys(cited)))
		}
		keys = append(keys, slices.Sorted(maps.Keys(held)))
	}
	return keys
}
